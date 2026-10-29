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
				"--email-api-key", "re_key",
				"--email-from", "Tstamps <hello@example.com>",
				"--public-url", "https://time.example.com/",
			},
			want: Config{
				Addr: ":8080", DB: "./tstamps.db",
				EmailAPIKey: "re_key", EmailFrom: "Tstamps <hello@example.com>",
				PublicURL: "https://time.example.com",
			},
		},
		{
			name:    "signup partial is refused",
			args:    []string{"--email-api-key", "re_key"},
			wantErr: true,
		},
		{
			name: "signup public url must be absolute",
			args: []string{
				"--email-api-key", "re_key", "--email-from", "x@example.com", "--public-url", "/",
			},
			wantErr: true,
		},
		{
			name: "demo from flag",
			args: []string{"--demo"},
			want: Config{Addr: ":8080", DB: "./tstamps.db", Demo: true},
		},
		{
			name: "demo from env",
			env:  map[string]string{"TSTAMPS_DEMO": "true"},
			want: Config{Addr: ":8080", DB: "./tstamps.db", Demo: true},
		},
		{
			name:    "invalid demo env",
			env:     map[string]string{"TSTAMPS_DEMO": "yes please"},
			wantErr: true,
		},
		{
			name:    "demo cannot sign up",
			args:    []string{"--demo", "--email-api-key", "re_key"},
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
	full := Config{DB: "x.db", EmailAPIKey: "k", EmailFrom: "f@example.com", PublicURL: "https://x.example"}
	if !full.SignupEnabled() {
		t.Fatal("a server with all email settings must offer signup")
	}
	for _, tt := range []struct {
		name string
		cfg  Config
	}{
		{"no key", Config{DB: "x.db", EmailFrom: "f@example.com", PublicURL: "https://x.example"}},
		{"no from", Config{DB: "x.db", EmailAPIKey: "k", PublicURL: "https://x.example"}},
		{"no public url", Config{DB: "x.db", EmailAPIKey: "k", EmailFrom: "f@example.com"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg.SignupEnabled() {
				t.Fatal("signup enabled without the full email path")
			}
		})
	}
}
