package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRecordAuditRoundTrip(t *testing.T) {
	s := openTestStore(t)
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	tokenID := uint(7)

	err := s.RecordAudit(at, AuditActor{UserID: 3, TokenID: &tokenID, Role: "admin"},
		"project.archive", "project", 42,
		Change{Before: map[string]any{"archived_at": nil}, After: map[string]any{"archived_at": at}})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	events, err := s.AuditEvents(AuditFilter{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if !e.At.Equal(at) || e.ActorUserID != 3 || e.ActorTokenID == nil || *e.ActorTokenID != tokenID ||
		e.ActorRole != "admin" || e.Action != "project.archive" ||
		e.TargetType != "project" || e.TargetID != 42 {
		t.Fatalf("event round trip mismatch: %+v", e)
	}

	var c Change
	if err := json.Unmarshal([]byte(e.Changes), &c); err != nil {
		t.Fatalf("changes not JSON: %v", err)
	}
	if c.Before["archived_at"] != nil {
		t.Errorf("before.archived_at = %v, want null", c.Before["archived_at"])
	}
	if got, want := c.After["archived_at"], at.Format(time.RFC3339); got != want {
		t.Errorf("after.archived_at = %v, want %q", got, want)
	}
}

func TestInTxRollsBackMutationAndEvent(t *testing.T) {
	s := openTestStore(t)
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	boom := errors.New("boom")

	err := s.InTx(func(tx *Store) error {
		if err := tx.CreateUser(&User{Email: "ada@example.com", Name: "Ada", PasswordHash: "x"}); err != nil {
			return err
		}
		if err := tx.RecordAudit(at, AuditActor{UserID: 1, Role: "admin"}, "user.create", "user", 1,
			Change{After: map[string]any{"email": "ada@example.com"}}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx = %v, want boom", err)
	}
	if _, err := s.UserByEmail("ada@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("user after rollback = %v, want ErrNotFound", err)
	}
	if events, err := s.AuditEvents(AuditFilter{}); err != nil || len(events) != 0 {
		t.Fatalf("events after rollback = %d, %v, want none", len(events), err)
	}
}

func TestInTxNestedTransactionRollsBackAndCommitsBoth(t *testing.T) {
	s := openTestStore(t)
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	u := &User{Email: "grace@example.com", Name: "Grace", PasswordHash: "x"}
	if err := s.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// SaveUser opens its own transaction; inside InTx it is a savepoint.
	write := func(tx *Store) error {
		changed := *u
		changed.Name = "Grace Hopper"
		if err := tx.SaveUser(&changed, at); err != nil {
			return err
		}
		return tx.RecordAudit(at, AuditActor{UserID: u.ID, Role: "admin"}, "user.update", "user", u.ID,
			Change{Before: u.AuditFields(), After: changed.AuditFields()})
	}

	rollback := errors.New("rollback")
	if err := s.InTx(func(tx *Store) error {
		if err := write(tx); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("InTx = %v, want rollback", err)
	}
	got, err := s.UserByID(u.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Name != "Grace" {
		t.Errorf("name after rollback = %q, want Grace", got.Name)
	}
	if events, err := s.AuditEvents(AuditFilter{}); err != nil || len(events) != 0 {
		t.Fatalf("events after rollback = %d, %v, want none", len(events), err)
	}

	if err := s.InTx(write); err != nil {
		t.Fatalf("InTx: %v", err)
	}
	got, err = s.UserByID(u.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Name != "Grace Hopper" {
		t.Errorf("name after commit = %q, want Grace Hopper", got.Name)
	}
	events, err := s.AuditEvents(AuditFilter{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Action != "user.update" {
		t.Fatalf("events after commit = %+v, want one user.update", events)
	}
}

func TestAuditFieldsExcludePasswordHash(t *testing.T) {
	const hash = "sha256:super-secret-hash"
	u := User{
		ID: 1, Email: "ada@example.com", Name: "Ada", PasswordHash: hash,
		Timezone: "UTC", Active: true,
	}
	raw, err := json.Marshal(u.AuditFields())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(raw)
	if strings.Contains(js, hash) {
		t.Errorf("audited fields contain the password hash: %s", js)
	}
	if strings.Contains(js, "password_hash") {
		t.Errorf("audited fields contain the key password_hash: %s", js)
	}
	if !strings.Contains(js, "ada@example.com") {
		t.Errorf("audited fields lost the email: %s", js)
	}
}

func TestAuditEventsFiltersOrderAndLimit(t *testing.T) {
	s := openTestStore(t)
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

	type event struct {
		minutes    int
		actor      uint
		action     string
		targetType string
		targetID   uint
	}
	seed := []event{
		{0, 1, "user.update", "user", 10},
		{1, 1, "project.archive", "project", 20},
		{2, 2, "user.update", "user", 20},
		{3, 2, "project.archive", "project", 10},
		{4, 1, "team.archive", "team", 5},
	}
	for _, e := range seed {
		err := s.RecordAudit(base.Add(time.Duration(e.minutes)*time.Minute),
			AuditActor{UserID: e.actor, Role: "admin"}, e.action, e.targetType, e.targetID, Change{})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	tests := []struct {
		name string
		f    AuditFilter
		want []uint // target ids, newest first
	}{
		{name: "any", f: AuditFilter{}, want: []uint{5, 10, 20, 20, 10}},
		{name: "actor", f: AuditFilter{ActorUserID: 2}, want: []uint{10, 20}},
		{name: "action", f: AuditFilter{Action: "user.update"}, want: []uint{20, 10}},
		{name: "target type", f: AuditFilter{TargetType: "project"}, want: []uint{10, 20}},
		{name: "target", f: AuditFilter{TargetType: "project", TargetID: 10}, want: []uint{10}},
		{name: "range", f: AuditFilter{From: base.Add(2 * time.Minute), To: base.Add(4 * time.Minute)}, want: []uint{10, 20}},
		{name: "limit", f: AuditFilter{Limit: 2}, want: []uint{5, 10}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events, err := s.AuditEvents(tc.f)
			if err != nil {
				t.Fatalf("events: %v", err)
			}
			got := make([]uint, len(events))
			for i, e := range events {
				got[i] = e.TargetID
			}
			if len(got) != len(tc.want) {
				t.Fatalf("targets = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("targets = %v, want %v (newest first)", got, tc.want)
				}
			}
		})
	}
}

func TestMigrateRecordsAuditSchemaVersion(t *testing.T) {
	s := openTestStore(t)
	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, SchemaVersion)
	}
}
