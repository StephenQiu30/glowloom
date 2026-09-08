package application

import (
	"context"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type DraftReader interface {
	Execution(context.Context, domain.Run) (domain.ExecutionSnapshot, error)
	Draft(context.Context, domain.Run, string) (domain.DraftEnvelope, error)
	Resume(context.Context, domain.Run) (domain.ResumeReceipt, error)
}

func (s *ProposalService) Resume(ctx context.Context, actor Actor, runID string, expectedRevision int64) (domain.ResumeReceipt, error) {
	if !validID(runID) || expectedRevision < 1 {
		return domain.ResumeReceipt{}, Problem("validation_failed", 422)
	}
	run, err := s.store.AuthorizedRun(ctx, actor, runID, true)
	if err != nil {
		return domain.ResumeReceipt{}, err
	}
	if run.Command.ActorID != actor.UserID {
		return domain.ResumeReceipt{}, Problem("forbidden", 403)
	}
	if run.Revision != expectedRevision {
		return domain.ResumeReceipt{}, Problem("revision_conflict", 409)
	}
	if run.TokenVersion != actor.TokenVersion {
		return domain.ResumeReceipt{}, Problem("creation_actor_authorization_changed", 409)
	}
	snapshot, err := s.reader.Execution(ctx, run)
	if err != nil {
		return domain.ResumeReceipt{}, err
	}
	if snapshot.RunID != runID || snapshot.CommandID != runID || snapshot.PayloadHash != run.PayloadHash || snapshot.SourceRevisionID != run.Command.Source.RevisionID {
		return domain.ResumeReceipt{}, Problem("execution_binding_mismatch", 502)
	}
	if snapshot.Status != "blocked" || !snapshot.CanResume {
		return domain.ResumeReceipt{}, Problem("creation_not_resumable", 409)
	}
	receipt, err := s.reader.Resume(ctx, run)
	if err != nil {
		return domain.ResumeReceipt{}, err
	}
	if receipt.Schema != "creation-resume-production" || receipt.RunID != runID || receipt.CommandID != runID || receipt.Status != "resume_requested" {
		return domain.ResumeReceipt{}, Problem("resume_receipt_mismatch", 502)
	}
	return receipt, nil
}

type ProposalStore interface {
	AuthorizedRun(context.Context, Actor, string, bool) (domain.Run, error)
	MachineRun(context.Context, string, string) (domain.Run, error)
	SaveProposal(context.Context, Actor, domain.Run, domain.Proposal, time.Time) (domain.Proposal, error)
	Proposals(context.Context, Actor, string) ([]domain.Proposal, error)
	AdoptProposal(context.Context, Actor, string, string, AdoptCommand, time.Time) (domain.AdoptionReceipt, error)
	GateState(context.Context, Actor, string, string, []domain.DraftRef) (domain.GateResult, error)
}
type AdoptCommand struct {
	ExpectedRevision int                     `json:"expected_revision"`
	DecisionID       string                  `json:"decision_id"`
	IdempotencyKey   string                  `json:"idempotency_key"`
	RiskResolutions  []domain.RiskResolution `json:"risk_resolutions"`
}
type SyncResult struct {
	Execution domain.ExecutionSnapshot `json:"execution"`
	Proposals []domain.Proposal        `json:"proposals"`
}
type ResolveCommand struct {
	PayloadHash string            `json:"payload_hash"`
	Drafts      []domain.DraftRef `json:"drafts"`
}
type ProposalService struct {
	store  ProposalStore
	reader DraftReader
	config Config
}

func NewProposalService(store ProposalStore, reader DraftReader, config Config) *ProposalService {
	return &ProposalService{store: store, reader: reader, config: config}
}
func (s *ProposalService) Execution(ctx context.Context, actor Actor, runID string) (domain.ExecutionSnapshot, error) {
	run, err := s.store.AuthorizedRun(ctx, actor, runID, false)
	if err != nil {
		return domain.ExecutionSnapshot{}, err
	}
	value, err := s.reader.Execution(ctx, run)
	if err == nil && (value.RunID != runID || value.CommandID != runID || value.PayloadHash != run.PayloadHash || value.SourceRevisionID != run.Command.Source.RevisionID) {
		return domain.ExecutionSnapshot{}, Problem("execution_binding_mismatch", 502)
	}
	return value, err
}
func (s *ProposalService) Sync(ctx context.Context, actor Actor, runID string) (SyncResult, error) {
	run, err := s.store.AuthorizedRun(ctx, actor, runID, true)
	if err != nil {
		return SyncResult{}, err
	}
	snapshot, err := s.reader.Execution(ctx, run)
	if err != nil {
		return SyncResult{}, err
	}
	if snapshot.RunID != runID || snapshot.CommandID != runID || snapshot.PayloadHash != run.PayloadHash || snapshot.SourceRevisionID != run.Command.Source.RevisionID {
		return SyncResult{}, Problem("execution_binding_mismatch", 502)
	}
	if len(snapshot.Outputs) > 1000 {
		return SyncResult{}, Problem("execution_limit_exceeded", 502)
	}
	for _, ref := range snapshot.Outputs {
		if _, err = s.syncDraft(ctx, actor, run, ref); err != nil {
			return SyncResult{}, err
		}
	}
	proposals, err := s.store.Proposals(ctx, actor, runID)
	return SyncResult{Execution: snapshot, Proposals: proposals}, err
}
func (s *ProposalService) List(ctx context.Context, actor Actor, runID string) ([]domain.Proposal, error) {
	return s.store.Proposals(ctx, actor, runID)
}
func (s *ProposalService) Get(ctx context.Context, actor Actor, runID, proposalID string) (domain.Proposal, error) {
	values, err := s.List(ctx, actor, runID)
	if err != nil {
		return domain.Proposal{}, err
	}
	for _, p := range values {
		if p.ID == proposalID {
			return p, nil
		}
	}
	return domain.Proposal{}, ErrNotFound
}
func (s *ProposalService) Adopt(ctx context.Context, actor Actor, runID, proposalID string, input AdoptCommand) (domain.AdoptionReceipt, error) {
	if !validID(runID) || !validID(proposalID) || !validID(input.DecisionID) || input.ExpectedRevision < 1 || strings.TrimSpace(input.IdempotencyKey) == "" || len(input.IdempotencyKey) > 200 || len(input.RiskResolutions) > 200 {
		return domain.AdoptionReceipt{}, Problem("validation_failed", 422)
	}
	return s.store.AdoptProposal(ctx, actor, runID, proposalID, input, s.config.Now().UTC())
}
func (s *ProposalService) Resolve(ctx context.Context, runID, gate string, input ResolveCommand) (domain.GateResult, error) {
	if !validID(runID) || !domain.ValidStage(gate) || !validHash(input.PayloadHash) || len(input.Drafts) < 1 || len(input.Drafts) > 1000 {
		return domain.GateResult{}, Problem("validation_failed", 422)
	}
	run, err := s.store.MachineRun(ctx, runID, input.PayloadHash)
	if err != nil {
		return domain.GateResult{}, err
	}
	actor := Actor{UserID: run.Command.ActorID, TokenVersion: run.TokenVersion}
	seen := map[string]bool{}
	for _, ref := range input.Drafts {
		if seen[ref.DraftID] {
			return domain.GateResult{}, Problem("duplicate_gate_draft", 422)
		}
		seen[ref.DraftID] = true
		p, loadErr := s.syncDraft(ctx, actor, run, ref)
		if loadErr != nil {
			return domain.GateResult{}, loadErr
		}
		if p.Stage != gate {
			return domain.GateResult{}, Problem("gate_stage_mismatch", 409)
		}
	}
	return s.store.GateState(ctx, actor, runID, gate, input.Drafts)
}
func (s *ProposalService) syncDraft(ctx context.Context, actor Actor, run domain.Run, ref domain.DraftRef) (domain.Proposal, error) {
	if !validID(ref.DraftID) || !validID(ref.StepID) || !validHash(ref.CandidateHash) || !validHash(ref.ResultHash) {
		return domain.Proposal{}, Problem("invalid_draft_reference", 502)
	}
	draft, err := s.reader.Draft(ctx, run, ref.DraftID)
	if err != nil {
		return domain.Proposal{}, err
	}
	if draft.DraftID != ref.DraftID || draft.StepID != ref.StepID || draft.CandidateHash != ref.CandidateHash || draft.ResultHash != ref.ResultHash {
		return domain.Proposal{}, Problem("draft_reference_mismatch", 502)
	}
	proposal, err := domain.ValidateDraft(run, draft)
	if err != nil {
		return domain.Proposal{}, Problem("invalid_draft_evidence", 502)
	}
	return s.store.SaveProposal(ctx, actor, run, proposal, s.config.Now().UTC())
}
