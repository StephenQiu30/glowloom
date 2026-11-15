package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// AccountQualificationIntent is an accepted Identity responsibility. It contains
// no actor credential or team fact and is usable only by Creation governance.
type AccountQualificationIntent struct {
	RequestID, AccountID uuid.UUID
	Action, InputSHA256  string
	CloseControl         writefence.Control
	EligibleAfter        bool
}

// Validate binds the original operation and its exact blocked account generation.
func (p AccountQualificationIntent) Validate() error {
	if p.RequestID == uuid.Nil || p.AccountID == uuid.Nil || !qualificationDigest(p.InputSHA256) || p.CloseControl.Validate() != nil || p.CloseControl.Issuer != writefence.Identity || p.CloseControl.Kind != writefence.Account || p.CloseControl.ID != p.AccountID || !p.CloseControl.Blocked {
		return writefence.ErrInvalid
	}
	switch p.Action {
	case "disable", "delete":
		if p.EligibleAfter {
			return writefence.ErrInvalid
		}
	case "enable":
	default:
		return writefence.ErrInvalid
	}
	return nil
}

// TeamQualificationRequest fixes Creation's original team/member candidate set.
// A successor is an explicit fresh qualification read, never a rewritten proof.
type TeamQualificationRequest struct {
	RequestID, TeamID, CandidateID, PreviousProofID uuid.UUID
	SnapshotSHA256                                  string
}

// Validate rejects unbound or caller-invented purpose inputs.
func (r TeamQualificationRequest) Validate() error {
	if r.RequestID == uuid.Nil || r.TeamID == uuid.Nil || r.CandidateID == uuid.Nil || !qualificationDigest(r.SnapshotSHA256) {
		return writefence.ErrInvalid
	}
	return nil
}

// TeamQualificationProof certifies account eligibility only. Creation must still
// lock the exact team/member snapshot and its local candidate fence at the effect.
type TeamQualificationProof struct {
	ID, PreviousProofID uuid.UUID
	Intent              AccountQualificationIntent
	TeamID, CandidateID uuid.UUID
	SnapshotSHA256      string
	Eligible            bool
	Control             writefence.Control
	Command             writefence.Command
}

// Validate checks every original binding, including the distinct candidate,
// immutable command, target-specific purpose and observed source control.
func (p TeamQualificationProof) Validate() error {
	if p.Intent.Validate() != nil || p.ID == uuid.Nil || p.TeamID == uuid.Nil || p.CandidateID == uuid.Nil || p.CandidateID == p.Intent.AccountID || !qualificationDigest(p.SnapshotSHA256) || p.Control.Validate() != nil || p.Control.Issuer != writefence.Identity || p.Control.Kind != writefence.Account || p.Control.ID != p.CandidateID || p.Control.Blocked == p.Eligible || p.PreviousProofID == p.ID {
		return writefence.ErrInvalid
	}
	expected, err := buildTeamQualificationProof(p.Intent, TeamQualificationRequest{RequestID: p.Intent.RequestID, TeamID: p.TeamID, CandidateID: p.CandidateID, SnapshotSHA256: p.SnapshotSHA256, PreviousProofID: p.PreviousProofID}, p.Control, p.Eligible)
	if err != nil || expected.ID != p.ID || expected.Command.ID != p.Command.ID || expected.Command.SHA256 != p.Command.SHA256 || expected.Command.Action != p.Command.Action || len(p.Command.Authorities) != 1 || p.Command.Authorities[0] != p.Control.Authority {
		return writefence.ErrInvalid
	}
	_, err = p.Command.AuthorityDigest()
	return err
}

// TeamQualificationFacts are sampled under Identity policy and account locks.
// Complete is computed from the actual exact source coordination and four ACKs.
type TeamQualificationFacts struct {
	Intent                                       AccountQualificationIntent
	CandidateID                                  uuid.UUID
	Active, Verified, Deleted, Pending, Complete bool
	Control                                      writefence.Control
}

// TeamQualificationTx contains only issuer-local reads and immutable writes.
type TeamQualificationTx interface {
	Facts() TeamQualificationFacts
	Previous() *TeamQualificationProof
	SaveTeamQualification(TeamQualificationProof) (TeamQualificationProof, error)
}

// TeamQualificationStore keeps source policy and positive admission atomic.
// Negative proof records never create a normal open-authority admission.
type TeamQualificationStore interface {
	QualificationTransaction(context.Context, TeamQualificationRequest, func(TeamQualificationTx) error) error
	ReadTeamQualification(context.Context, uuid.UUID) (TeamQualificationProof, error)
	ReadAccountQualificationIntent(context.Context, uuid.UUID) (AccountQualificationIntent, error)
}

// TeamQualifications implements only the LastTeamAdministrator account purpose.
// It cannot turn a candidate UUID into a session or grant team membership.
type TeamQualifications struct{ store TeamQualificationStore }

// NewTeamQualifications binds the issuer-local policy store.
func NewTeamQualifications(store TeamQualificationStore) *TeamQualifications {
	return &TeamQualifications{store: store}
}

// Issue returns current source eligibility while preserving every earlier proof.
// Pending or inconsistent current facts are unavailable, never an ineligible vote.
func (s *TeamQualifications) Issue(ctx context.Context, r TeamQualificationRequest) (TeamQualificationProof, error) {
	if s == nil || s.store == nil {
		return TeamQualificationProof{}, writefence.ErrUnavailable
	}
	if err := r.Validate(); err != nil {
		return TeamQualificationProof{}, err
	}
	var out TeamQualificationProof
	err := s.store.QualificationTransaction(ctx, r, func(tx TeamQualificationTx) error {
		facts := tx.Facts()
		if facts.Intent.Validate() != nil || facts.Intent.RequestID != r.RequestID || facts.CandidateID != r.CandidateID || facts.CandidateID == facts.Intent.AccountID {
			return writefence.ErrInvalid
		}
		if facts.Pending || !facts.Complete {
			return writefence.ErrUnavailable
		}
		eligible := facts.Active && facts.Verified && !facts.Deleted
		if facts.Control.Validate() != nil || facts.Control.Blocked == eligible {
			return writefence.ErrUnavailable
		}
		if r.PreviousProofID != uuid.Nil {
			old := tx.Previous()
			if old == nil || old.Validate() != nil || old.ID != r.PreviousProofID || old.Intent != facts.Intent || old.TeamID != r.TeamID || old.CandidateID != r.CandidateID || old.SnapshotSHA256 != r.SnapshotSHA256 {
				return writefence.ErrConflict
			}
			if old.Control == facts.Control && old.Eligible == eligible {
				out = *old
				return nil
			}
			if facts.Control.Epoch <= old.Control.Epoch {
				return writefence.ErrConflict
			}
		}
		proof, err := buildTeamQualificationProof(facts.Intent, r, facts.Control, eligible)
		if err != nil {
			return err
		}
		if err := proof.Validate(); err != nil {
			return err
		}
		out, err = tx.SaveTeamQualification(proof)
		if err != nil {
			return err
		}
		if out.Validate() != nil || out.ID != proof.ID || out.Command.SHA256 != proof.Command.SHA256 || out.PreviousProofID != proof.PreviousProofID {
			return writefence.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return TeamQualificationProof{}, err
	}
	return out, nil
}

// Resolve reads only an existing exact proof; no source head is initialized,
// reopened or pinned and no new qualification is inferred from a miss.
func (s *TeamQualifications) Resolve(ctx context.Context, id uuid.UUID) (TeamQualificationProof, error) {
	if s == nil || s.store == nil || id == uuid.Nil {
		return TeamQualificationProof{}, writefence.ErrInvalid
	}
	p, err := s.store.ReadTeamQualification(ctx, id)
	if err != nil {
		return p, err
	}
	if p.ID != id || p.Validate() != nil {
		return TeamQualificationProof{}, writefence.ErrUnavailable
	}
	return p, nil
}

// Intent reads the exact durable account closure needed by Creation governance.
func (s *TeamQualifications) Intent(ctx context.Context, id uuid.UUID) (AccountQualificationIntent, error) {
	if s == nil || s.store == nil || id == uuid.Nil {
		return AccountQualificationIntent{}, writefence.ErrInvalid
	}
	p, err := s.store.ReadAccountQualificationIntent(ctx, id)
	if err != nil {
		return p, err
	}
	if p.RequestID != id || p.Validate() != nil {
		return AccountQualificationIntent{}, writefence.ErrUnavailable
	}
	return p, nil
}

func qualificationDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func buildTeamQualificationProof(intent AccountQualificationIntent, r TeamQualificationRequest, control writefence.Control, eligible bool) (TeamQualificationProof, error) {
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("identity.team_administrator/%s/%s/%s/%s/%d/%s/%t", r.RequestID, r.TeamID, r.CandidateID, r.SnapshotSHA256, control.Epoch, control.CoordinationID, eligible)))
	payload := struct {
		Intent              AccountQualificationIntent
		TeamID, CandidateID uuid.UUID
		SnapshotSHA256      string
		Control             writefence.Control
		Eligible            bool
		Target              writefence.Writer
		PreviousProofID     uuid.UUID
	}{intent, r.TeamID, r.CandidateID, r.SnapshotSHA256, control, eligible, writefence.WriterCreation, r.PreviousProofID}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return TeamQualificationProof{}, err
	}
	digest := sha256.Sum256(encoded)
	action := "identity.team_administrator.ineligible"
	if eligible {
		action = "identity.team_administrator.eligible"
	}
	return TeamQualificationProof{ID: id, PreviousProofID: r.PreviousProofID, Intent: intent, TeamID: r.TeamID, CandidateID: r.CandidateID, SnapshotSHA256: r.SnapshotSHA256, Eligible: eligible, Control: control, Command: writefence.Command{ID: id, SHA256: hex.EncodeToString(digest[:]), Action: action, Authorities: []writefence.Authority{control.Authority}}}, nil
}
