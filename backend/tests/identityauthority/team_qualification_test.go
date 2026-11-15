package identityauthority_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type qualificationModel struct {
	identity.TeamQualificationStore
	facts           identity.TeamQualificationFacts
	previous        *identity.TeamQualificationProof
	proofs          map[uuid.UUID]identity.TeamQualificationProof
	saved, admitted int
}

func (m *qualificationModel) QualificationTransaction(_ context.Context, _ identity.TeamQualificationRequest, run func(identity.TeamQualificationTx) error) error {
	return run(m)
}
func (m *qualificationModel) Facts() identity.TeamQualificationFacts     { return m.facts }
func (m *qualificationModel) Previous() *identity.TeamQualificationProof { return m.previous }
func (m *qualificationModel) SaveTeamQualification(p identity.TeamQualificationProof) (identity.TeamQualificationProof, error) {
	m.saved++
	if p.Eligible {
		m.admitted++
	}
	m.proofs[p.ID] = p
	return p, nil
}
func (m *qualificationModel) ReadTeamQualification(_ context.Context, id uuid.UUID) (identity.TeamQualificationProof, error) {
	p, ok := m.proofs[id]
	if !ok {
		return p, writefence.ErrNotFound
	}
	return p, nil
}
func qualificationFixture() (*qualificationModel, identity.TeamQualificationRequest) {
	r := identity.TeamQualificationRequest{RequestID: uuid.New(), TeamID: uuid.New(), CandidateID: uuid.New(), SnapshotSHA256: strings.Repeat("b", 64)}
	target := uuid.New()
	m := &qualificationModel{proofs: map[uuid.UUID]identity.TeamQualificationProof{}}
	m.facts = identity.TeamQualificationFacts{Intent: identity.AccountQualificationIntent{RequestID: r.RequestID, AccountID: target, Action: "delete", InputSHA256: strings.Repeat("a", 64), CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: target, Epoch: 2}, Blocked: true, CoordinationID: uuid.New()}}, CandidateID: r.CandidateID, Active: true, Verified: true, Complete: true, Control: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.CandidateID, Epoch: 1}, CoordinationID: uuid.New()}}
	return m, r
}
func TestTeamQualificationUnknownNeverBecomesNegative(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*identity.TeamQualificationFacts)
	}{
		{"pending", func(f *identity.TeamQualificationFacts) { f.Pending = true }},
		{"missing ACK", func(f *identity.TeamQualificationFacts) { f.Complete = false }},
		{"active closed", func(f *identity.TeamQualificationFacts) { f.Control.Blocked = true }},
		{"inactive open", func(f *identity.TeamQualificationFacts) { f.Active = false }},
	} {
		t.Run(change.name, func(t *testing.T) {
			m, r := qualificationFixture()
			change.apply(&m.facts)
			_, err := identity.NewTeamQualifications(m).Issue(t.Context(), r)
			require.ErrorIs(t, err, writefence.ErrUnavailable)
			require.Zero(t, m.saved)
			require.Zero(t, m.admitted)
		})
	}
}
func TestTeamQualificationNegativeDoesNotAdmitAndSuccessorCannotRewriteOldProof(t *testing.T) {
	m, r := qualificationFixture()
	s := identity.NewTeamQualifications(m)
	old, err := s.Issue(t.Context(), r)
	require.NoError(t, err)
	require.True(t, old.Eligible)
	require.Equal(t, 1, m.admitted)
	m.previous = &old
	r.PreviousProofID = old.ID
	m.facts.Active = false
	m.facts.Control.Epoch = 2
	m.facts.Control.Blocked = true
	m.facts.Control.CoordinationID = uuid.New()
	negative, err := s.Issue(t.Context(), r)
	require.NoError(t, err)
	require.False(t, negative.Eligible)
	require.Equal(t, 1, m.admitted)
	require.NotEqual(t, old.ID, negative.ID)
	require.Equal(t, old.ID, negative.PreviousProofID)
	stored, err := s.Resolve(t.Context(), old.ID)
	require.NoError(t, err)
	require.Equal(t, old.Command, stored.Command)
	require.True(t, stored.Eligible)
	r.CandidateID = uuid.New()
	m.facts.CandidateID = r.CandidateID
	m.facts.Control.ID = r.CandidateID
	_, err = s.Issue(t.Context(), r)
	require.ErrorIs(t, err, writefence.ErrConflict)
}
