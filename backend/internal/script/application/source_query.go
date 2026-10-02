package application

import (
	"context"
	"slices"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// SourceSummary is the safe lazy-list metadata, excluding object locations and bodies.
type SourceSummary struct {
	ID           uuid.UUID  `json:"id"`
	LineageID    uuid.UUID  `json:"source_lineage_id"`
	PreviousID   *uuid.UUID `json:"previous_source_id,omitempty"`
	Revision     int64      `json:"source_revision"`
	Kind         string     `json:"source_kind"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	Origin       string     `json:"origin"`
	Position     int        `json:"position"`
	CharCount    int        `json:"char_count"`
	ContentHash  string     `json:"content_hash"`
	RichSHA256   string     `json:"rich_sha256"`
	MediaAssetID *uuid.UUID `json:"media_asset_id,omitempty"`
}

// SourceDetail carries selected editable bodies after a current authorization check.
type SourceDetail struct {
	SourceSummary
	Document   domain.RichDocument     `json:"document"`
	PlainText  string                  `json:"plain_text"`
	Span       domain.SourceSpan       `json:"source_span"`
	Provenance domain.SourceProvenance `json:"provenance"`
}

// WorkspaceView contains only script heads and the current caller's own scope.
type WorkspaceView struct {
	CurrentActorID uuid.UUID           `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID           `json:"current_org_id"`
	State          domain.ProjectState `json:"state"`
}

// SourcePage binds all positions to the immutable version used for this query.
type SourcePage struct {
	VersionID    *uuid.UUID      `json:"version_id,omitempty"`
	Items        []SourceSummary `json:"items"`
	NextPosition *int            `json:"next_position,omitempty"`
}

// Workspace returns revision zero for an empty project without any side effect.
func (s *SourceService) Workspace(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (WorkspaceView, error) {
	if s == nil || s.store == nil {
		return WorkspaceView{}, ErrUnavailable
	}
	base, err := s.store.LoadBase(ctx, actor, project, nil)
	if err != nil {
		return WorkspaceView{}, err
	}
	return WorkspaceView{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, State: base.State}, nil
}

func summary(record domain.SourceRecord, position int) SourceSummary {
	return SourceSummary{ID: record.ID, LineageID: record.LineageID, PreviousID: record.PreviousID, Revision: record.Revision, Kind: record.Kind, Title: record.Title, Status: record.Status, Origin: record.Origin, Position: position, CharCount: record.CharCount, ContentHash: record.ContentHash, RichSHA256: record.Rich.SHA256, MediaAssetID: record.MediaAssetID}
}

// Sources pages frozen metadata; no original or rich bytes are fetched for the list.
func (s *SourceService) Sources(ctx context.Context, actor identityapp.Principal, project uuid.UUID, version *uuid.UUID, after, limit int) (SourcePage, error) {
	if s == nil || s.store == nil {
		return SourcePage{}, ErrUnavailable
	}
	if after < 0 || limit < 1 || limit > 100 {
		return SourcePage{}, domain.ErrInvalidSource
	}
	base, err := s.store.LoadBase(ctx, actor, project, version)
	if err != nil {
		return SourcePage{}, err
	}
	result := SourcePage{Items: make([]SourceSummary, 0)}
	if base.Version == nil {
		return result, nil
	}
	result.VersionID = &base.Version.ID
	if after > len(base.Sources) {
		return SourcePage{}, domain.ErrInvalidSource
	}
	stop := min(len(base.Sources), after+limit)
	for i := after; i < stop; i++ {
		result.Items = append(result.Items, summary(base.Sources[i], i))
	}
	if stop < len(base.Sources) {
		result.NextPosition = &stop
	}
	return result, nil
}

// Source reauthorizes and proves selected private rich bytes before returning text.
func (s *SourceService) Source(ctx context.Context, actor identityapp.Principal, project, lineage uuid.UUID, version *uuid.UUID) (SourceDetail, error) {
	if s == nil || s.store == nil || s.objects == nil {
		return SourceDetail{}, ErrUnavailable
	}
	base, err := s.store.LoadBase(ctx, actor, project, version)
	if err != nil {
		return SourceDetail{}, err
	}
	index := slices.IndexFunc(base.Sources, func(r domain.SourceRecord) bool { return r.LineageID == lineage })
	if index < 0 || base.Version == nil {
		return SourceDetail{}, ErrNotFound
	}
	record := base.Sources[index]
	data, err := s.readObject(ctx, record.Rich)
	if err != nil {
		return SourceDetail{}, err
	}
	document, err := domain.DecodeRichDocument(data)
	if err != nil {
		return SourceDetail{}, err
	}
	plain, err := document.PlainText()
	if err != nil {
		return SourceDetail{}, err
	}
	if domain.ContentSHA([]byte(plain)) != record.ContentHash {
		return SourceDetail{}, ErrObjectMismatch
	}
	spanIndex := slices.IndexFunc(base.Version.Spans, func(s domain.SourceSpan) bool { return s.SourceID == record.ID })
	if spanIndex < 0 {
		return SourceDetail{}, ErrUnavailable
	}
	return SourceDetail{SourceSummary: summary(record, index), Document: document, PlainText: plain, Span: base.Version.Spans[spanIndex], Provenance: record.Provenance}, nil
}
