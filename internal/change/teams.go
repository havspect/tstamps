package change

import (
	"context"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

// Teams manages mutations to teams and memberships.
type Teams struct {
	store *store.Store
	clock clock.Clock
}

// Create inserts team and records an audit event in a single transaction.
func (s *Teams) Create(ctx context.Context, actor Actor, team *store.Team) (*store.Team, error) {
	now := s.clock.Now()
	err := s.store.InTx(func(tx *store.Store) error {
		if err := tx.CreateTeam(team); err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "team.create", "team", team.ID,
			store.Change{After: team.AuditFields()})
	})
	if err != nil {
		return nil, err
	}
	return team, nil
}

// Rename updates team name and records an audit event in a single transaction.
func (s *Teams) Rename(ctx context.Context, actor Actor, id uint, newName string) (*store.Team, []store.Member, error) {
	now := s.clock.Now()
	var team *store.Team
	var members []store.Member

	err := s.store.InTx(func(tx *store.Store) error {
		before, err := tx.TeamByID(id)
		if err != nil {
			return err
		}
		if team, err = tx.RenameTeam(id, newName, now); err != nil {
			return err
		}
		if members, err = tx.MembersOf(id); err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "team.rename", "team", id,
			store.Change{Before: before.AuditFields(), After: team.AuditFields()})
	})
	if err != nil {
		return nil, nil, err
	}
	return team, members, nil
}

// Archive archives team id and cascades archiving to all of its active projects,
// recording audit events for the team and each affected project in one transaction.
func (s *Teams) Archive(ctx context.Context, actor Actor, id uint) error {
	now := s.clock.Now()
	return s.store.InTx(func(tx *store.Store) error {
		before, err := tx.TeamByID(id)
		if err != nil {
			return err
		}
		var projects []store.Project
		if before.ArchivedAt == nil {
			all, err := tx.Projects(true)
			if err != nil {
				return err
			}
			for _, p := range all {
				if p.TeamID == id && p.ArchivedAt == nil {
					projects = append(projects, p)
				}
			}
		}
		if err := tx.ArchiveTeam(id, now); err != nil {
			return err
		}
		if before.ArchivedAt != nil {
			return nil
		}
		auditActor := actor.AuditActor()
		after, err := tx.TeamByID(id)
		if err != nil {
			return err
		}
		if err := tx.RecordAudit(now, auditActor, "team.archive", "team", id,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()}); err != nil {
			return err
		}
		for _, p := range projects {
			afterProject, err := tx.ProjectByID(p.ID)
			if err != nil {
				return err
			}
			if err := tx.RecordAudit(now, auditActor, "project.archive", "project", p.ID,
				store.Change{Before: p.AuditFields(), After: afterProject.AuditFields()}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReplaceMembers sets the memberships for team id and records an audit event in one transaction.
func (s *Teams) ReplaceMembers(ctx context.Context, actor Actor, id uint, roles []store.MemberRole) ([]store.Member, error) {
	now := s.clock.Now()
	var after []store.Member
	err := s.store.InTx(func(tx *store.Store) error {
		before, err := tx.MembersOf(id)
		if err != nil {
			return err
		}
		if err := tx.ReplaceMemberships(id, roles, now); err != nil {
			return err
		}
		if after, err = tx.MembersOf(id); err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "team.members.replace", "team", id,
			store.Change{Before: membersAudit(before), After: membersAudit(after)})
	})
	if err != nil {
		return nil, err
	}
	return after, nil
}

func membersAudit(rows []store.Member) map[string]any {
	list := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		list = append(list, map[string]any{"user_id": m.UserID, "email": m.Email, "role": m.Role})
	}
	return map[string]any{"members": list}
}
