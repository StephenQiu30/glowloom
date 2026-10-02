package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// VersionHead contains only current split selection; immutable confirmations retain history.
type VersionHead struct {
	VersionID      uuid.UUID  `json:"version_id"`
	SplitRevision  int64      `json:"split_revision"`
	CandidateSetID uuid.UUID  `json:"candidate_split_set_id"`
	ConfirmedSetID *uuid.UUID `json:"confirmed_split_set_id,omitempty"`
}

// EpisodeView separates proposed boundaries from formal episode identities.
type EpisodeView struct {
	VersionID uuid.UUID        `json:"version_id"`
	Head      VersionHead      `json:"head"`
	Candidate domain.SplitSet  `json:"candidate"`
	Episodes  []domain.Episode `json:"episodes"`
}

// SplitCommand binds every reviewed boundary to both script and split CAS.
type SplitCommand struct {
	ProjectID             uuid.UUID                `json:"project_id"`
	VersionID             uuid.UUID                `json:"version_id"`
	Key                   uuid.UUID                `json:"key"`
	RequestID             uuid.UUID                `json:"request_id"`
	ExpectedRevision      int64                    `json:"expected_revision"`
	ExpectedSplitRevision int64                    `json:"expected_split_revision"`
	CandidateSetID        uuid.UUID                `json:"candidate_set_id"`
	Boundaries            []domain.EpisodeBoundary `json:"boundaries,omitempty"`
	Preface               *domain.ScalarSpan       `json:"preface,omitempty"`
	AckInvalidate         bool                     `json:"ack_invalidate"`
}

// EpisodeMapping makes changed and retained formal identity decisions explicit.
type EpisodeMapping struct {
	PreviousID    *uuid.UUID `json:"previous_episode_id,omitempty"`
	EpisodeID     *uuid.UUID `json:"episode_id,omitempty"`
	InheritStatus string     `json:"inherit_status"`
}

// SplitReceipt is the permanent whole-confirmation result.
type SplitReceipt struct {
	ScriptRevision  int64            `json:"script_revision"`
	ProjectRevision int64            `json:"project_revision"`
	SplitRevision   int64            `json:"split_revision"`
	SplitSetID      uuid.UUID        `json:"split_set_id"`
	ConfirmationID  *uuid.UUID       `json:"confirmation_id,omitempty"`
	Episodes        []domain.Episode `json:"episodes"`
	Mappings        []EpisodeMapping `json:"episode_mappings"`
	RenamedIDs      []uuid.UUID      `json:"renamed_episode_ids"`
}

// StructureCommand retains the complete immutable manual candidate and both heads' CAS.
type StructureCommand struct {
	ProjectID               uuid.UUID                `json:"project_id"`
	EpisodeID               uuid.UUID                `json:"episode_id"`
	Key                     uuid.UUID                `json:"key"`
	RequestID               uuid.UUID                `json:"request_id"`
	ExpectedRevision        int64                    `json:"expected_revision"`
	ExpectedEpisodeRevision int64                    `json:"expected_episode_revision"`
	BaseStructureVersionNo  int64                    `json:"base_structure_version_no"`
	Document                domain.StructureDocument `json:"document"`
	AckInvalidate           bool                     `json:"ack_invalidate"`
}

// StructureReceipt keeps first save/confirm identity and revision facts stable.
type StructureReceipt struct {
	ScriptRevision  int64     `json:"script_revision"`
	ProjectRevision int64     `json:"project_revision"`
	EpisodeRevision int64     `json:"episode_revision"`
	StructureID     uuid.UUID `json:"structure_id"`
	VersionNo       int64     `json:"version_no"`
	ReviewStatus    string    `json:"review_status"`
}

// EpisodeStore owns scoped split/structure transactions and immutable receipts.
type EpisodeStore interface {
	ReplaySplit(context.Context, identityapp.Principal, SplitCommand, string) (*SplitReceipt, error)
	ReplayStructure(context.Context, identityapp.Principal, StructureCommand, string) (*StructureReceipt, error)
	Episodes(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (EpisodeView, error)
	Resplit(context.Context, identityapp.Principal, SplitCommand, domain.SplitResult, time.Time) (SplitReceipt, error)
	ConfirmSplit(context.Context, identityapp.Principal, SplitCommand, time.Time) (SplitReceipt, error)
	Episode(context.Context, identityapp.Principal, uuid.UUID) (domain.Episode, error)
	SaveStructure(context.Context, identityapp.Principal, StructureCommand, time.Time) (StructureReceipt, error)
	ConfirmStructure(context.Context, identityapp.Principal, StructureCommand, time.Time) (StructureReceipt, error)
}

// EpisodeService executes actual rules/manual review without a placeholder model runner.
type EpisodeService struct {
	store   EpisodeStore
	sources *SourceService
	now     func() time.Time
}

// NewEpisodeService injects formal persistence, authorized canonical source bytes and clock.
func NewEpisodeService(store EpisodeStore, sources *SourceService, now func() time.Time) *EpisodeService {
	return &EpisodeService{store: store, sources: sources, now: now}
}

// VersionText fetches and proves the canonical immutable version body.
func (s *SourceService) VersionText(ctx context.Context, actor identityapp.Principal, project, version uuid.UUID) (string, error) {
	if s == nil || s.store == nil || s.objects == nil {
		return "", ErrUnavailable
	}
	base, err := s.store.LoadBase(ctx, actor, project, &version)
	if err != nil {
		return "", err
	}
	if base.Version == nil {
		return "", ErrNotFound
	}
	bytes, err := s.readObject(ctx, base.Version.Text)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Episodes returns only current authorized candidate/formal facts of this immutable version.
func (s *EpisodeService) Episodes(ctx context.Context, actor identityapp.Principal, project, version uuid.UUID) (EpisodeView, error) {
	if s == nil || s.store == nil {
		return EpisodeView{}, ErrUnavailable
	}
	return s.store.Episodes(ctx, actor, project, version)
}

func validSplitCommand(input SplitCommand) bool {
	return input.ProjectID != uuid.Nil && input.VersionID != uuid.Nil && input.Key != uuid.Nil && input.RequestID != uuid.Nil && input.ExpectedRevision > 0 && input.ExpectedSplitRevision >= 0 && input.CandidateSetID != uuid.Nil
}

// Resplit reads actual canonical bytes; it cannot label a model response as rules.
func (s *EpisodeService) Resplit(ctx context.Context, actor identityapp.Principal, input SplitCommand) (SplitReceipt, error) {
	if s == nil || s.store == nil || s.sources == nil || s.now == nil {
		return SplitReceipt{}, ErrUnavailable
	}
	if !validSplitCommand(input) || len(input.Boundaries) != 0 || input.Preface != nil {
		return SplitReceipt{}, domain.ErrInvalidSpan
	}
	prior, err := s.store.ReplaySplit(ctx, actor, input, "resplit")
	if err != nil {
		return SplitReceipt{}, err
	}
	if prior != nil {
		return *prior, nil
	}
	text, err := s.sources.VersionText(ctx, actor, input.ProjectID, input.VersionID)
	if err != nil {
		return SplitReceipt{}, err
	}
	split, err := domain.RulesSplit(text)
	if err != nil {
		return SplitReceipt{}, err
	}
	return s.store.Resplit(ctx, actor, input, split, s.now().UTC().Truncate(time.Microsecond))
}

// ConfirmSplit retains a complete reviewed boundary/identity history.
func (s *EpisodeService) ConfirmSplit(ctx context.Context, actor identityapp.Principal, input SplitCommand) (SplitReceipt, error) {
	if s == nil || s.store == nil || s.now == nil {
		return SplitReceipt{}, ErrUnavailable
	}
	if !validSplitCommand(input) {
		return SplitReceipt{}, domain.ErrInvalidSpan
	}
	prior, err := s.store.ReplaySplit(ctx, actor, input, "confirm_split")
	if err != nil {
		return SplitReceipt{}, err
	}
	if prior != nil {
		return *prior, nil
	}
	return s.store.ConfirmSplit(ctx, actor, input, s.now().UTC().Truncate(time.Microsecond))
}

// SaveStructure preserves explicit scene/action/dialogue interleaving as a new candidate.
func (s *EpisodeService) SaveStructure(ctx context.Context, actor identityapp.Principal, input StructureCommand) (StructureReceipt, error) {
	if s == nil || s.store == nil || s.now == nil {
		return StructureReceipt{}, ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.EpisodeID == uuid.Nil || input.Key == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedEpisodeRevision < 1 || input.BaseStructureVersionNo < 0 {
		return StructureReceipt{}, domain.ErrInvalidStructure
	}
	prior, err := s.store.ReplayStructure(ctx, actor, input, "save_structure")
	if err != nil {
		return StructureReceipt{}, err
	}
	if prior != nil {
		return *prior, nil
	}
	if err := s.proveEpisodeText(ctx, actor, input.ProjectID, input.EpisodeID); err != nil {
		return StructureReceipt{}, err
	}
	return s.store.SaveStructure(ctx, actor, input, s.now().UTC().Truncate(time.Microsecond))
}

// ConfirmStructure publishes only a reviewed current formal structure with owning evidence.
func (s *EpisodeService) ConfirmStructure(ctx context.Context, actor identityapp.Principal, input StructureCommand) (StructureReceipt, error) {
	if s == nil || s.store == nil || s.now == nil {
		return StructureReceipt{}, ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.EpisodeID == uuid.Nil || input.Key == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedEpisodeRevision < 1 {
		return StructureReceipt{}, domain.ErrInvalidStructure
	}
	prior, err := s.store.ReplayStructure(ctx, actor, input, "confirm_structure")
	if err != nil {
		return StructureReceipt{}, err
	}
	if prior != nil {
		return *prior, nil
	}
	if err := s.proveEpisodeText(ctx, actor, input.ProjectID, input.EpisodeID); err != nil {
		return StructureReceipt{}, err
	}
	return s.store.ConfirmStructure(ctx, actor, input, s.now().UTC().Truncate(time.Microsecond))
}

func (s *EpisodeService) proveEpisodeText(ctx context.Context, actor identityapp.Principal, project, episodeID uuid.UUID) error {
	if s.sources == nil {
		return ErrUnavailable
	}
	episode, err := s.store.Episode(ctx, actor, episodeID)
	if err != nil {
		return err
	}
	if episode.ProjectID != project {
		return ErrNotFound
	}
	text, err := s.sources.VersionText(ctx, actor, project, episode.VersionID)
	if err != nil {
		return err
	}
	_, err = domain.ScalarSlice(text, episode.Start, episode.End)
	return err
}
