// Package app provides the composition root for Tstamps, wiring configuration,
// persistence, domain services, and inbound transports (HTTP API and MCP).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/httpapi"
	"github.com/havspect/tstamps/internal/mcpserver"
	"github.com/havspect/tstamps/internal/oauth"
	"github.com/havspect/tstamps/internal/onboarding"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/report"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
	"gorm.io/gorm"
)

// Config defines the application startup configuration.
type Config struct {
	Addr        string
	DB          string
	LogLevel    slog.Level
	DevMode     bool
	EmailAPIKey string
	EmailAPIURL string
	EmailFrom   string
	PublicURL   string
}

// FromConfig converts a config.Config into an app.Config.
func FromConfig(c config.Config) Config {
	return Config{
		Addr:        c.Addr,
		DB:          c.DB,
		LogLevel:    c.LogLevel,
		EmailAPIKey: c.EmailAPIKey,
		EmailAPIURL: c.EmailAPIURL,
		EmailFrom:   c.EmailFrom,
		PublicURL:   c.PublicURL,
	}
}

// BuildServer wires all layers from config and returns the configured http.Handler,
// the underlying database connection, and any error encountered.
func BuildServer(cfg Config) (http.Handler, *gorm.DB, error) {
	db, err := gormrepo.Open(cfg.DB)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	if err := gormrepo.Migrate(db); err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, nil, fmt.Errorf("migrate database: %w", err)
	}

	clk := clock.System{}
	uow := gormrepo.NewTxRunner(db)

	// Repositories
	userRepo := gormrepo.NewUserRepository(db)
	projectRepo := gormrepo.NewProjectRepository(db)
	teamRepo := gormrepo.NewTeamRepository(db)
	entryRepo := gormrepo.NewEntryRepository(db)
	auditRepo := gormrepo.NewAuditRepository(db)
	onboardingRepo := gormrepo.NewOnboardingRepository(db)
	reportSource := gormrepo.NewReportSource(db)
	oauthStore := gormrepo.NewOAuthStore(db, 0)

	var secretRow gormrepo.ServerSecret
	_ = db.First(&secretRow, 1)
	hmacSecret := secretRow.Secret

	// Domain Services
	policy := authz.NewPolicyEngine()
	auditSvc := audit.New(auditRepo, clk)
	userSvc := user.New(userRepo, auditSvc, entryRepo, clk)
	teamSvc := team.New(teamRepo, teamRepo, auditSvc, clk)
	projectSvc := project.NewService(projectRepo, uow, auditSvc, projectRepo, policy, clk)
	entrySvc := entry.NewService(entryRepo, uow, auditSvc, entryRepo, policy, clk)
	reportSvc := report.NewService(reportSource, policy, clk)
	onboardingSvc := onboarding.NewService(onboardingRepo)
	if cfg.EmailAPIKey != "" {
		endpoint := cfg.EmailAPIURL
		if endpoint == "" {
			endpoint = onboarding.DefaultEmailEndpoint
		}
		from := cfg.EmailFrom
		if from == "" {
			from = "Tstamps <hello@example.com>"
		}
		onboardingSvc.WithMailer(&onboarding.Resend{
			Endpoint: endpoint,
			Key:      cfg.EmailAPIKey,
			From:     from,
		}, cfg.PublicURL)
	}
	oauthProvider := oauth.NewProvider(oauthStore, hmacSecret)

	// Inbound Transport: MCP Server
	mcpHandler := mcpserver.NewHandler(mcpserver.Config{
		UserService:    userSvc,
		ProjectService: projectSvc,
		TeamService:    teamSvc,
		EntryService:   entrySvc,
		ReportService:  reportSvc,
	})

	// Inbound Transport: HTTP API
	httpHandler := httpapi.New(httpapi.Dependencies{
		DB:                db,
		Clock:             clk,
		UserService:       userSvc,
		ProjectService:    projectSvc,
		TeamService:       teamSvc,
		EntryService:      entrySvc,
		ReportService:     reportSvc,
		AuditService:      auditSvc,
		OnboardingService: onboardingSvc,
		OAuthProvider:     oauthProvider,
		MCPHandler:        mcpHandler,
		DevMode:           cfg.DevMode,
	})

	return httpHandler, db, nil
}

// Run executes the application composition root, starting the server and awaiting shutdown.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Addr == "" {
		cfg.Addr = config.DefaultAddr
	}
	if cfg.DB == "" {
		cfg.DB = config.DefaultDB
	}

	handler, db, err := BuildServer(cfg)
	if err != nil {
		return err
	}
	if db != nil {
		sqlDB, err := db.DB()
		if err == nil {
			defer sqlDB.Close()
		}
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Addr, err)
	}
	defer ln.Close()

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	slog.Info("serving", "addr", ln.Addr().String(), "db", cfg.DB)

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-sigCtx.Done():
	}

	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	slog.Info("tstamps server stopped gracefully")
	return nil
}
