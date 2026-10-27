package gorm

import (
	"context"
	"fmt"
	"reflect"

	"github.com/havspect/tstamps/internal/auth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type workspaceContextKey struct{}
type bypassTenancyContextKey struct{}

// WithWorkspaceID returns a child context with the workspace ID attached.
func WithWorkspaceID(ctx context.Context, id uint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, workspaceContextKey{}, id)
}

// WithBypassTenancy returns a child context allowing global queries across workspaces (e.g. for authentication token resolution).
func WithBypassTenancy(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, bypassTenancyContextKey{}, true)
}

func isBypassTenancy(db *gorm.DB) bool {
	if db.Statement != nil && db.Statement.Context != nil {
		if b, ok := db.Statement.Context.Value(bypassTenancyContextKey{}).(bool); ok && b {
			return true
		}
	}
	if v, ok := db.Get("tenancy:bypass"); ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	return false
}

// WorkspaceIDFrom extracts the workspace ID from the context if present and positive.
// It checks workspaceContextKey first, and falls back to auth.Principal attached to ctx.
func WorkspaceIDFrom(ctx context.Context) (uint, bool) {
	if ctx == nil {
		return 0, false
	}
	if id, ok := ctx.Value(workspaceContextKey{}).(uint); ok && id > 0 {
		return id, true
	}
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil && p.WorkspaceID > 0 {
		return p.WorkspaceID, true
	}
	return 0, false
}

// getWorkspaceID extracts the workspace ID from the db statement's context or settings.
func getWorkspaceID(db *gorm.DB) (uint, bool) {
	if db.Statement != nil && db.Statement.Context != nil {
		if id, ok := WorkspaceIDFrom(db.Statement.Context); ok {
			return id, true
		}
	}
	if v, ok := db.Get("workspace_id"); ok {
		if id, ok := v.(uint); ok && id > 0 {
			return id, true
		}
	}
	return 0, false
}

// getTenantWorkspaceField reports whether stmt operates on a model that has a WorkspaceID field,
// and returns the Field if found. Server-global models without WorkspaceID return nil.
func getTenantWorkspaceField(stmt *gorm.Statement) *schema.Field {
	if stmt == nil {
		return nil
	}
	if stmt.Schema == nil {
		if stmt.Model != nil {
			_ = stmt.Parse(stmt.Model)
		} else if stmt.Dest != nil {
			_ = stmt.Parse(stmt.Dest)
		}
	}
	if stmt.Schema == nil {
		return nil
	}
	return stmt.Schema.LookUpField("WorkspaceID")
}

func tenantCreateCallback(db *gorm.DB) {
	if db.Error != nil || db.Statement == nil {
		return
	}
	field := getTenantWorkspaceField(db.Statement)
	if field == nil {
		return
	}

	wsID, ok := getWorkspaceID(db)
	if !ok {
		if isBypassTenancy(db) {
			return
		}
		name := ""
		if db.Statement.Schema != nil {
			name = db.Statement.Schema.Name
		}
		db.AddError(fmt.Errorf("workspace ID required for tenant model %s", name))
		return
	}

	setOrVerify := func(val reflect.Value) error {
		for val.Kind() == reflect.Ptr {
			if val.IsNil() {
				return nil
			}
			val = val.Elem()
		}
		if val.Kind() != reflect.Struct {
			return nil
		}
		currentVal, isZero := field.ValueOf(db.Statement.Context, val)
		if !isZero {
			if curID, ok := currentVal.(uint); ok && curID != 0 && curID != wsID {
				return fmt.Errorf("workspace ID mismatch: got %d, want %d", curID, wsID)
			}
		}
		return field.Set(db.Statement.Context, val, wsID)
	}

	reflectVal := db.Statement.ReflectValue
	switch reflectVal.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < reflectVal.Len(); i++ {
			if err := setOrVerify(reflectVal.Index(i)); err != nil {
				db.AddError(err)
				return
			}
		}
	case reflect.Struct:
		if err := setOrVerify(reflectVal); err != nil {
			db.AddError(err)
			return
		}
	default:
		if db.Statement.Dest != nil {
			if err := setOrVerify(reflect.ValueOf(db.Statement.Dest)); err != nil {
				db.AddError(err)
				return
			}
		}
	}
}

func tenantScopeCallback(db *gorm.DB) {
	if db.Error != nil || db.Statement == nil {
		return
	}
	field := getTenantWorkspaceField(db.Statement)
	if field == nil {
		return
	}

	wsID, ok := getWorkspaceID(db)
	if !ok {
		if isBypassTenancy(db) {
			return
		}
		name := ""
		if db.Statement.Schema != nil {
			name = db.Statement.Schema.Name
		}
		db.AddError(fmt.Errorf("workspace ID required for tenant model %s", name))
		return
	}

	const appliedKey = "tenancy:applied"
	if _, loaded := db.Statement.Settings.LoadOrStore(appliedKey, true); loaded {
		return
	}

	tableName := db.Statement.Schema.Table
	if db.Statement.Table != "" {
		tableName = db.Statement.Table
	}

	col := clause.Column{Table: tableName, Name: field.DBName}
	db.Statement.AddClause(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: col, Value: wsID},
		},
	})
}

// RegisterTenancyCallbacks registers tenancy enforcement callbacks on GORM.
func RegisterTenancyCallbacks(db *gorm.DB) {
	_ = db.Callback().Create().Before("gorm:before_create").Register("tenancy:create", tenantCreateCallback)
	_ = db.Callback().Query().Before("gorm:query").Register("tenancy:query", tenantScopeCallback)
	_ = db.Callback().Row().Before("gorm:row").Register("tenancy:row", tenantScopeCallback)
	_ = db.Callback().Update().Before("gorm:before_update").Register("tenancy:update", tenantScopeCallback)
	_ = db.Callback().Delete().Before("gorm:before_delete").Register("tenancy:delete", tenantScopeCallback)
}
