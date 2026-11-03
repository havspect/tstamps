package gorm

import "gorm.io/gorm"

// applyRowScope narrows q to the entries a lead may read: entries on projects
// of the given teams, plus their own entries. It adds no predicate when the
// teams and the user are both empty. Caller filters are added before this, so
// they can only narrow the union.
func applyRowScope(db, q *gorm.DB, teamIDs []uint, selfUserID uint) (*gorm.DB, error) {
	if len(teamIDs) == 0 && selfUserID == 0 {
		return q, nil
	}

	var projectIDs []uint
	if len(teamIDs) > 0 {
		if err := db.Model(&Project{}).Where("team_id IN ?", teamIDs).Pluck("id", &projectIDs).Error; err != nil {
			return nil, err
		}
	}

	cond := db.Where("1 = 0")
	if len(projectIDs) > 0 {
		cond = db.Where("project_id IN ?", projectIDs)
	}
	if selfUserID > 0 {
		cond = cond.Or("user_id = ?", selfUserID)
	}
	return q.Where(cond), nil
}
