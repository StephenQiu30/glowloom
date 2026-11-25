package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func governancePageQuery(q *gorm.DB, f application.GovernanceFilter, p application.GovernancePosition) *gorm.DB {
	q = q.Where("created_at<=?", p.AsOf)
	switch f.Status {
	case "expired":
		q = q.Where("status='expired' OR (status='pending' AND expires_at<=?)", p.AsOf)
	case "pending":
		q = q.Where("status='pending' AND expires_at>?", p.AsOf)
	case "":
	default:
		q = q.Where("status=?", f.Status)
	}
	if p.ID != uuid.Nil {
		q = q.Where("(created_at,id)<(?,?)", p.CreatedAt, p.ID)
	}
	return q.Order("created_at DESC,id DESC").Limit(f.Limit + 1)
}
func (t *governanceTx) QueryInvitations(team *uuid.UUID, f application.GovernanceFilter, p application.GovernancePosition) ([]application.Invitation, error) {
	out := []application.Invitation{}
	err := governancePageQuery(t.db.Table("identity.invitation").Where("team_id IS NOT DISTINCT FROM ?::uuid", team), f, p).Find(&out).Error
	for n := range out {
		if out[n].Status == "pending" && !p.AsOf.Before(out[n].ExpiresAt) {
			out[n].Status = "expired"
		}
	}
	return out, err
}
func (t *governanceTx) QueryProposals(team *uuid.UUID, scope string, f application.GovernanceFilter, p application.GovernancePosition) ([]application.AuthorityProposal, error) {
	out := []application.AuthorityProposal{}
	q := t.db.Table("identity.authority_proposal")
	switch scope {
	case "mine":
		q = q.Where("target_user_id=?", t.actor.ID)
	case "platform":
		q = q.Where("kind='platform_grant'")
	default:
		q = q.Where("team_id=?", team)
	}
	err := governancePageQuery(q, f, p).Find(&out).Error
	for n := range out {
		if out[n].Status == "pending" && !p.AsOf.Before(out[n].ExpiresAt) {
			out[n].Status = "expired"
		}
	}
	return out, err
}
