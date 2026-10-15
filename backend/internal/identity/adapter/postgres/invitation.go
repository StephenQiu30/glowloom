package postgres

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func (t *governanceTx) Invitation(id uuid.UUID, hash []byte) (application.Invitation, error) {
	var i application.Invitation
	q := t.db.Table("identity.invitation")
	if id != uuid.Nil {
		q = q.Where("id=?", id)
	} else {
		q = q.Where("token_hash=?", hash)
	}
	r := q.Find(&i)
	if r.Error != nil {
		return i, r.Error
	}
	if r.RowsAffected != 1 {
		return i, application.ErrProofInvalid
	}
	return i, nil
}
func (t *governanceTx) InsertInvitation(i application.Invitation) error {
	if err := t.db.Exec(`UPDATE identity.invitation SET status='expired',revision=revision+1,updated_at=? WHERE status='pending' AND expires_at<=?`, i.CreatedAt, i.CreatedAt).Error; err != nil {
		return err
	}
	var exists bool
	if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.invitation WHERE kind=? AND team_id IS NOT DISTINCT FROM ?::uuid AND email_key=? AND status='pending')`, i.Kind, i.TeamID, i.EmailKey).Scan(&exists).Error; err != nil {
		return err
	}
	if exists {
		return application.ErrGovernanceConflict
	}
	if i.TeamID != nil {
		var n int64
		if err := t.db.Raw(`SELECT count(*) FROM identity.invitation WHERE team_id=? AND status='pending'`, i.TeamID).Scan(&n).Error; err != nil {
			return err
		}
		if n >= 100 {
			return application.ErrGovernanceConflict
		}
	}
	return t.db.Exec(`INSERT INTO identity.invitation(id,kind,team_id,email_key,team_role,inviter_id,status,token_hash,expires_at,membership_revision,revision,created_at,updated_at,platform_issued) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, i.ID, i.Kind, i.TeamID, i.EmailKey, i.TeamRole, i.InviterID, i.Status, i.TokenHash, i.ExpiresAt, i.MembershipRevision, i.Revision, i.CreatedAt, i.CreatedAt, i.PlatformIssued).Error
}
func (t *governanceTx) SaveInvitation(i application.Invitation, now time.Time) error {
	r := t.db.Exec(`UPDATE identity.invitation SET status=?,token_hash=?,expires_at=?,accepted_user_id=?,revision=?,updated_at=? WHERE id=? AND revision=? AND status='pending'`, i.Status, i.TokenHash, i.ExpiresAt, i.AcceptedUserID, i.Revision, now, i.ID, i.Revision-1)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	return nil
}
func (t *governanceTx) InvitationReceipt(key uuid.UUID, hash string) (application.Invitation, bool, error) {
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	r := t.db.Raw(`SELECT request_hash,response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete`, t.actor.ID, "invitation.create:"+key.String()).Scan(&row)
	if r.Error != nil {
		return application.Invitation{}, false, r.Error
	}
	if r.RowsAffected == 0 {
		return application.Invitation{}, false, nil
	}
	if row.RequestHash != hash {
		return application.Invitation{}, false, application.ErrGovernanceConflict
	}
	var id uuid.UUID
	if err := json.Unmarshal(row.ResponseBody, &id); err != nil {
		return application.Invitation{}, false, err
	}
	i, err := t.Invitation(id, nil)
	return i, true, err
}
func (t *governanceTx) SaveInvitationReceipt(key uuid.UUID, hash string, i application.Invitation, now time.Time) error {
	body, err := json.Marshal(i.ID)
	if err != nil {
		return err
	}
	return t.db.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at,create_time,update_time) VALUES(?,?,?,?,201,?::jsonb,?,?,?)`, uuid.New(), t.actor.ID, "invitation.create:"+key.String(), hash, string(body), now.Add(24*time.Hour), now, now).Error
}
func (t *governanceTx) ReserveInvitationMail(email string, now time.Time) error {
	b, err := t.Budget("invitation/" + email)
	if err != nil {
		return err
	}
	b.Prune(now)
	if wait := b.RetryAfter(now, 5, 10, time.Minute); wait > 0 {
		return &application.RateLimit{RetryAfter: wait}
	}
	b.SentAt = append(b.SentAt, now)
	return t.SaveBudget("invitation/"+email, b)
}

// RevokePendingTeamInvitations invalidates proofs under the caller's shared governance lock.
// A pool connection is rejected: invitation and team policy changes must commit together.
func (s *Store) RevokePendingTeamInvitations(ctx context.Context, team uuid.UUID, now time.Time) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrGovernanceConflict
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.ErrGovernanceConflict
	}
	return s.db.WithContext(ctx).Exec(`UPDATE identity.invitation SET status='revoked',revision=revision+1,updated_at=? WHERE team_id=? AND kind='team' AND status='pending'`, now, team).Error
}
