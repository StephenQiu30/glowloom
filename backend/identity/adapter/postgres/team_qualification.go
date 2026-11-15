package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// TeamQualificationStore owns only Identity facts and immutable purpose proofs.
// It never queries a team, uses a session credential or pins candidate eligibility.
type TeamQualificationStore struct{ db *gorm.DB }

// NewTeamQualificationStore binds the actual restricted Identity root pool.
func NewTeamQualificationStore(db *gorm.DB) *TeamQualificationStore {
	return &TeamQualificationStore{db: db}
}

type teamQualificationTx struct {
	db       *gorm.DB
	facts    application.TeamQualificationFacts
	previous *application.TeamQualificationProof
}

func (t *teamQualificationTx) Facts() application.TeamQualificationFacts     { return t.facts }
func (t *teamQualificationTx) Previous() *application.TeamQualificationProof { return t.previous }

// QualificationTransaction retains the original settings/account lock order and
// completes source admission in the same local transaction as the signed purpose.
func (s *TeamQualificationStore) QualificationTransaction(ctx context.Context, r application.TeamQualificationRequest, run func(application.TeamQualificationTx) error) error {
	if s == nil || r.Validate() != nil {
		return writefence.ErrInvalid
	}
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		if _, err := (&emailAuthTx{authTx: &authTx{db: tx}}).RegistrationEnabled(); err != nil {
			return err
		}
		var target struct{ TargetID uuid.UUID }
		read := tx.Raw(`SELECT target_id FROM identity.account_qualification_change WHERE request_id=?`, r.RequestID).Scan(&target)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || target.TargetID == r.CandidateID {
			return writefence.ErrUnavailable
		}
		var users []struct {
			ID              uuid.UUID
			Status          domain.Status
			EmailVerifiedAt *time.Time
			IsDelete        bool
		}
		read = tx.Raw(`SELECT id,status,email_verified_at,is_delete FROM identity."user" WHERE id IN ? ORDER BY id FOR UPDATE`, []uuid.UUID{target.TargetID, r.CandidateID}).Scan(&users)
		if read.Error != nil {
			return read.Error
		}
		if len(users) != 2 {
			return writefence.ErrUnavailable
		}
		intent, err := readAccountQualificationIntentTx(tx, r.RequestID, true)
		if err != nil {
			return err
		}
		facts := application.TeamQualificationFacts{Intent: intent, CandidateID: r.CandidateID}
		for _, u := range users {
			if u.ID == r.CandidateID {
				facts.Active = u.Status == domain.StatusActive
				facts.Verified = u.EmailVerifiedAt != nil && !u.EmailVerifiedAt.IsZero()
				facts.Deleted = u.IsDelete
			}
		}
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity.account_qualification_change WHERE target_id=? AND outcome='pending')`, r.CandidateID).Scan(&facts.Pending).Error; err != nil {
			return err
		}
		var head struct {
			Epoch             int64
			CoordinationID    uuid.UUID
			Blocked, Complete bool
		}
		read = tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, r.CandidateID).Scan(&head)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return writefence.ErrUnavailable
		}
		facts.Control = writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.CandidateID, Epoch: head.Epoch}, CoordinationID: head.CoordinationID, Blocked: head.Blocked}
		if head.Complete {
			if err := requirePasswordControl(ctx, tx, facts.Control, true); err != nil {
				return err
			}
			facts.Complete = true
		}
		state := &teamQualificationTx{db: tx, facts: facts}
		if r.PreviousProofID != uuid.Nil {
			previous, err := readTeamQualificationTx(tx, r.PreviousProofID)
			if err != nil {
				return err
			}
			state.previous = &previous
		}
		return run(state)
	})
}

func (t *teamQualificationTx) SaveTeamQualification(p application.TeamQualificationProof) (application.TeamQualificationProof, error) {
	if p.Validate() != nil {
		return application.TeamQualificationProof{}, writefence.ErrInvalid
	}
	if p.Eligible {
		if err := pgfence.NewAuthorityStore(t.db).Register(t.db.Statement.Context, t.db, p.Command, writefence.WriterCreation); err != nil {
			return application.TeamQualificationProof{}, err
		}
	}
	binding, err := p.Command.AuthorityDigest()
	if err != nil {
		return application.TeamQualificationProof{}, err
	}
	var previous *uuid.UUID
	if p.PreviousProofID != uuid.Nil {
		previous = &p.PreviousProofID
	}
	if err := t.db.Exec(`INSERT INTO identity.team_administrator_qualification(id,request_id,team_id,candidate_id,team_snapshot_sha256,qualification_epoch,source_coordination_id,eligible,command_sha256,authority_set_sha256,previous_proof_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,statement_timestamp()) ON CONFLICT(id) DO NOTHING`, p.ID, p.Intent.RequestID, p.TeamID, p.CandidateID, p.SnapshotSHA256, p.Control.Epoch, p.Control.CoordinationID, p.Eligible, p.Command.SHA256, binding, previous).Error; err != nil {
		return application.TeamQualificationProof{}, err
	}
	stored, err := readTeamQualificationTx(t.db, p.ID)
	if err != nil {
		return stored, err
	}
	if stored.Command.SHA256 != p.Command.SHA256 || stored.PreviousProofID != p.PreviousProofID {
		return application.TeamQualificationProof{}, writefence.ErrConflict
	}
	return stored, nil
}

// ReadAccountQualificationIntent returns a current exact four-writer closure.
func (s *TeamQualificationStore) ReadAccountQualificationIntent(ctx context.Context, id uuid.UUID) (application.AccountQualificationIntent, error) {
	if s == nil || id == uuid.Nil {
		return application.AccountQualificationIntent{}, writefence.ErrInvalid
	}
	var out application.AccountQualificationIntent
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		var err error
		out, err = readAccountQualificationIntentTx(tx, id, true)
		return err
	})
	return out, err
}

// ReadTeamQualification resolves immutable original evidence only. Safety closes
// can advance normally; Creation's exact current local fence decides its usability.
func (s *TeamQualificationStore) ReadTeamQualification(ctx context.Context, id uuid.UUID) (application.TeamQualificationProof, error) {
	if s == nil || id == uuid.Nil {
		return application.TeamQualificationProof{}, writefence.ErrInvalid
	}
	var out application.TeamQualificationProof
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error { var err error; out, err = readTeamQualificationTx(tx, id); return err })
	return out, err
}

func readAccountQualificationIntentTx(tx *gorm.DB, id uuid.UUID, current bool) (application.AccountQualificationIntent, error) {
	var row struct {
		RequestID, TargetID, CloseCoordinationID uuid.UUID
		Action, Outcome                          string
		InputDigest                              []byte
		Candidate                                json.RawMessage
		CloseExpectedEpoch                       int64
	}
	query := `SELECT request_id,target_id,action,outcome,input_digest,candidate,close_coordination_id,close_expected_epoch FROM identity.account_qualification_change WHERE request_id=?`
	if current {
		query += ` FOR UPDATE`
	}
	read := tx.Raw(query, id).Scan(&row)
	if read.Error != nil {
		return application.AccountQualificationIntent{}, read.Error
	}
	if read.RowsAffected != 1 || len(row.InputDigest) != 32 || row.CloseExpectedEpoch < 1 || row.CloseExpectedEpoch == math.MaxInt64 {
		return application.AccountQualificationIntent{}, writefence.ErrUnavailable
	}
	out := application.AccountQualificationIntent{RequestID: row.RequestID, AccountID: row.TargetID, Action: row.Action, InputSHA256: hex.EncodeToString(row.InputDigest), CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: row.TargetID, Epoch: row.CloseExpectedEpoch + 1}, CoordinationID: row.CloseCoordinationID, Blocked: true}}
	if row.Action == "enable" {
		var candidate governanceCandidate
		if err := json.Unmarshal(row.Candidate, &candidate); err != nil {
			return out, err
		}
		if candidate.User.ID != row.TargetID {
			return out, writefence.ErrConflict
		}
		out.EligibleAfter = !candidate.User.EmailVerifiedAt.IsZero() && candidate.User.DeletedAt.IsZero()
	}
	if out.Validate() != nil {
		return application.AccountQualificationIntent{}, writefence.ErrInvalid
	}
	if current {
		if row.Outcome != "pending" {
			return out, writefence.ErrConflict
		}
		if err := requirePasswordControl(tx.Statement.Context, tx, out.CloseControl, true); err != nil {
			return out, err
		}
	}
	return out, nil
}

func readTeamQualificationTx(tx *gorm.DB, id uuid.UUID) (application.TeamQualificationProof, error) {
	var row struct {
		ID, RequestID, TeamID, CandidateID, SourceCoordinationID            uuid.UUID
		TeamSnapshotSHA256, CommandSHA256, AuthoritySetSHA256, TargetWriter string
		QualificationEpoch                                                  int64
		Eligible                                                            bool
		PreviousProofID                                                     *uuid.UUID
	}
	read := tx.Raw(`SELECT id,request_id,team_id,candidate_id,team_snapshot_sha256,qualification_epoch,source_coordination_id,target_writer,eligible,command_sha256,authority_set_sha256,previous_proof_id FROM identity.team_administrator_qualification WHERE id=?`, id).Scan(&row)
	if read.Error != nil {
		return application.TeamQualificationProof{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.TeamQualificationProof{}, writefence.ErrNotFound
	}
	intent, err := readAccountQualificationIntentTx(tx, row.RequestID, false)
	if err != nil {
		return application.TeamQualificationProof{}, err
	}
	out := application.TeamQualificationProof{ID: row.ID, Intent: intent, TeamID: row.TeamID, CandidateID: row.CandidateID, SnapshotSHA256: row.TeamSnapshotSHA256, Eligible: row.Eligible, Control: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: row.CandidateID, Epoch: row.QualificationEpoch}, CoordinationID: row.SourceCoordinationID, Blocked: !row.Eligible}}
	if row.PreviousProofID != nil {
		out.PreviousProofID = *row.PreviousProofID
	}
	action := "identity.team_administrator.ineligible"
	if out.Eligible {
		action = "identity.team_administrator.eligible"
	}
	out.Command = writefence.Command{ID: out.ID, SHA256: row.CommandSHA256, Action: action, Authorities: []writefence.Authority{out.Control.Authority}}
	binding, err := out.Command.AuthorityDigest()
	if err != nil || binding != row.AuthoritySetSHA256 || row.TargetWriter != "creation" || out.Validate() != nil {
		return application.TeamQualificationProof{}, writefence.ErrConflict
	}
	return out, nil
}

var _ application.TeamQualificationStore = (*TeamQualificationStore)(nil)
