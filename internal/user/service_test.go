package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/user"
)

type mockClock struct {
	now time.Time
}

func (m mockClock) Now() time.Time {
	return m.now
}

type mockRepo struct {
	users               map[uint]*user.User
	sessions            map[string]*user.Session
	tokens              map[uint]*user.APIToken
	nextUserID          uint
	nextTokenID         uint
	revokedOAuthUsers   []uint
	deletedSessionUsers []uint
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:       make(map[uint]*user.User),
		sessions:    make(map[string]*user.Session),
		tokens:      make(map[uint]*user.APIToken),
		nextUserID:  1,
		nextTokenID: 1,
	}
}

func (m *mockRepo) GetByID(ctx context.Context, id uint) (*user.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	cpy := *u
	return &cpy, nil
}

func (m *mockRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			cpy := *u
			return &cpy, nil
		}
	}
	return nil, user.ErrNotFound
}

func (m *mockRepo) List(ctx context.Context) ([]user.User, error) {
	var out []user.User
	for _, u := range m.users {
		out = append(out, *u)
	}
	return out, nil
}

func (m *mockRepo) Create(ctx context.Context, u *user.User) error {
	u.ID = m.nextUserID
	m.nextUserID++
	cpy := *u
	m.users[u.ID] = &cpy
	return nil
}

func (m *mockRepo) Update(ctx context.Context, u *user.User) error {
	if _, ok := m.users[u.ID]; !ok {
		return user.ErrNotFound
	}
	cpy := *u
	m.users[u.ID] = &cpy
	return nil
}

func (m *mockRepo) CountActiveAdmins(ctx context.Context) (int, error) {
	var count int
	for _, u := range m.users {
		if u.IsAdmin && u.Active {
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) SaveSession(ctx context.Context, s *user.Session) error {
	m.sessions[s.TokenHash] = s
	return nil
}

func (m *mockRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*user.Session, error) {
	s, ok := m.sessions[tokenHash]
	if !ok {
		return nil, user.ErrNotFound
	}
	return s, nil
}

func (m *mockRepo) DeleteSession(ctx context.Context, tokenHash string) error {
	delete(m.sessions, tokenHash)
	return nil
}

func (m *mockRepo) DeleteUserSessionsExcept(ctx context.Context, userID uint, keepTokenHash string) error {
	m.deletedSessionUsers = append(m.deletedSessionUsers, userID)
	for hash, s := range m.sessions {
		if s.UserID == userID && hash != keepTokenHash {
			delete(m.sessions, hash)
		}
	}
	return nil
}

func (m *mockRepo) CreateAPIToken(ctx context.Context, t *user.APIToken) error {
	t.ID = m.nextTokenID
	m.nextTokenID++
	cpy := *t
	m.tokens[t.ID] = &cpy
	return nil
}

func (m *mockRepo) GetAPITokenByTokenHash(ctx context.Context, tokenHash string) (*user.APIToken, error) {
	for _, tok := range m.tokens {
		if tok.TokenHash == tokenHash && tok.RevokedAt == nil {
			return tok, nil
		}
	}
	return nil, user.ErrNotFound
}

func (m *mockRepo) TouchAPIToken(ctx context.Context, id uint, at time.Time) error {
	tok, ok := m.tokens[id]
	if !ok {
		return user.ErrNotFound
	}
	tok.LastUsedAt = &at
	return nil
}

func (m *mockRepo) RevokeAPIToken(ctx context.Context, id uint, userID uint) error {
	t, ok := m.tokens[id]
	if !ok || t.UserID != userID {
		return user.ErrNotFound
	}
	now := time.Now()
	t.RevokedAt = &now
	return nil
}

func (m *mockRepo) ListAPITokens(ctx context.Context, userID uint) ([]user.APIToken, error) {
	var out []user.APIToken
	for _, t := range m.tokens {
		if t.UserID == userID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (m *mockRepo) RevokeAllOAuthTokens(ctx context.Context, userID uint) error {
	m.revokedOAuthUsers = append(m.revokedOAuthUsers, userID)
	return nil
}

type mockAuditor struct {
	events []string
}

func (a *mockAuditor) Record(ctx context.Context, action, targetType string, targetID uint, changes any) error {
	a.events = append(a.events, action)
	return nil
}

type mockTimerStopper struct {
	stoppedUsers []uint
	stoppedNotes []string
}

func (s *mockTimerStopper) StopRunningTimer(ctx context.Context, userID uint, note string) error {
	s.stoppedUsers = append(s.stoppedUsers, userID)
	s.stoppedNotes = append(s.stoppedNotes, note)
	return nil
}

func adminCtx() context.Context {
	return auth.WithPrincipal(context.Background(), &auth.Principal{
		WorkspaceID: 1,
		UserID:      1,
		Roles:       []string{"admin"},
		TokenID:     "sess_admin",
	})
}

func userCtx(userID uint) context.Context {
	return auth.WithPrincipal(context.Background(), &auth.Principal{
		WorkspaceID: 1,
		UserID:      userID,
		Roles:       []string{"member"},
		TokenID:     "sess_user",
		LedTeamIDs:  []uint{10, 20},
	})
}

func TestCreateUser(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	svc := user.New(repo, auditor, nil, mockClock{now: now})

	t.Run("admin authorization and input validation", func(t *testing.T) {
		// Unauthorized without principal
		if _, _, err := svc.CreateUser(context.Background(), user.CreateInput{Email: "a@b.com", Name: "A"}); !errors.Is(err, authz.ErrUnauthorized) {
			t.Errorf("expected ErrUnauthorized, got %v", err)
		}

		// Forbidden with regular user
		if _, _, err := svc.CreateUser(userCtx(2), user.CreateInput{Email: "a@b.com", Name: "A"}); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}

		// Empty email
		if _, _, err := svc.CreateUser(adminCtx(), user.CreateInput{Email: "", Name: "A"}); !errors.Is(err, user.ErrInvalidEmail) {
			t.Errorf("expected ErrInvalidEmail, got %v", err)
		}

		// Empty name
		if _, _, err := svc.CreateUser(adminCtx(), user.CreateInput{Email: "a@b.com", Name: "   "}); !errors.Is(err, user.ErrInvalidName) {
			t.Errorf("expected ErrInvalidName, got %v", err)
		}
	})

	t.Run("success creates user and audits", func(t *testing.T) {
		u, rawPass, err := svc.CreateUser(adminCtx(), user.CreateInput{
			Email:    "Alice@Example.COM",
			Name:     "Alice Smith",
			Timezone: "America/New_York",
			IsAdmin:  false,
		})
		if err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}
		if rawPass == "" {
			t.Fatal("expected non-empty generated password")
		}
		if !auth.VerifyPassword(u.PasswordHash, rawPass) {
			t.Errorf("password verification failed against generated password")
		}
		if u.Email != "alice@example.com" {
			t.Errorf("expected lowercased email, got %s", u.Email)
		}
		if !u.MustChangePassword {
			t.Errorf("expected must_change_password=true")
		}
		if !u.Active {
			t.Errorf("expected active=true")
		}
		if auditor.events[len(auditor.events)-1] != "user.create" {
			t.Errorf("expected user.create audit event, got %v", auditor.events)
		}

		// Duplicate email returns ErrDuplicateEmail
		_, _, err = svc.CreateUser(adminCtx(), user.CreateInput{
			Email: "alice@example.com",
			Name:  "Another Alice",
		})
		if !errors.Is(err, user.ErrDuplicateEmail) {
			t.Errorf("expected ErrDuplicateEmail, got %v", err)
		}
	})
}

func TestUpdateUser(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	stopper := &mockTimerStopper{}
	svc := user.New(repo, auditor, stopper, mockClock{now: now})

	// Create initial admin
	admin, _, _ := svc.CreateUser(adminCtx(), user.CreateInput{
		Email:   "admin@example.com",
		Name:    "Admin",
		IsAdmin: true,
	})

	// Create user 2
	u2, _, _ := svc.CreateUser(adminCtx(), user.CreateInput{
		Email:   "user2@example.com",
		Name:    "User Two",
		IsAdmin: false,
	})

	t.Run("last active admin protection", func(t *testing.T) {
		isNotAdmin := false
		// Demoting the only admin must fail
		_, err := svc.UpdateUser(adminCtx(), user.UpdateInput{
			ID:      admin.ID,
			IsAdmin: &isNotAdmin,
		})
		if !errors.Is(err, user.ErrLastAdmin) {
			t.Errorf("expected ErrLastAdmin on demoting last admin, got %v", err)
		}

		// Deactivating the only admin must fail
		isActive := false
		_, err = svc.UpdateUser(adminCtx(), user.UpdateInput{
			ID:     admin.ID,
			Active: &isActive,
		})
		if !errors.Is(err, user.ErrLastAdmin) {
			t.Errorf("expected ErrLastAdmin on deactivating last admin, got %v", err)
		}
	})

	t.Run("deactivation cascades to timer stopper and oauth revocation", func(t *testing.T) {
		isActive := false
		updated, err := svc.UpdateUser(adminCtx(), user.UpdateInput{
			ID:     u2.ID,
			Active: &isActive,
		})
		if err != nil {
			t.Fatalf("UpdateUser deactivation failed: %v", err)
		}
		if updated.Active {
			t.Errorf("expected active=false")
		}
		if len(stopper.stoppedUsers) != 1 || stopper.stoppedUsers[0] != u2.ID {
			t.Errorf("expected timer stopped for user %d, got %v", u2.ID, stopper.stoppedUsers)
		}
		if len(stopper.stoppedNotes) != 1 || stopper.stoppedNotes[0] != "stopped on deactivation" {
			t.Errorf("expected 'stopped on deactivation' note, got %v", stopper.stoppedNotes)
		}
		if len(repo.revokedOAuthUsers) != 1 || repo.revokedOAuthUsers[0] != u2.ID {
			t.Errorf("expected OAuth tokens revoked for user %d, got %v", u2.ID, repo.revokedOAuthUsers)
		}
		if auditor.events[len(auditor.events)-1] != "user.update" {
			t.Errorf("expected user.update audit, got %v", auditor.events)
		}
	})
}

func TestResetPassword(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	svc := user.New(repo, auditor, nil, mockClock{now: now})

	u, _, _ := svc.CreateUser(adminCtx(), user.CreateInput{
		Email: "target@example.com",
		Name:  "Target User",
	})

	newPass, err := svc.ResetPassword(adminCtx(), u.ID)
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}
	if newPass == "" {
		t.Fatal("expected new generated password")
	}

	updated, _ := repo.GetByID(context.Background(), u.ID)
	if !auth.VerifyPassword(updated.PasswordHash, newPass) {
		t.Errorf("updated password hash did not verify against reset password")
	}
	if !updated.MustChangePassword {
		t.Errorf("expected must_change_password=true after reset")
	}
	if len(repo.revokedOAuthUsers) != 1 || repo.revokedOAuthUsers[0] != u.ID {
		t.Errorf("expected OAuth revocation for user %d", u.ID)
	}
	if auditor.events[len(auditor.events)-1] != "user.password.reset" {
		t.Errorf("expected user.password.reset audit event, got %v", auditor.events)
	}
}

func TestChangeOwnPassword(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	svc := user.New(repo, auditor, nil, mockClock{now: now})

	u, rawPass, _ := svc.CreateUser(adminCtx(), user.CreateInput{
		Email: "self@example.com",
		Name:  "Self User",
	})

	ctx := userCtx(u.ID)

	t.Run("wrong current password", func(t *testing.T) {
		err := svc.ChangeOwnPassword(ctx, "wrong-pass", "new-secret-password-123")
		if !errors.Is(err, user.ErrInvalidPassword) {
			t.Errorf("expected ErrInvalidPassword, got %v", err)
		}
	})

	t.Run("too short new password", func(t *testing.T) {
		err := svc.ChangeOwnPassword(ctx, rawPass, "short")
		if !errors.Is(err, user.ErrPasswordTooShort) {
			t.Errorf("expected ErrPasswordTooShort, got %v", err)
		}
	})

	t.Run("success changes password and invalidates other sessions", func(t *testing.T) {
		err := svc.ChangeOwnPassword(ctx, rawPass, "valid-new-password-123")
		if err != nil {
			t.Fatalf("ChangeOwnPassword failed: %v", err)
		}
		updated, _ := repo.GetByID(ctx, u.ID)
		if !auth.VerifyPassword(updated.PasswordHash, "valid-new-password-123") {
			t.Errorf("password verification failed after change")
		}
		if updated.MustChangePassword {
			t.Errorf("expected must_change_password=false")
		}
		if len(repo.deletedSessionUsers) == 0 {
			t.Errorf("expected other sessions deleted")
		}
		if auditor.events[len(auditor.events)-1] != "user.password.change" {
			t.Errorf("expected user.password.change audit, got %v", auditor.events)
		}
	})
}

func TestProfileAndList(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	svc := user.New(repo, nil, nil, mockClock{now: now})

	u, _, _ := svc.CreateUser(adminCtx(), user.CreateInput{
		Email: "profile@example.com",
		Name:  "Profile User",
	})

	t.Run("GetProfile returns profile and led teams", func(t *testing.T) {
		prof, err := svc.GetProfile(userCtx(u.ID))
		if err != nil {
			t.Fatalf("GetProfile failed: %v", err)
		}
		if prof.User.ID != u.ID {
			t.Errorf("expected user ID %d, got %d", u.ID, prof.User.ID)
		}
		if len(prof.LedTeamIDs) != 2 || prof.LedTeamIDs[0] != 10 {
			t.Errorf("unexpected led team IDs: %v", prof.LedTeamIDs)
		}
	})

	t.Run("ListUsers requires admin", func(t *testing.T) {
		if _, err := svc.ListUsers(userCtx(u.ID)); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("expected ErrForbidden for non-admin, got %v", err)
		}
		users, err := svc.ListUsers(adminCtx())
		if err != nil || len(users) != 1 {
			t.Errorf("ListUsers failed: err=%v, count=%d", err, len(users))
		}
	})
}

func TestPersonalTokens(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	svc := user.New(repo, auditor, nil, mockClock{now: now})

	ctx := userCtx(42)

	t.Run("CreatePersonalToken validation and creation", func(t *testing.T) {
		if _, _, err := svc.CreatePersonalToken(ctx, "   "); !errors.Is(err, user.ErrInvalidTokenName) {
			t.Errorf("expected ErrInvalidTokenName, got %v", err)
		}

		tok, raw, err := svc.CreatePersonalToken(ctx, "ci-runner")
		if err != nil {
			t.Fatalf("CreatePersonalToken failed: %v", err)
		}
		if raw == "" || tok.ID == 0 {
			t.Errorf("unexpected token output: tok=%+v, raw=%s", tok, raw)
		}
		if tok.UserID != 42 {
			t.Errorf("expected token UserID=42, got %d", tok.UserID)
		}
		if auditor.events[len(auditor.events)-1] != "token.create" {
			t.Errorf("expected token.create audit, got %v", auditor.events)
		}

		// List personal tokens
		list, err := svc.ListPersonalTokens(ctx)
		if err != nil || len(list) != 1 {
			t.Errorf("ListPersonalTokens failed: err=%v, count=%d", err, len(list))
		}

		// Revoke personal token
		err = svc.RevokePersonalToken(ctx, tok.ID)
		if err != nil {
			t.Fatalf("RevokePersonalToken failed: %v", err)
		}
		if repo.tokens[tok.ID].RevokedAt == nil {
			t.Errorf("expected token to be marked revoked")
		}
		if auditor.events[len(auditor.events)-1] != "token.revoke" {
			t.Errorf("expected token.revoke audit, got %v", auditor.events)
		}
	})
}
