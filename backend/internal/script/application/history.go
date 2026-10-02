package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// VersionSummary lists independent immutable source, plain and rich fingerprints.
type VersionSummary struct {
	ID                   uuid.UUID `json:"id"`
	VersionNo            int64     `json:"version_no"`
	ContentHash          string    `json:"content_hash"`
	DocumentSHA256       string    `json:"document_sha256"`
	SourceManifestSHA256 string    `json:"source_manifest_sha256"`
	CharCount            int       `json:"char_count"`
	SourceCount          int       `json:"source_count"`
	CreatedAt            time.Time `json:"created_at"`
}

// VersionPage uses an exclusive immutable version number cursor.
type VersionPage struct {
	Items         []VersionSummary `json:"items"`
	NextVersionNo *int64           `json:"next_version_no,omitempty"`
}

// SourceHistoryPage keeps every replaced or removed snapshot on its stable lineage.
type SourceHistoryPage struct {
	Items        []SourceSummary `json:"items"`
	NextRevision *int64          `json:"next_revision,omitempty"`
}

// SourceSnapshotDetail retains selected rich semantics and legacy HTML evidence.
type SourceSnapshotDetail struct {
	SourceSummary
	Document     domain.RichDocument     `json:"document"`
	PlainText    string                  `json:"plain_text"`
	Provenance   domain.SourceProvenance `json:"provenance"`
	OriginalHTML *string                 `json:"original_html,omitempty"`
}

// StructureSummary exposes immutable review metadata without eagerly fetching a body.
type StructureSummary struct {
	ID         uuid.UUID `json:"id"`
	EpisodeID  uuid.UUID `json:"episode_id"`
	VersionNo  int64     `json:"version_no"`
	SourceHash string    `json:"source_hash"`
	CreatedAt  time.Time `json:"created_at"`
}

// StructurePage lists all manual versions, including superseded candidates.
type StructurePage struct {
	Items         []StructureSummary `json:"items"`
	NextVersionNo *int64             `json:"next_version_no,omitempty"`
}

// EpisodeText is a bounded slice using the immutable version's scalar coordinates.
type EpisodeText struct {
	EpisodeID   uuid.UUID `json:"episode_id"`
	VersionID   uuid.UUID `json:"script_version_id"`
	Start       int       `json:"span_start"`
	End         int       `json:"span_end"`
	ContentHash string    `json:"content_hash"`
	Text        string    `json:"text"`
}

// HistoryStore keeps every query scoped to current owning project authorization.
type HistoryStore interface {
	Confirmations(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, int64, int) (ConfirmationPage, error)
	Confirmation(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (domain.SplitConfirmation, error)
	Versions(context.Context, identityapp.Principal, uuid.UUID, int64, int) (VersionPage, error)
	SourceHistory(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, int64, int) (SourceHistoryPage, error)
	SourceSnapshot(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.SourceRecord, error)
	Structures(context.Context, identityapp.Principal, uuid.UUID, int64, int) (StructurePage, error)
	Structure(context.Context, identityapp.Principal, uuid.UUID, int64) (domain.EpisodeStructure, error)
	Episode(context.Context, identityapp.Principal, uuid.UUID) (domain.Episode, error)
}

// HistoryService proves selected private source bytes and limits scalar reads.
type HistoryService struct {
	store   HistoryStore
	sources *SourceService
}

// NewHistoryService injects authorized history and the existing private-byte reader.
func NewHistoryService(store HistoryStore, sources *SourceService) *HistoryService {
	return &HistoryService{store: store, sources: sources}
}

// Versions is metadata only, with no side effect on empty projects.
func (s *HistoryService) Versions(ctx context.Context, actor identityapp.Principal, project uuid.UUID, before int64, limit int) (VersionPage, error) {
	if s == nil || s.store == nil {
		return VersionPage{}, ErrUnavailable
	}
	if before < 0 || limit < 1 || limit > 100 {
		return VersionPage{}, domain.ErrInvalidSource
	}
	return s.store.Versions(ctx, actor, project, before, limit)
}

// SourceHistory preserves the actual chronological lineage rather than current membership.
func (s *HistoryService) SourceHistory(ctx context.Context, actor identityapp.Principal, project, lineage uuid.UUID, before int64, limit int) (SourceHistoryPage, error) {
	if s == nil || s.store == nil {
		return SourceHistoryPage{}, ErrUnavailable
	}
	if before < 0 || limit < 1 || limit > 100 {
		return SourceHistoryPage{}, domain.ErrInvalidSource
	}
	return s.store.SourceHistory(ctx, actor, project, lineage, before, limit)
}

// SourceSnapshot reads an explicit immutable snapshot, including removed source history.
func (s *HistoryService) SourceSnapshot(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (SourceSnapshotDetail, error) {
	if s == nil || s.store == nil || s.sources == nil {
		return SourceSnapshotDetail{}, ErrUnavailable
	}
	r, err := s.store.SourceSnapshot(ctx, actor, project, id)
	if err != nil {
		return SourceSnapshotDetail{}, err
	}
	bytes, err := s.sources.readObject(ctx, r.Rich)
	if err != nil {
		return SourceSnapshotDetail{}, err
	}
	doc, err := domain.DecodeRichDocument(bytes)
	if err != nil {
		return SourceSnapshotDetail{}, err
	}
	plain, err := doc.PlainText()
	if err != nil {
		return SourceSnapshotDetail{}, err
	}
	if domain.ContentSHA([]byte(plain)) != r.ContentHash {
		return SourceSnapshotDetail{}, ErrObjectMismatch
	}
	out := SourceSnapshotDetail{SourceSummary: summary(r, 0), Document: doc, PlainText: plain, Provenance: r.Provenance}
	if r.Origin == "beeftv" {
		data, err := s.sources.readObject(ctx, r.Original)
		if err != nil {
			return SourceSnapshotDetail{}, err
		}
		html := string(data)
		out.OriginalHTML = &html
	}
	return out, nil
}

// Structures queries immutable versions through the episode's current project scope.
func (s *HistoryService) Structures(ctx context.Context, actor identityapp.Principal, episode uuid.UUID, before int64, limit int) (StructurePage, error) {
	if s == nil || s.store == nil {
		return StructurePage{}, ErrUnavailable
	}
	if before < 0 || limit < 1 || limit > 100 {
		return StructurePage{}, domain.ErrInvalidStructure
	}
	return s.store.Structures(ctx, actor, episode, before, limit)
}

// Structure returns a selected historical version; zero selects the current head.
func (s *HistoryService) Structure(ctx context.Context, actor identityapp.Principal, episode uuid.UUID, version int64) (domain.EpisodeStructure, error) {
	if s == nil || s.store == nil {
		return domain.EpisodeStructure{}, ErrUnavailable
	}
	if version < 0 {
		return domain.EpisodeStructure{}, domain.ErrInvalidStructure
	}
	return s.store.Structure(ctx, actor, episode, version)
}

// EpisodeText rejects invalid or oversized ranges before private object access.
func (s *HistoryService) EpisodeText(ctx context.Context, actor identityapp.Principal, id uuid.UUID, from, to int) (EpisodeText, error) {
	if s == nil || s.store == nil || s.sources == nil {
		return EpisodeText{}, ErrUnavailable
	}
	e, err := s.store.Episode(ctx, actor, id)
	if err != nil {
		return EpisodeText{}, err
	}
	if from < e.Start || to > e.End || from >= to || to-from > 65536 {
		return EpisodeText{}, domain.ErrInvalidSpan
	}
	text, err := s.sources.VersionText(ctx, actor, e.ProjectID, e.VersionID)
	if err != nil {
		return EpisodeText{}, err
	}
	slice, err := domain.ScalarSlice(text, from, to)
	if err != nil {
		return EpisodeText{}, err
	}
	return EpisodeText{EpisodeID: id, VersionID: e.VersionID, Start: from, End: to, ContentHash: domain.ContentSHA([]byte(text)), Text: slice}, nil
}

// Episode exposes authorized formal metadata, including retained deleted history.
func (s *HistoryService) Episode(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (domain.Episode, error) {
	if s == nil || s.store == nil {
		return domain.Episode{}, ErrUnavailable
	}
	return s.store.Episode(ctx, actor, id)
}
