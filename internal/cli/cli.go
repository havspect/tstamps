// Package cli implements the client subcommands: everything that talks to a
// running server through internal/client.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/havspect/tstamps/internal/client"
	"github.com/havspect/tstamps/internal/config"
)

// Client is the API surface the commands use. *client.Client is the production
// implementation; tests pass a fake.
type Client interface {
	Login(email, password string) (*client.Me, error)
	Me() (*client.Me, error)
	StartDeviceAuth() (*client.DeviceAuthResponse, error)
	PollDeviceToken(ctx context.Context, deviceCode string, interval time.Duration) (*client.TokenSet, error)
	RefreshOAuthToken() (*client.TokenSet, error)

	ListProjects(all bool) ([]client.Project, error)
	CreateProject(in client.ProjectInput) (*client.Project, error)
	PatchProject(id uint, p client.ProjectPatch) (*client.Project, error)

	ListTeams() ([]client.Team, error)
	ListUsers() ([]client.User, error)

	ListEntries(q client.EntryQuery) ([]client.Entry, error)
	CreateEntry(in client.EntryInput) (*client.Entry, error)
	PatchEntry(id uint, p client.EntryPatch) (*client.Entry, error)
	DeleteEntry(id uint) error
	StartTimer(projectID uint, timezone, note string) (*client.Entry, error)
	StopTimer(note string) (*client.Entry, error)

	Report(q client.ReportQuery) (*client.Report, error)
	ReportCSV(q client.ReportQuery) ([]byte, error)

	ListTokens() ([]client.Token, error)
	CreateToken(name string) (*client.Token, error)
	RevokeToken(id uint) error

	ListAudit(q client.AuditQuery) ([]client.AuditEvent, error)
}

// App is the CLI with its edges injected: writers, the client factory, the
// config file and the clock. New wires the real ones.
type App struct {
	Out, Err   io.Writer
	In         io.Reader
	NewClient  func(client.Config) Client
	LoadConfig func(urlFlag, tokenFlag string) (client.Config, error)
	SaveConfig func(client.Config) error
	Now        func() time.Time
	// Local is the zone wall-clock input is read in; Zone is the IANA name sent
	// with a new entry. Both default to the machine and exist for tests.
	Local *time.Location
	Zone  string
}

// New returns the CLI wired to the real filesystem, environment and clock.
func New(out, errOut io.Writer) *App {
	return &App{
		Out: out, Err: errOut, In: os.Stdin,
		NewClient: func(cfg client.Config) Client {
			c := client.New(cfg)
			c.SetOnTokenRefresh(func(newCfg client.Config) {
				_ = config.SaveClient(newCfg)
			})
			return c
		},
		LoadConfig: func(urlFlag, tokenFlag string) (client.Config, error) {
			return config.LoadClient(urlFlag, tokenFlag, os.Getenv)
		},
		SaveConfig: config.SaveClient,
		Now:        time.Now,
	}
}

// errNoURL is the one setup mistake every client command can hit.
var errNoURL = errors.New("no server URL; run tstamps login --url URL or set TSTAMPS_URL")

// Run dispatches one client subcommand and returns its exit code: 0 ok, 1
// error, 2 usage.
func (a *App) Run(args []string) int {
	if len(args) == 0 {
		return a.unknown("")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "login":
		return a.login(rest)
	case "whoami":
		return a.whoami(rest)
	case "start":
		return a.start(rest)
	case "stop":
		return a.stop(rest)
	case "add":
		return a.add(rest)
	case "ls":
		return a.ls(rest)
	case "edit":
		return a.edit(rest)
	case "rm":
		return a.rm(rest)
	case "projects":
		return a.projects(rest)
	case "teams":
		return a.teams(rest)
	case "report":
		return a.report(rest)
	case "tokens":
		return a.tokens(rest)
	case "audit":
		return a.audit(rest)
	}
	return a.unknown(cmd)
}

// unknown reports a command this package does not own. The full command list
// lives in cmd/tstamps, which prints it.
func (a *App) unknown(cmd string) int {
	if cmd == "" {
		fmt.Fprintln(a.Err, "tstamps: no command given; run tstamps help")
	} else {
		fmt.Fprintf(a.Err, "tstamps: unknown command %q; run tstamps help\n", cmd)
	}
	return 2
}

// newFlags returns a flag set that stays quiet: the command prints one line.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags accepts flags before or after the positional arguments, because
// every usage line puts the project or id first: `tstamps add Timeline --from`.
func parseFlags(fs *flag.FlagSet, args []string) error {
	return fs.Parse(reorder(fs, args))
}

// reorder moves positionals to the end, keeping flags and their values in order.
// The flag set must already have its flags registered.
func reorder(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue
		}
		f := fs.Lookup(name)
		if f == nil {
			continue // let Parse report the unknown flag
		}
		isBool, ok := f.Value.(interface{ IsBoolFlag() bool })
		if ok && isBool.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

// clientFlags registers the pair every client command accepts.
func clientFlags(fs *flag.FlagSet) (urlFlag, tokenFlag *string) {
	return fs.String("url", "", "server URL"), fs.String("token", "", "personal API token")
}

// connect loads the config and returns a client. The URL is required; the token
// may be empty for commands that work over a session, though the CLI never
// keeps one.
func (a *App) connect(urlFlag, tokenFlag string) (Client, error) {
	cfg, err := a.LoadConfig(urlFlag, tokenFlag)
	if err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, errNoURL
	}
	c := a.NewClient(cfg)
	if sc, ok := c.(interface{ SetOnTokenRefresh(func(client.Config)) }); ok {
		sc.SetOnTokenRefresh(func(newCfg client.Config) {
			_ = a.SaveConfig(newCfg)
		})
	}
	return c, nil
}

// fail prints one line and returns 1.
func (a *App) fail(cmd string, err error) int {
	fmt.Fprintf(a.Err, "tstamps %s: %v\n", cmd, err)
	return 1
}

// usageLine prints the command's usage and returns 2.
func (a *App) usageLine(line string) int {
	fmt.Fprintln(a.Err, "usage: "+line)
	return 2
}

func (a *App) flagError(cmd string, err error) int {
	fmt.Fprintf(a.Err, "tstamps %s: %v\n", cmd, err)
	return 2
}

// isSet reports whether the named flag appeared on the command line, so a
// pointer field can be cleared to zero on purpose.
func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func (a *App) checkNoArgs(cmd string, fs *flag.FlagSet) (int, bool) {
	if fs.NArg() != 0 {
		return a.usageLine("tstamps " + cmd), false
	}
	return 0, true
}
