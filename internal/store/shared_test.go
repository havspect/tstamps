package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// TestRegistryScopesEveryWorkspace is the core promise of the shared file: two
// workspaces live in one database, and no read in one can see the other's rows.
func TestRegistryScopesEveryWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tstamps.db")
	reg, err := OpenRegistry(path)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	if err := reg.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	alpha, beta := &Workspace{Slug: "alpha"}, &Workspace{Slug: "beta"}
	for _, w := range []*Workspace{alpha, beta} {
		if err := reg.CreateWorkspace(w); err != nil {
			t.Fatalf("create %s: %v", w.Slug, err)
		}
	}
	as, bs := reg.ForWorkspace(alpha.ID), reg.ForWorkspace(beta.ID)

	// The same email is two accounts, one per workspace.
	au, bu := &User{Email: "ada@example.com", Name: "Ada Alpha", PasswordHash: "x"},
		&User{Email: "ada@example.com", Name: "Ada Beta", PasswordHash: "y"}
	for _, u := range []struct {
		st *Store
		u  *User
	}{{as, au}, {bs, bu}} {
		if err := u.st.CreateUser(u.u); err != nil {
			t.Fatalf("create user in %s: %v", u.u.Name, err)
		}
	}
	got, err := as.UserByEmail("ada@example.com")
	if err != nil || got.Name != "Ada Alpha" {
		t.Fatalf("alpha email = %+v, %v; want Ada Alpha", got, err)
	}
	if _, err := as.UserByID(bu.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta user: %v, want ErrNotFound", err)
	}
	if err := as.SetUserPassword(bu.ID, "new", false, 0, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha reset beta password: %v, want ErrNotFound", err)
	}
	if err := as.SaveUser(&User{ID: bu.ID, Name: "hijack"}, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha save beta user: %v, want ErrNotFound", err)
	}

	// Teams, projects and entries are scoped the same way.
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	at, bt := &Team{Name: "Platform"}, &Team{Name: "Mobile"}
	_ = as.CreateTeam(at)
	_ = bs.CreateTeam(bt)
	ap, bp := &Project{TeamID: at.ID, Name: "Timeline"}, &Project{TeamID: bt.ID, Name: "Sync"}
	if err := as.CreateProject(ap); err != nil {
		t.Fatalf("create alpha project: %v", err)
	}
	if err := bs.CreateProject(bp); err != nil {
		t.Fatalf("create beta project: %v", err)
	}
	if _, err := as.ProjectByID(bp.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta project: %v, want ErrNotFound", err)
	}
	if _, err := as.TeamByID(bt.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta team: %v, want ErrNotFound", err)
	}
	if err := as.ArchiveTeam(bt.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha archive beta team: %v, want ErrNotFound", err)
	}
	if err := as.ReplaceMemberships(bt.ID, nil, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha edit beta memberships: %v, want ErrNotFound", err)
	}

	end := now.Add(time.Hour)
	for _, e := range []struct {
		st *Store
		e  *Entry
	}{
		{as, &Entry{UserID: au.ID, ProjectID: ap.ID, StartAt: now, EndAt: &end, Timezone: "UTC"}},
		{bs, &Entry{UserID: bu.ID, ProjectID: bp.ID, StartAt: now, EndAt: &end, Timezone: "UTC"}},
	} {
		if err := e.st.CreateEntry(e.e); err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}
	from, to := now.Add(-time.Hour), now.Add(2*time.Hour)
	entries, err := as.EntriesInRange(from, to, Scope{All: true})
	if err != nil {
		t.Fatalf("alpha entries: %v", err)
	}
	if len(entries) != 1 || entries[0].UserID != au.ID {
		t.Fatalf("alpha entries = %+v, want only its own", entries)
	}

	// A running entry per workspace is legal, because the partial index is per
	// workspace, and an entry ID from beta is invisible in alpha.
	alphaRunning := &Entry{UserID: au.ID, ProjectID: ap.ID, StartAt: now, Timezone: "UTC"}
	betaRunning := &Entry{UserID: bu.ID, ProjectID: bp.ID, StartAt: now, Timezone: "UTC"}
	if err := as.CreateEntry(alphaRunning); err != nil {
		t.Fatalf("create alpha running entry: %v", err)
	}
	if err := bs.CreateEntry(betaRunning); err != nil {
		t.Fatalf("create beta running entry: %v", err)
	}
	if _, err := as.EntryByID(betaRunning.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta entry: %v, want ErrNotFound", err)
	}
	if _, err := as.RunningEntry(au.ID); err != nil {
		t.Fatalf("alpha running entry: %v", err)
	}

	// Sessions, tokens and audit events are workspace-local too.
	if err := as.CreateSession(&Session{UserID: au.ID, TokenHash: "same-hash", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("create alpha session: %v", err)
	}
	if _, err := bs.SessionByTokenHash("same-hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("beta session lookup: %v, want ErrNotFound", err)
	}
	betaSession := &Session{UserID: bu.ID, TokenHash: "beta-session", ExpiresAt: now.Add(time.Hour)}
	if err := bs.CreateSession(betaSession); err != nil {
		t.Fatalf("create beta session: %v", err)
	}
	if err := as.DeleteSession(betaSession.ID); err != nil {
		t.Fatalf("alpha delete beta session: %v", err)
	}
	if _, err := bs.SessionByTokenHash("beta-session"); err != nil {
		t.Fatalf("beta session was deleted from alpha: %v", err)
	}
	if err := as.CreateAPIToken(&APIToken{UserID: au.ID, Name: "cli", TokenHash: "same-token"}); err != nil {
		t.Fatalf("create alpha token: %v", err)
	}
	if _, err := bs.APITokenByTokenHash("same-token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("beta token lookup: %v, want ErrNotFound", err)
	}
	betaToken := &APIToken{UserID: bu.ID, Name: "cli", TokenHash: "beta-token"}
	if err := bs.CreateAPIToken(betaToken); err != nil {
		t.Fatalf("create beta token: %v", err)
	}
	if err := as.RevokeAPIToken(betaToken.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha revoke beta token: %v, want ErrNotFound", err)
	}
	if tok, err := bs.APITokenByTokenHash("beta-token"); err != nil || tok.RevokedAt != nil {
		t.Fatalf("beta token = %+v, %v; want still live", tok, err)
	}
	if err := as.RecordAudit(now, AuditActor{UserID: au.ID, Role: "admin"}, "project.create", "project", ap.ID, Change{}); err != nil {
		t.Fatalf("record audit: %v", err)
	}
	events, err := bs.AuditEvents(AuditFilter{})
	if err != nil || len(events) != 0 {
		t.Fatalf("beta audit = %v, %v; want none", events, err)
	}

	// One file holds both workspaces: this is the architecture, not two stores
	// that happen to point at a copy.
	all, err := reg.Workspaces()
	if err != nil || len(all) != 2 {
		t.Fatalf("workspaces = %v, %v; want two", all, err)
	}
}

// TestDynamicTenancyScopingContract dynamically verifies that all models registered
// in AutoMigrate are properly categorized as either tenant-scoped or server-global,
// and that all tenant models enforce automatic tenancy session scoping on Create,
// Query, Update, and Delete.
func TestDynamicTenancyScopingContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.db")
	reg, err := OpenRegistry(path)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	if err := reg.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	w1, w2 := &Workspace{Slug: "w1"}, &Workspace{Slug: "w2"}
	if err := reg.CreateWorkspace(w1); err != nil {
		t.Fatalf("create w1: %v", err)
	}
	if err := reg.CreateWorkspace(w2); err != nil {
		t.Fatalf("create w2: %v", err)
	}

	s1 := reg.ForWorkspace(w1.ID)
	s2 := reg.ForWorkspace(w2.ID)

	var tenantCount, globalCount int
	seq := 1

	for _, modelPtr := range allModels {
		modelType := reflect.TypeOf(modelPtr).Elem()
		modelName := modelType.Name()

		t.Run(modelName, func(t *testing.T) {
			wsField, hasWorkspaceID := modelType.FieldByName("WorkspaceID")

			if !hasWorkspaceID {
				globalCount++
				// Server-global models must not have WorkspaceID and must be queryable via reg.db
				if wsField.Name == "WorkspaceID" {
					t.Fatalf("%s unexpectedly has WorkspaceID", modelName)
				}
				sliceType := reflect.SliceOf(modelType)
				slicePtr := reflect.New(sliceType).Interface()
				if err := reg.db.Find(slicePtr).Error; err != nil {
					t.Fatalf("global model %s query failed on reg.db: %v", modelName, err)
				}
				return
			}

			tenantCount++

			// 1. Un-scoped query/create through reg.db (no workspace in context) must fail.
			sliceType := reflect.SliceOf(modelType)
			slicePtr := reflect.New(sliceType).Interface()
			if err := reg.db.Find(slicePtr).Error; err == nil {
				t.Fatalf("expected error querying tenant model %s without workspace, got nil", modelName)
			} else if !strings.Contains(err.Error(), "workspace ID required") {
				t.Fatalf("unexpected error message for un-scoped query on %s: %v", modelName, err)
			}

			// 2. Creation via s1 must automatically set WorkspaceID to w1.ID.
			inst1 := newDummyTenantModel(modelPtr, seq)
			seq++
			if err := s1.db.Create(inst1).Error; err != nil {
				t.Fatalf("create %s in s1: %v", modelName, err)
			}

			instVal := reflect.ValueOf(inst1).Elem()
			gotWSID := uint(instVal.FieldByName("WorkspaceID").Uint())
			if gotWSID != w1.ID {
				t.Fatalf("expected %s.WorkspaceID = %d, got %d", modelName, w1.ID, gotWSID)
			}

			var rowID uint
			if idField := instVal.FieldByName("ID"); idField.IsValid() {
				rowID = uint(idField.Uint())
			}

			// 3. Creation with mismatched WorkspaceID in s1 must be rejected.
			instMismatch := newDummyTenantModel(modelPtr, seq)
			seq++
			reflect.ValueOf(instMismatch).Elem().FieldByName("WorkspaceID").SetUint(uint64(w2.ID))
			if err := s1.db.Create(instMismatch).Error; err == nil {
				t.Fatalf("expected error creating %s with mismatched WorkspaceID in s1, got nil", modelName)
			} else if !strings.Contains(err.Error(), "workspace ID mismatch") {
				t.Fatalf("unexpected error for mismatched create on %s: %v", modelName, err)
			}

			if rowID == 0 {
				return
			}

			// 4. Query isolation: s2 must NOT see the row created in s1.
			var count int64
			if err := s2.db.Model(modelPtr).Where("id = ?", rowID).Count(&count).Error; err != nil {
				t.Fatalf("count %s in s2: %v", modelName, err)
			}
			if count != 0 {
				t.Fatalf("s2 saw %d rows of %s created in s1, want 0", count, modelName)
			}

			queryInst := reflect.New(modelType).Interface()
			if err := s2.db.First(queryInst, rowID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("s2 First(%s, %d) = %v; want ErrRecordNotFound", modelName, rowID, err)
			}

			// 5. Update isolation: s2 update must affect 0 rows.
			res := s2.db.Model(modelPtr).Where("id = ?", rowID).Update("created_at", time.Now())
			if res.Error != nil {
				t.Fatalf("s2 update %s: %v", modelName, res.Error)
			}
			if res.RowsAffected != 0 {
				t.Fatalf("s2 update affected %d rows of %s in s1, want 0", res.RowsAffected, modelName)
			}

			// 6. Delete isolation: s2 delete must affect 0 rows.
			delRes := s2.db.Delete(reflect.New(modelType).Interface(), rowID)
			if delRes.Error != nil {
				t.Fatalf("s2 delete %s: %v", modelName, delRes.Error)
			}
			if delRes.RowsAffected != 0 {
				t.Fatalf("s2 delete affected %d rows of %s in s1, want 0", delRes.RowsAffected, modelName)
			}

			// 7. Verify the row in s1 still exists and is untouched.
			found := reflect.New(modelType).Interface()
			if err := s1.db.First(found, rowID).Error; err != nil {
				t.Fatalf("s1 First(%s, %d) failed after s2 attempts: %v", modelName, rowID, err)
			}

			// 8. Transaction scoping: transaction on s1 sees it, transaction on s2 does not.
			err = s1.InTx(func(tx *Store) error {
				var txFound int64
				if err := tx.db.Model(modelPtr).Where("id = ?", rowID).Count(&txFound).Error; err != nil {
					return err
				}
				if txFound != 1 {
					return fmt.Errorf("s1 InTx count = %d, want 1", txFound)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("s1 InTx verification failed: %v", err)
			}

			err = s2.InTx(func(tx *Store) error {
				var txFound int64
				if err := tx.db.Model(modelPtr).Where("id = ?", rowID).Count(&txFound).Error; err != nil {
					return err
				}
				if txFound != 0 {
					return fmt.Errorf("s2 InTx count = %d, want 0", txFound)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("s2 InTx verification failed: %v", err)
			}
		})
	}

	if tenantCount != 13 {
		t.Fatalf("expected 13 tenant models, checked %d", tenantCount)
	}
	if globalCount != 3 {
		t.Fatalf("expected 3 server-global models, checked %d", globalCount)
	}
}

func newDummyTenantModel(m any, seq int) any {
	elemType := reflect.TypeOf(m).Elem()
	val := reflect.New(elemType).Elem()

	now := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < elemType.NumField(); i++ {
		f := elemType.Field(i)
		vf := val.Field(i)
		if !vf.CanSet() {
			continue
		}
		if f.Name == "ID" || f.Name == "WorkspaceID" {
			continue
		}
		switch vf.Kind() {
		case reflect.String:
			switch f.Name {
			case "Email":
				vf.SetString(fmt.Sprintf("user-%d@example.com", seq))
			case "Role":
				vf.SetString(RoleMember)
			case "RedirectURIs", "GrantTypes", "ResponseTypes", "Scopes", "Audience":
				vf.SetString(`["test"]`)
			case "Changes":
				vf.SetString(`{}`)
			default:
				vf.SetString(fmt.Sprintf("%s-%d", strings.ToLower(f.Name), seq))
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			vf.SetUint(uint64(seq))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			vf.SetInt(int64(seq))
		case reflect.Struct:
			if f.Type == reflect.TypeOf(time.Time{}) {
				vf.Set(reflect.ValueOf(now))
			}
		case reflect.Slice:
			if f.Type == reflect.TypeOf([]byte{}) {
				vf.SetBytes([]byte(fmt.Sprintf("secret-%d", seq)))
			}
		case reflect.Ptr:
			if f.Name == "EndAt" {
				end := now.Add(time.Hour)
				vf.Set(reflect.ValueOf(&end))
			}
		}
	}
	return val.Addr().Interface()
}
