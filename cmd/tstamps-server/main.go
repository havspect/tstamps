// Command tstamps-server serves every workspace in one SQLite database: the web
// UI, the HTTP API, the database, plus the setup commands that run on the same
// host. Users talk to it through cmd/tstamps, the CLI.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	// Users pick an IANA timezone; the zoneinfo tree is not guaranteed to exist
	// on the hosts this ships to, so the database is embedded.
	_ "time/tzdata"

	"github.com/havspect/tstamps/internal/httpapi"
	"github.com/havspect/tstamps/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without the process, so tests drive subcommands directly.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "tstamps-server %s\n", version.Version)
		return 0
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	case "serve":
		return serve(args[1:], stderr)
	case "openapi":
		// The document goes to stdout; the request log would only pollute a pipe.
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		spec, err := httpapi.Spec()
		if err != nil {
			fmt.Fprintf(stderr, "tstamps-server openapi: %v\n", err)
			return 1
		}
		if _, err := stdout.Write(spec); err != nil {
			fmt.Fprintf(stderr, "tstamps-server openapi: %v\n", err)
			return 1
		}
		return 0
	case "backup":
		return backup(args[1:], stdout, stderr)
	case "workspace":
		return workspaceCmd(args[1:], stdout, stderr)
	}

	fmt.Fprintf(stderr, "tstamps-server: unknown command %q; run tstamps-server help\n", args[0])
	return 2
}

const usageText = `usage: tstamps-server <command> [flags]

  serve                      run the web UI and API (--addr, --db, --legal-dir, --log-level)
  workspace create           create a workspace and its first admin (slug, --email, --name, --db)
  backup <destination>       copy the database
  openapi                    print the OpenAPI document
  version                    print the version

User commands (login, entries, projects, teams, reports, mcp) are in the
tstamps CLI, which talks to a running server.

Exit codes: 0 ok, 1 error, 2 usage.
`

func usage(w io.Writer) {
	fmt.Fprint(w, usageText)
}
