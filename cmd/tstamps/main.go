// Command tstamps is the user CLI: it talks to a running server over the HTTP
// API and needs no database. The server and its admin commands are in
// cmd/tstamps-server.
package main

import (
	"fmt"
	"io"
	"os"
	// Users pick an IANA timezone; the zoneinfo tree is not guaranteed to exist
	// on the machines this ships to, so the database is embedded.
	_ "time/tzdata"

	"github.com/havspect/tstamps/internal/cli"
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
		fmt.Fprintf(stdout, "tstamps %s\n", version.Version)
		return 0
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	case "mcp":
		return mcpCommand(args[1:], stderr)
	case "serve", "admin", "openapi":
		// A stale unit file or an old README is how someone ends up here.
		fmt.Fprintf(stderr, "tstamps: %q is part of tstamps-server, not the CLI\n", args[0])
		return 2
	}

	return cli.New(stdout, stderr).Run(args)
}

const usageText = `usage: tstamps <command> [flags]

server commands are in tstamps-server (serve, admin, openapi).

client (needs a token from "tstamps login"):
  login --url URL            store a personal API token
  whoami                     show the current user
  start <project>            start the running entry (--note)
  stop                       stop the running entry (--note)
  add <project>              log a finished entry (--from, --to, --date, --note)
  ls                         list entries (--from, --to, --user, --json)
  edit <id>                  change an entry (--project, --from, --to, --note)
  rm <id>                    delete an entry
  projects                   list projects (--all, --json)
  projects add               create a project (--team, --color)
  projects archive <id>      archive a project
  teams                      list teams (--json)
  teams members <team>       list a team's members
  report                     hours totals (--from, --to, --by, --json, --csv)
  tokens create|ls|rm        manage personal API tokens
  audit                      role-based changes (--from, --to, --actor, --action, --json)
  mcp                        MCP server on stdio

  version                    print the version
  help                       print this list

Exit codes: 0 ok, 1 error, 2 usage.
`

func usage(w io.Writer) {
	fmt.Fprint(w, usageText)
}
