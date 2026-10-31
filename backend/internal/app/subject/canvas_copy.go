package subject

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// NewCanvasCopySources retains fixed subject identities only inside the copy
// coordinator's transaction after both projects prove the same current scope.
func NewCanvasCopySources(tx *gorm.DB, binding canvas.ProjectCopyBinding) prompt.SubjectSources {
	return canvasCopySources{tx: tx, binding: binding}
}

type canvasCopySources struct {
	tx      *gorm.DB
	binding canvas.ProjectCopyBinding
}

func (s canvasCopySources) authorize(ctx context.Context, actor identity.Principal, project uuid.UUID) (identity.Principal, error) {
	if s.binding.Validate() != nil || actor.OrgID != s.binding.OrgID || project != s.binding.SourceProjectID && project != s.binding.TargetProjectID {
		return actor, promptdomain.ErrReferenceUnavailable
	}
	access := scopewire.ProvideProjectContentAccess(s.tx)
	if actor.SessionID == uuid.Nil {
		access = scopewire.ProvideWorkerProjectContentAccess(s.tx)
	}
	source, err := access.Authorize(ctx, actor, s.binding.SourceProjectID, false)
	if err != nil {
		return actor, err
	}
	target, err := access.AuthorizeCopyTarget(ctx, actor, s.binding.TargetProjectID)
	if err != nil {
		return actor, err
	}
	if source.OwnerScopeID == uuid.Nil || source.OwnerScopeID != target.OwnerScopeID || actor.ScopeID != uuid.Nil && actor.ScopeID != source.OwnerScopeID {
		return actor, promptdomain.ErrReferenceUnavailable
	}
	actor.ScopeID = source.OwnerScopeID
	return actor, nil
}

func (s canvasCopySources) ResolveSubject(ctx context.Context, actor identity.Principal, project uuid.UUID, id, version string) (promptdomain.ReferenceSource, bool, error) {
	out := promptdomain.ReferenceSource{Kind: "subject", SourceID: id, SourceVersion: version, Media: []promptdomain.SourceMedia{}}
	actor, err := s.authorize(ctx, actor, project)
	if err != nil {
		return out, true, err
	}
	subjectID, e1 := uuid.Parse(id)
	versionID, e2 := uuid.Parse(version)
	if e1 != nil || e2 != nil {
		return out, true, promptdomain.ErrReferenceUnavailable
	}
	store := NewStore(s.tx, Objects{})
	if actor.SessionID == uuid.Nil {
		store = workerStore(s.tx)
	}
	v, known, err := store.ReadVersionIfKnown(ctx, actor, subjectID, versionID)
	if err != nil || !known {
		return out, known, err
	}
	fillSubjectSource(&out, v.Content)
	return out, true, nil
}

func (s canvasCopySources) ValidateBinding(ctx context.Context, actor identity.Principal, project uuid.UUID, media promptdomain.SourceMedia, ready bool) error {
	actor, err := s.authorize(ctx, actor, project)
	if err != nil {
		return err
	}
	return (generationAccess{tx: s.tx}).ValidateBinding(ctx, actor, s.binding.SourceProjectID, media, ready)
}
