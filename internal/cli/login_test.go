package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/client"
)

func TestLoginDeviceFlowSuccess(t *testing.T) {
	f := &fakeClient{
		me: client.Me{ID: 1, Name: "Ada Lovelace", Email: "ada@example.com"},
		deviceAuth: &client.DeviceAuthResponse{
			DeviceCode:              "dc_12345",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "http://tstamps.test/auth/device",
			VerificationURIComplete: "http://tstamps.test/auth/device?user_code=ABCD-EFGH",
			Interval:                1,
		},
		tokenSet: &client.TokenSet{
			AccessToken:  "at_test_access",
			RefreshToken: "rt_test_refresh",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		},
	}
	a, out, errOut := newTestApp(f)
	var savedCfg client.Config
	a.SaveConfig = func(cfg client.Config) error {
		savedCfg = cfg
		return nil
	}

	code := a.Run([]string{"login", "--no-browser"})
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Open http://tstamps.test/auth/device and confirm code ABCD-EFGH") {
		t.Fatalf("stdout %q does not contain device instructions", out.String())
	}
	if !strings.Contains(out.String(), "logged in as Ada Lovelace <ada@example.com>") {
		t.Fatalf("stdout %q does not contain login confirmation", out.String())
	}
	if savedCfg.Token != "at_test_access" || savedCfg.RefreshToken != "rt_test_refresh" {
		t.Fatalf("savedCfg = %+v, want access and refresh tokens", savedCfg)
	}
}

func TestLoginDeviceFlowPollError(t *testing.T) {
	f := &fakeClient{
		deviceAuth: &client.DeviceAuthResponse{
			DeviceCode:      "dc_12345",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "http://tstamps.test/auth/device",
			Interval:        1,
		},
		pollErr: errors.New("authorization access denied"),
	}
	a, _, errOut := newTestApp(f)

	code := a.Run([]string{"login", "--no-browser"})
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "authorization access denied") {
		t.Fatalf("stderr %q does not contain polling error", errOut.String())
	}
}

func TestLoginPasswordFlagSuccess(t *testing.T) {
	f := &fakeClient{
		me:     client.Me{ID: 1, Name: "Ada Lovelace", Email: "ada@example.com"},
		minted: &client.Token{ID: 10, Name: "tstamps login", Token: "ts_personal_token"},
	}
	a, out, errOut := newTestApp(f)
	var savedCfg client.Config
	a.SaveConfig = func(cfg client.Config) error {
		savedCfg = cfg
		return nil
	}

	code := a.Run([]string{"login", "--email", "ada@example.com", "--password", "secret"})
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	if f.loginEmail != "ada@example.com" || f.loginPass != "secret" {
		t.Fatalf("credentials = %s:%s, want ada@example.com:secret", f.loginEmail, f.loginPass)
	}
	if savedCfg.Token != "ts_personal_token" || savedCfg.RefreshToken != "" {
		t.Fatalf("savedCfg = %+v, want personal token and empty refresh token", savedCfg)
	}
	if !strings.Contains(out.String(), "logged in as Ada Lovelace <ada@example.com>") {
		t.Fatalf("stdout %q does not contain login confirmation", out.String())
	}
}

func TestLoginMissingURL(t *testing.T) {
	f := &fakeClient{}
	a, _, errOut := newTestApp(f)
	a.LoadConfig = func(string, string) (client.Config, error) { return client.Config{}, nil }

	code := a.Run([]string{"login"})
	if code != 2 {
		t.Fatalf("exit %d, want 2 (usage)", code)
	}
	if !strings.Contains(errOut.String(), "usage: tstamps login") {
		t.Fatalf("stderr %q does not contain usage", errOut.String())
	}
}
