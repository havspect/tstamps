package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendSendsAndReportsFailures(t *testing.T) {
	var got struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer re_key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer fake.Close()

	r := &Resend{Endpoint: fake.URL, Key: "re_key", From: "Tstamps <hello@example.com>"}
	if err := r.Send(context.Background(), "ada@example.com", "Verify", "link"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.From != r.From || len(got.To) != 1 || got.To[0] != "ada@example.com" || got.Subject != "Verify" || got.Text != "link" {
		t.Fatalf("payload = %+v", got)
	}

	bad := &Resend{Endpoint: fake.URL, Key: "wrong", From: r.From}
	if err := bad.Send(context.Background(), "ada@example.com", "Verify", "link"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Send(bad key) = %v, want a status error", err)
	}
}
