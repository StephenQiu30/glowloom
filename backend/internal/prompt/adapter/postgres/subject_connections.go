package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// resolveGraphSubjectPins expands only references derived from the locked graph.
// Ordinary user-authored references continue to require complete exact source facts.
func (s *DraftStore) resolveGraphSubjectPins(ctx context.Context, tx *gorm.DB, actor identity.Principal, project uuid.UUID, graph canvas.Document, node canvas.Node, draft *domain.Draft) error {
	refs, err := canvas.SubjectConnectionReferences(graph, node)
	if err != nil {
		return err
	}
	owner := s.sources(tx)
	for _, ref := range refs {
		if ref.Source.Kind != "subject" {
			continue
		}
		if owner == nil {
			return domain.ErrReferenceUnavailable
		}
		expanded, known, err := owner.ResolveSubject(ctx, actor, project, ref.Source.SourceID, ref.Source.SourceVersion)
		if err != nil {
			return err
		}
		if !known || expanded.Kind != "subject" || expanded.SourceID != ref.Source.SourceID || expanded.SourceVersion != ref.Source.SourceVersion || len(expanded.Media) == 0 {
			return domain.ErrReferenceUnavailable
		}
		found := false
		for i := range draft.References {
			current := &draft.References[i]
			if current.ID != ref.ID {
				continue
			}
			if current.Source.Kind != "subject" || current.Source.SourceID != ref.Source.SourceID || current.Source.SourceVersion != ref.Source.SourceVersion || current.Source.Description != "" || len(current.Source.Media) != 0 {
				return domain.ErrReferenceUnavailable
			}
			current.Source = expanded
			current.MediaKind = expanded.Media[0].MediaKind
			found = true
		}
		if !found {
			return domain.ErrReferenceUnavailable
		}
	}
	return draft.Validate()
}
