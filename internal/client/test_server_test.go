package client

import (
	"net/http"
	"testing"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/httpapi"
	"github.com/havspect/tstamps/internal/onboarding"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/report"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
	"gorm.io/gorm"
)

func buildTestServer(t *testing.T, dbPath string) (http.Handler, *gorm.DB) {
	t.Helper()

	db, err := gormrepo.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := gormrepo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	clk := clock.System{}
	uow := gormrepo.NewTxRunner(db)

	userRepo := gormrepo.NewUserRepository(db)
	projectRepo := gormrepo.NewProjectRepository(db)
	teamRepo := gormrepo.NewTeamRepository(db)
	entryRepo := gormrepo.NewEntryRepository(db)
	auditRepo := gormrepo.NewAuditRepository(db)
	onboardingRepo := gormrepo.NewOnboardingRepository(db)
	reportSource := gormrepo.NewReportSource(db)

	policy := authz.NewPolicyEngine()
	auditSvc := audit.New(auditRepo, clk)
	userSvc := user.New(userRepo, auditSvc, entryRepo, clk)
	teamSvc := team.New(teamRepo, teamRepo, auditSvc, clk)
	projectSvc := project.NewService(projectRepo, uow, auditSvc, projectRepo, policy, clk)
	entrySvc := entry.NewService(entryRepo, uow, auditSvc, entryRepo, policy, clk)
	reportSvc := report.NewService(reportSource, policy, clk)
	onboardingSvc := onboarding.NewService(onboardingRepo)

	handler := httpapi.New(httpapi.Dependencies{
		DB:                db,
		Clock:             clk,
		UserService:       userSvc,
		ProjectService:    projectSvc,
		TeamService:       teamSvc,
		EntryService:      entrySvc,
		ReportService:     reportSvc,
		AuditService:      auditSvc,
		OnboardingService: onboardingSvc,
	})

	return handler, db
}
