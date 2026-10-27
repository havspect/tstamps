CREATE TRIGGER IF NOT EXISTS prevent_audit_log_update BEFORE UPDATE ON audit_log BEGIN SELECT RAISE(ABORT, 'audit_log is append-only: updates forbidden'); END;
CREATE TRIGGER IF NOT EXISTS prevent_audit_log_delete BEFORE DELETE ON audit_log BEGIN SELECT RAISE(ABORT, 'audit_log is append-only: deletes forbidden'); END;
CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_one_running ON entries(workspace_id, user_id) WHERE end_at IS NULL AND deleted_at IS NULL;
