package config

import (
	"log/slog"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "defaults",
			want: Config{Addr: ":8080", DB: "./tstamps.db"},
		},
		{
			name: "env only",
			env:  map[string]string{"TSTAMPS_ADDR": "127.0.0.1:9000", "TSTAMPS_DB": "/srv/tstamps.db"},
			want: Config{Addr: "127.0.0.1:9000", DB: "/srv/tstamps.db"},
		},
		{
			name: "flags only",
			args: []string{"--addr", ":7000", "--db", "flags.db"},
			want: Config{Addr: ":7000", DB: "flags.db"},
		},
		{
			name: "flags beat env",
			args: []string{"--db", "flags.db"},
			env:  map[string]string{"TSTAMPS_ADDR": "127.0.0.1:9000", "TSTAMPS_DB": "/srv/tstamps.db"},
			want: Config{Addr: "127.0.0.1:9000", DB: "flags.db"},
		},
		{
			name: "hosted from env",
			env:  map[string]string{"TSTAMPS_WORKSPACE_DIR": "/srv/workspaces"},
			want: Config{Addr: ":8080", WorkspaceDir: "/srv/workspaces"},
		},
		{
			name: "hosted from flag",
			args: []string{"--workspace-dir", "workspaces"},
			want: Config{Addr: ":8080", WorkspaceDir: "workspaces"},
		},
		{
			name:    "hosted and env db",
			env:     map[string]string{"TSTAMPS_WORKSPACE_DIR": "/srv/workspaces", "TSTAMPS_DB": "/srv/one.db"},
			wantErr: true,
		},
		{
			name:    "hosted and db flag",
			args:    []string{"--workspace-dir", "workspaces", "--db", "one.db"},
			wantErr: true,
		},
		{
			name:    "unknown flag",
			args:    []string{"--nope"},
			wantErr: true,
		},
		{
			name: "log level from env",
			env:  map[string]string{"TSTAMPS_LOG_LEVEL": "debug"},
			want: Config{Addr: ":8080", DB: "./tstamps.db", LogLevel: slog.LevelDebug},
		},
		{
			name: "log level from flag",
			args: []string{"--log-level", "warn"},
			want: Config{Addr: ":8080", DB: "./tstamps.db", LogLevel: slog.LevelWarn},
		},
		{
			name: "signup configured",
			args: []string{
				"--workspace-dir", "workspaces",
				"--email-api-key", "re_key",
				"--email-from", "Tstamps <hello@example.com>",
				"--public-url", "https://time.example.com/",
			},
			want: Config{
				Addr: ":8080", WorkspaceDir: "workspaces",
				EmailAPIKey: "re_key", EmailFrom: "Tstamps <hello@example.com>",
				PublicURL: "https://time.example.com",
			},
		},
		{
			name:    "signup partial is refused",
			args:    []string{"--workspace-dir", "workspaces", "--email-api-key", "re_key"},
			wantErr: true,
		},
		{
			name: "signup public url must be absolute",
			args: []string{
				"--workspace-dir", "workspaces",
				"--email-api-key", "re_key", "--email-from", "x@example.com", "--public-url", "/",
			},
			wantErr: true,
		},
		{
			name:    "invalid log level",
			env:     map[string]string{"TSTAMPS_LOG_LEVEL": "loud"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(tt.args, func(key string) string { return tt.env[key] })
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load(%q) = %+v, want error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(%q): %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("Load(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestSignupEnabled(t *testing.T) {
	full := Config{WorkspaceDir: "ws", EmailAPIKey: "k", EmailFrom: "f@example.com", PublicURL: "https://x.example"}
	if !full.SignupEnabled() {
		t.Fatal("a hosted server with all email settings must offer signup")
	}
	for _, tt := range []struct {
		name string
		cfg  Config
	}{
		{"one workspace mode", Config{DB: "x.db", EmailAPIKey: "k", EmailFrom: "f@example.com", PublicURL: "https://x.example"}},
		{"no key", Config{WorkspaceDir: "ws", EmailFrom: "f@example.com", PublicURL: "https://x.example"}},
		{"no from", Config{WorkspaceDir: "ws", EmailAPIKey: "k", PublicURL: "https://x.example"}},
		{"no public url", Config{WorkspaceDir: "ws", EmailAPIKey: "k", EmailFrom: "f@example.com"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg.SignupEnabled() {
				t.Fatal("signup enabled without the full email path")
			}
		})
	}
}
