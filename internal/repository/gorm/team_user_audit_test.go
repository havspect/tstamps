package gorm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
)

func TestAuditRepository(t *testing.T) {
	db := newTestDB(t)
	repo := NewAuditRepository(db)

	ctxWs1 := WithWorkspaceID(context.Background(), 1)
	ctxWs2 := WithWorkspaceID(context.Background(), 2)

	now := time.Now().Truncate(time.Second)

	tokenID := uint(42)
	evt1 := &audit.AuditEvent{
		At:            now.Add(-10 * time.Minute),
		ActorUserID:   1,
		ActorTokenID:  &tokenID,
		ActorClientID: "cli",
		ActorRole:     "admin",
		Action:        "team.create",
		TargetType:    "team",
		TargetID:      10,
		Changes:       `{"name":"Backend"}`,
		CreatedAt:     now.Add(-10 * time.Minute),
	}

	if err := repo.Record(ctxWs1, evt1); err != nil {
		t.Fatalf("Record evt1 in ws1: %v", err)
	}
	if evt1.ID == 0 {
		t.Errorf("expected evt1.ID to be populated")
	}

	evt2 := &audit.AuditEvent{
		At:          now.Add(-5 * time.Minute),
		ActorUserID: 2,
		ActorRole:   "admin",
		Action:      "team.rename",
		TargetType:  "team",
		TargetID:    10,
		Changes:     `{"name":"Core Backend"}`,
		CreatedAt:   now.Add(-5 * time.Minute),
	}
	if err := repo.Record(ctxWs1, evt2); err != nil {
		t.Fatalf("Record evt2 in ws1: %v", err)
	}

	evtWs2 := &audit.AuditEvent{
		At:          now,
		ActorUserID: 99,
		ActorRole:   "admin",
		Action:      "team.create",
		TargetType:  "team",
		TargetID:    20,
		Changes:     `{"name":"Other"}`,
		CreatedAt:   now,
	}
	if err := repo.Record(ctxWs2, evtWs2); err != nil {
		t.Fatalf("Record evtWs2 in ws2: %v", err)
	}

	t.Run("query ws1 events", func(t *testing.T) {
		events, err := repo.Query(ctxWs1, audit.QueryFilter{})
		if err != nil {
			t.Fatalf("Query ws1: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events in ws1, got %d", len(events))
		}
		// Ordered newest first
		if events[0].ID != evt2.ID || events[1].ID != evt1.ID {
			t.Errorf("expected descending order by at/id, got IDs [%d, %d]", events[0].ID, events[1].ID)
		}
		if events[1].ActorTokenID == nil || *events[1].ActorTokenID != 42 {
			t.Errorf("expected ActorTokenID 42, got %v", events[1].ActorTokenID)
		}
	})

	t.Run("query ws2 events isolated from ws1", func(t *testing.T) {
		events, err := repo.Query(ctxWs2, audit.QueryFilter{})
		if err != nil {
			t.Fatalf("Query ws2: %v", err)
		}
		if len(events) != 1 || events[0].ID != evtWs2.ID {
			t.Errorf("expected only evtWs2 in ws2, got %+v", events)
		}
	})

	t.Run("filter by action and target", func(t *testing.T) {
		events, err := repo.Query(ctxWs1, audit.QueryFilter{Action: "team.rename"})
		if err != nil {
			t.Fatalf("Query with action filter: %v", err)
		}
		if len(events) != 1 || events[0].Action != "team.rename" {
			t.Errorf("expected 1 team.rename event, got %+v", events)
		}

		events, err = repo.Query(ctxWs1, audit.QueryFilter{
			TargetType: "team",
			TargetID:   10,
		})
		if err != nil {
			t.Fatalf("Query with target filter: %v", err)
		}
		if len(events) != 2 {
			t.Errorf("expected 2 events for target team:10, got %d", len(events))
		}
	})
}

func TestTeamRepository(t *testing.T) {
	db := newTestDB(t)
	teamRepo := NewTeamRepository(db)
	userRepo := NewUserRepository(db)

	ctxWs1 := WithWorkspaceID(context.Background(), 1)
	ctxWs2 := WithWorkspaceID(context.Background(), 2)

	now := time.Now().Truncate(time.Second)

	// Create users in ws1
	u1 := &user.User{Email: "alice@example.com", Name: "Alice", PasswordHash: "x", Active: true}
	u2 := &user.User{Email: "bob@example.com", Name: "Bob", PasswordHash: "x", Active: true}
	if err := userRepo.Create(ctxWs1, u1); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := userRepo.Create(ctxWs1, u2); err != nil {
		t.Fatalf("create u2: %v", err)
	}

	// 1. Create team
	t1 := &team.Team{
		Name:      "Alpha",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := teamRepo.Create(ctxWs1, t1); err != nil {
		t.Fatalf("create team 1: %v", err)
	}
	if t1.ID == 0 || t1.WorkspaceID != 1 {
		t.Errorf("expected t1 populated with ID and wsID 1, got ID=%d wsID=%d", t1.ID, t1.WorkspaceID)
	}

	// 2. GetByID
	got, err := teamRepo.GetByID(ctxWs1, t1.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Alpha" {
		t.Errorf("expected team name 'Alpha', got %s", got.Name)
	}

	// Query from ws2 must return not found
	if _, err := teamRepo.GetByID(ctxWs2, t1.ID); !errors.Is(err, team.ErrNotFound) {
		t.Errorf("expected ErrNotFound querying ws1 team from ws2, got %v", err)
	}

	// 3. Update team
	t1.Name = "Alpha Team"
	t1.UpdatedAt = now.Add(time.Minute)
	if err := teamRepo.Update(ctxWs1, t1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = teamRepo.GetByID(ctxWs1, t1.ID)
	if got.Name != "Alpha Team" {
		t.Errorf("expected updated name 'Alpha Team', got %s", got.Name)
	}

	// 4. ReplaceMemberships
	members := []team.Membership{
		{UserID: u1.ID, Role: "lead", CreatedAt: now},
		{UserID: u2.ID, Role: "member", CreatedAt: now},
	}
	if err := teamRepo.ReplaceMemberships(ctxWs1, t1.ID, members); err != nil {
		t.Fatalf("ReplaceMemberships: %v", err)
	}

	// ReplaceMemberships with unknown user must fail
	unknownUserMembers := []team.Membership{{UserID: 9999, Role: "member"}}
	if err := teamRepo.ReplaceMemberships(ctxWs1, t1.ID, unknownUserMembers); !errors.Is(err, team.ErrUnknownUser) {
		t.Errorf("expected ErrUnknownUser, got %v", err)
	}

	// 5. List teams with members
	teams, err := teamRepo.List(ctxWs1, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(teams) != 1 || len(teams[0].Members) != 2 {
		t.Fatalf("expected 1 team with 2 members, got %d teams", len(teams))
	}
	if teams[0].Members[0].Role != "lead" && teams[0].Members[1].Role != "lead" {
		t.Errorf("expected a lead member in %+v", teams[0].Members)
	}

	// 6. Project cascading archive
	proj := &Project{
		TeamID:    t1.ID,
		Name:      "Top Secret Project",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.WithContext(ctxWs1).Create(proj).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	archiveTime := now.Add(2 * time.Hour)
	if err := teamRepo.Archive(ctxWs1, t1.ID, archiveTime); err != nil {
		t.Fatalf("Archive team: %v", err)
	}
	if err := teamRepo.ArchiveProjectsByTeam(ctxWs1, t1.ID, archiveTime); err != nil {
		t.Fatalf("ArchiveProjectsByTeam: %v", err)
	}

	archivedTeam, _ := teamRepo.GetByID(ctxWs1, t1.ID)
	if archivedTeam.ArchivedAt == nil || !archivedTeam.ArchivedAt.Equal(archiveTime) {
		t.Errorf("expected team archived at %v, got %v", archiveTime, archivedTeam.ArchivedAt)
	}

	var pCheck Project
	if err := db.WithContext(ctxWs1).First(&pCheck, proj.ID).Error; err != nil {
		t.Fatalf("find project: %v", err)
	}
	if pCheck.ArchivedAt == nil || !pCheck.ArchivedAt.Equal(archiveTime) {
		t.Errorf("expected project archived at %v, got %v", archiveTime, pCheck.ArchivedAt)
	}

	// List without includeArchived should return 0 teams
	teams, _ = teamRepo.List(ctxWs1, false)
	if len(teams) != 0 {
		t.Errorf("expected 0 active teams, got %d", len(teams))
	}

	// List with includeArchived should return 1 team
	teams, _ = teamRepo.List(ctxWs1, true)
	if len(teams) != 1 {
		t.Errorf("expected 1 team including archived, got %d", len(teams))
	}
}

func TestUserRepository(t *testing.T) {
	db := newTestDB(t)
	repo := NewUserRepository(db)

	ctxWs1 := WithWorkspaceID(context.Background(), 1)
	ctxWs2 := WithWorkspaceID(context.Background(), 2)

	now := time.Now().Truncate(time.Second)

	// 1. Create users
	adminUser := &user.User{
		Email:              "admin@example.com",
		Name:               "Super Admin",
		PasswordHash:       "hash1",
		Timezone:           "UTC",
		IsAdmin:            true,
		Active:             true,
		MustChangePassword: false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Create(ctxWs1, adminUser); err != nil {
		t.Fatalf("Create adminUser: %v", err)
	}
	if adminUser.ID == 0 || adminUser.WorkspaceID != 1 {
		t.Errorf("expected adminUser ID and wsID populated")
	}

	regularUser := &user.User{
		Email:              "member@example.com",
		Name:               "Regular Member",
		PasswordHash:       "hash2",
		Timezone:           "Europe/Berlin",
		IsAdmin:            false,
		Active:             true,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Create(ctxWs1, regularUser); err != nil {
		t.Fatalf("Create regularUser: %v", err)
	}

	// 2. CountActiveAdmins
	adminsCount, err := repo.CountActiveAdmins(ctxWs1)
	if err != nil {
		t.Fatalf("CountActiveAdmins: %v", err)
	}
	if adminsCount != 1 {
		t.Errorf("expected 1 active admin, got %d", adminsCount)
	}

	// 3. GetByID & GetByEmail
	gotByID, err := repo.GetByID(ctxWs1, adminUser.ID)
	if err != nil || gotByID.Email != "admin@example.com" {
		t.Errorf("GetByID failed: err=%v, user=%+v", err, gotByID)
	}
	gotByEmail, err := repo.GetByEmail(ctxWs1, "MEMBER@EXAMPLE.COM") // case insensitive
	if err != nil || gotByEmail.ID != regularUser.ID {
		t.Errorf("GetByEmail failed: err=%v, user=%+v", err, gotByEmail)
	}

	// Isolation: ws2 should not find ws1 user
	if _, err := repo.GetByID(ctxWs2, adminUser.ID); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("expected ErrNotFound querying ws1 user from ws2, got %v", err)
	}

	// 4. Update
	regularUser.Name = "Updated Name"
	regularUser.Timezone = "Asia/Tokyo"
	if err := repo.Update(ctxWs1, regularUser); err != nil {
		t.Fatalf("Update: %v", err)
	}
	checkUser, _ := repo.GetByID(ctxWs1, regularUser.ID)
	if checkUser.Name != "Updated Name" || checkUser.Timezone != "Asia/Tokyo" {
		t.Errorf("unexpected updated user: %+v", checkUser)
	}

	// 5. Sessions
	sess1 := &user.Session{
		UserID:    regularUser.ID,
		TokenHash: "hash_sess_1",
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}
	sess2 := &user.Session{
		UserID:    regularUser.ID,
		TokenHash: "hash_sess_2",
		ExpiresAt: now.Add(48 * time.Hour),
		CreatedAt: now,
	}
	if err := repo.SaveSession(ctxWs1, sess1); err != nil {
		t.Fatalf("SaveSession 1: %v", err)
	}
	if err := repo.SaveSession(ctxWs1, sess2); err != nil {
		t.Fatalf("SaveSession 2: %v", err)
	}

	// DeleteUserSessionsExcept sess1
	if err := repo.DeleteUserSessionsExcept(ctxWs1, regularUser.ID, "hash_sess_1"); err != nil {
		t.Fatalf("DeleteUserSessionsExcept: %v", err)
	}
	var remSessions []Session
	if err := db.WithContext(ctxWs1).Where("user_id = ?", regularUser.ID).Find(&remSessions).Error; err != nil {
		t.Fatalf("find sessions: %v", err)
	}
	if len(remSessions) != 1 || remSessions[0].TokenHash != "hash_sess_1" {
		t.Errorf("expected only hash_sess_1 to remain, got %+v", remSessions)
	}

	// Delete single session
	if err := repo.DeleteSession(ctxWs1, "hash_sess_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	db.WithContext(ctxWs1).Where("user_id = ?", regularUser.ID).Find(&remSessions)
	if len(remSessions) != 0 {
		t.Errorf("expected 0 sessions after DeleteSession, got %d", len(remSessions))
	}

	// 6. APITokens
	tok1 := &user.APIToken{
		UserID:    regularUser.ID,
		Name:      "cli",
		TokenHash: "tok_hash_1",
		CreatedAt: now,
	}
	if err := repo.CreateAPIToken(ctxWs1, tok1); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if tok1.ID == 0 {
		t.Errorf("expected APIToken ID populated")
	}

	tokens, err := repo.ListAPITokens(ctxWs1, regularUser.ID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListAPITokens failed: err=%v, count=%d", err, len(tokens))
	}

	if err := repo.RevokeAPIToken(ctxWs1, tok1.ID, regularUser.ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	tokens, _ = repo.ListAPITokens(ctxWs1, regularUser.ID)
	if tokens[0].RevokedAt == nil {
		t.Errorf("expected token to be marked revoked")
	}

	// 7. RevokeAllOAuthTokens
	oauthAT := &OAuthAccessToken{
		RequestID:     "req-1",
		ClientID:      "cid",
		SignatureHash: "sig-at",
		UserID:        regularUser.ID,
		Scopes:        "[]",
		Audience:      "[]",
		SessionData:   "{}",
		ExpiresAt:     now.Add(time.Hour),
		CreatedAt:     now,
	}
	oauthRT := &OAuthRefreshToken{
		RequestID:           "req-1",
		ClientID:            "cid",
		SignatureHash:       "sig-rt",
		AccessSignatureHash: "sig-at",
		UserID:              regularUser.ID,
		Scopes:              "[]",
		SessionData:         "{}",
		Active:              true,
		ExpiresAt:           now.Add(time.Hour),
		CreatedAt:           now,
	}
	oauthCode := &OAuthCode{
		RequestID:     "req-1",
		ClientID:      "cid",
		SignatureHash: "sig-code",
		UserID:        regularUser.ID,
		SessionData:   "{}",
		Used:          false,
		ExpiresAt:     now.Add(time.Hour),
		CreatedAt:     now,
	}
	oauthDev := &OAuthDeviceCode{
		RequestID:      "req-1",
		ClientID:       "cid",
		DeviceCodeHash: "dev-hash",
		UserCodeHash:   "user-hash",
		UserID:         regularUser.ID,
		Status:         0,
		Scopes:         "[]",
		SessionData:    "{}",
		ExpiresAt:      now.Add(time.Hour),
		CreatedAt:      now,
	}

	for _, m := range []any{oauthAT, oauthRT, oauthCode, oauthDev} {
		if err := db.WithContext(ctxWs1).Create(m).Error; err != nil {
			t.Fatalf("create oauth model: %v", err)
		}
	}

	if err := repo.RevokeAllOAuthTokens(ctxWs1, regularUser.ID); err != nil {
		t.Fatalf("RevokeAllOAuthTokens: %v", err)
	}

	var checkAT OAuthAccessToken
	db.WithContext(ctxWs1).First(&checkAT, oauthAT.ID)
	if checkAT.RevokedAt == nil {
		t.Errorf("expected OAuth access token to be revoked")
	}

	var checkRT OAuthRefreshToken
	db.WithContext(ctxWs1).First(&checkRT, oauthRT.ID)
	if checkRT.Active || checkRT.RevokedAt == nil {
		t.Errorf("expected OAuth refresh token to be inactive and revoked")
	}

	var checkCode OAuthCode
	db.WithContext(ctxWs1).First(&checkCode, oauthCode.ID)
	if !checkCode.Used {
		t.Errorf("expected OAuth code to be used")
	}

	var checkDev OAuthDeviceCode
	db.WithContext(ctxWs1).First(&checkDev, oauthDev.ID)
	if checkDev.Status != 2 {
		t.Errorf("expected OAuth device code status 2, got %d", checkDev.Status)
	}
}
