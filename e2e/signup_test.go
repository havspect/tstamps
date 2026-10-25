package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelfServiceSignup walks the real binary: an email address is verified
// through a stand-in Resend endpoint, then the link creates a workspace whose
// admin is signed in without a second login.
func TestSelfServiceSignup(t *testing.T) {
	dir := t.TempDir()
	serverBin := build(t, dir, "tstamps-server", "../cmd/tstamps-server")
	workspaceDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		t.Fatal(err)
	}

	var sentTo, sentLink string
	mail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer re_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			To   []string `json:"to"`
			Text string   `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sentTo = body.To[0]
		for _, field := range strings.Fields(body.Text) {
			if strings.Contains(field, "/signup/verify?token=") {
				sentLink = field
			}
		}
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer mail.Close()

	base, stop := startServe(t, serverBin,
		"serve", "--workspace-dir", workspaceDir, "--addr", "127.0.0.1:0",
		"--email-api-key", "re_test",
		"--email-api-url", mail.URL,
		"--email-from", "Tstamps <hello@example.com>",
		"--public-url", "https://time.example.com",
	)
	defer stop()
	post := func(path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", base)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		return resp
	}

	resp := post("/api/v1/signup", `{"email":"ada@example.com"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("start signup = %d, want 202", resp.StatusCode)
	}
	if sentTo != "ada@example.com" || sentLink == "" {
		t.Fatalf("mail = to %q link %q", sentTo, sentLink)
	}

	// No workspace exists until the link is used, even though the host runs.
	if _, err := os.Stat(filepath.Join(workspaceDir, "acme.db")); !os.IsNotExist(err) {
		t.Fatalf("workspace file before verification: %v", err)
	}

	link, err := url.Parse(sentLink)
	if err != nil {
		t.Fatal(err)
	}
	token := link.Query().Get("token")
	verify, err := http.Get(base + "/api/v1/signup/verify?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	verify.Body.Close()
	if verify.StatusCode != http.StatusOK {
		t.Fatalf("verify = %d, want 200", verify.StatusCode)
	}

	resp = post("/api/v1/signup/verify",
		`{"token":"`+token+`","slug":"acme","name":"Ada Lovelace","password":"correct horse"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("complete = %d, want 201", resp.StatusCode)
	}
	cookies := resp.Cookies()
	resp.Body.Close()
	if len(cookies) == 0 || cookies[0].Name != "tstamps_session" {
		t.Fatalf("cookies = %v, want a session", cookies)
	}

	me, err := http.NewRequest(http.MethodGet, base+"/w/acme/api/v1/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	me.AddCookie(cookies[0])
	meResp, err := http.DefaultClient.Do(me)
	if err != nil {
		t.Fatal(err)
	}
	defer meResp.Body.Close()
	var body struct {
		Email   string `json:"email"`
		IsAdmin bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if meResp.StatusCode != http.StatusOK || body.Email != "ada@example.com" || !body.IsAdmin {
		t.Fatalf("me = %d %+v, want the signed-in admin", meResp.StatusCode, body)
	}

	// The link is single use.
	again, err := http.Get(base + "/api/v1/signup/verify?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	again.Body.Close()
	if again.StatusCode != http.StatusNotFound {
		t.Fatalf("reused link = %d, want 404", again.StatusCode)
	}
}
