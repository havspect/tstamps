package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/store"
)

func TestRemoteMCPServer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tstamps.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	clk := clock.System{}
	srv := &Server{Store: st, Clock: clk, entries: entry.New(st, clk)}

	user := &store.User{
		Email:    "ada@example.com",
		Name:     "Ada Lovelace",
		Timezone: "Europe/London",
		IsAdmin:  true,
		Active:   true,
	}
	if err := st.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	team := &store.Team{Name: "Platform"}
	if err := st.CreateTeam(team); err != nil {
		t.Fatalf("create team: %v", err)
	}
	proj := &store.Project{TeamID: team.ID, Name: "Timeline"}
	if err := st.CreateProject(proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// 1. Read-only scope context
	readOnlyCtx := context.WithValue(context.Background(), requestKey{}, &requestInfo{
		User:   user,
		Scopes: []string{"tstamps.read"},
	})

	mcpSrv := srv.newRemoteMCPServer(readOnlyCtx)
	t1, t2 := sdk.NewInMemoryTransports()
	ss, err := mcpSrv.Connect(readOnlyCtx, t1, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer ss.Close()

	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(readOnlyCtx, t2, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer cs.Close()

	// Call whoami
	res, err := cs.CallTool(readOnlyCtx, &sdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("whoami call error: %v", err)
	}
	if res.IsError {
		t.Fatalf("whoami returned tool error: %s", mcpContentText(res))
	}
	if body := mcpContentText(res); !strings.Contains(body, "ada@example.com") {
		t.Fatalf("expected ada@example.com in whoami output, got: %s", body)
	}

	// Call start_timer with read-only scope -> should fail with insufficient scope
	res, err = cs.CallTool(readOnlyCtx, &sdk.CallToolParams{
		Name:      "start_timer",
		Arguments: map[string]any{"project_id": proj.ID},
	})
	if err != nil {
		t.Fatalf("start_timer call error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("start_timer succeeded with read-only scope, want refusal")
	}
	if body := mcpContentText(res); !strings.Contains(body, "insufficient scope") {
		t.Fatalf("expected insufficient scope error, got: %s", body)
	}

	_ = cs.Close()
	_ = ss.Close()

	// 2. Read-write scope context
	writeCtx := context.WithValue(context.Background(), requestKey{}, &requestInfo{
		User:   user,
		Scopes: []string{"tstamps.read", "tstamps.write"},
	})

	mcpWriteSrv := srv.newRemoteMCPServer(writeCtx)
	t3, t4 := sdk.NewInMemoryTransports()
	ssWrite, err := mcpWriteSrv.Connect(writeCtx, t3, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer ssWrite.Close()

	csWrite, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(writeCtx, t4, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer csWrite.Close()

	// Call start_timer with write scope -> should succeed and fallback to user timezone Europe/London
	res, err = csWrite.CallTool(writeCtx, &sdk.CallToolParams{
		Name:      "start_timer",
		Arguments: map[string]any{"project_id": proj.ID, "note": "working on mcp"},
	})
	if err != nil {
		t.Fatalf("start_timer call error: %v", err)
	}
	if res.IsError {
		t.Fatalf("start_timer failed: %s", mcpContentText(res))
	}
	body := mcpContentText(res)
	if !strings.Contains(body, "Europe/London") {
		t.Fatalf("expected user timezone Europe/London in timer output, got: %s", body)
	}

	// Call stop_timer
	res, err = csWrite.CallTool(writeCtx, &sdk.CallToolParams{
		Name:      "stop_timer",
		Arguments: map[string]any{"note": "done working on mcp"},
	})
	if err != nil {
		t.Fatalf("stop_timer call error: %v", err)
	}
	if res.IsError {
		t.Fatalf("stop_timer failed: %s", mcpContentText(res))
	}

	// Call list_entries
	res, err = csWrite.CallTool(writeCtx, &sdk.CallToolParams{
		Name:      "list_entries",
		Arguments: map[string]any{"project_id": proj.ID},
	})
	if err != nil {
		t.Fatalf("list_entries call error: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_entries failed: %s", mcpContentText(res))
	}
	if body := mcpContentText(res); !strings.Contains(body, "done working on mcp") {
		t.Fatalf("expected logged note in list_entries, got: %s", body)
	}
}

func mcpContentText(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*sdk.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		raw, _ := json.Marshal(res.StructuredContent)
		b.Write(raw)
	}
	return b.String()
}
