package store

// TeamMember is one membership joined with its team's name, for /me.
type TeamMember struct {
	TeamID uint
	Team   string
	Role   string
}

// MembershipsOf returns every membership of userID in this workspace, archived
// teams included: the rows stay after a team is archived, they just grant nothing.
func (s *Store) MembershipsOf(userID uint) ([]TeamMember, error) {
	out := []TeamMember{}
	err := s.db.Model(&TeamMembership{}).
		Select("team_memberships.team_id, teams.name AS team, team_memberships.role").
		Where("team_memberships.workspace_id = ?", s.workspaceID).
		Joins("JOIN teams ON teams.id = team_memberships.team_id AND teams.workspace_id = team_memberships.workspace_id").
		Where("team_memberships.user_id = ?", userID).
		Order("teams.name").
		Scan(&out).Error
	return out, err
}

// LeadsTeam reports whether userID leads teamID in this workspace. An archived
// team grants nothing, so its membership rows are not enough.
func (s *Store) LeadsTeam(userID, teamID uint) (bool, error) {
	var n int64
	err := s.db.Model(&TeamMembership{}).
		Where("team_memberships.workspace_id = ?", s.workspaceID).
		Joins("JOIN teams ON teams.id = team_memberships.team_id AND teams.workspace_id = team_memberships.workspace_id").
		Where("team_memberships.user_id = ? AND team_memberships.team_id = ? AND team_memberships.role = ? AND teams.archived_at IS NULL",
			userID, teamID, RoleLead).
		Count(&n).Error
	return n > 0, err
}
