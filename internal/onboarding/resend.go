package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultEmailEndpoint is Resend's send endpoint, used when no URL is configured.
const DefaultEmailEndpoint = "https://api.resend.com/emails"

// Mailer sends verification messages.
type Mailer interface {
	Send(ctx context.Context, to, subject, text string) error
}

// Resend posts a plain-text email to a Resend-compatible endpoint.
type Resend struct {
	Endpoint string
	Key      string
	From     string
	Client   *http.Client
}

// Send delivers one message and treats any non-2xx answer as a failure.
func (r *Resend) Send(ctx context.Context, to, subject, text string) error {
	body, err := json.Marshal(struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}{r.From, []string{to}, subject, text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.Key)
	req.Header.Set("Content-Type", "application/json")

	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("email endpoint answered %s: %s", resp.Status, bytes.TrimSpace(detail))
	}
	return nil
}
