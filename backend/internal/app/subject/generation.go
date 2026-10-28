package subject

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	subjectdomain "github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

type generationAccess struct {
	tx      *gorm.DB
	objects Objects
}

// NewGenerationAccess is the operation consumer's exact binding-proof adapter.
func NewGenerationAccess(tx *gorm.DB, objects Objects) operation.SubjectMediaAccess {
	return generationAccess{tx: tx, objects: objects}
}

// NewPromptSources resolves unified subjects without reinterpreting old Bible IDs.
func NewPromptSources(tx *gorm.DB) prompt.SubjectSources { return generationAccess{tx: tx} }

type workerProjectAccess struct{ tx *gorm.DB }

func (w workerProjectAccess) AuthorizeOperation(ctx context.Context, a identity.Principal, p uuid.UUID, write bool) error {
	_, err := scopewire.ProvideWorkerProjectContentAccess(w.tx).Authorize(ctx, a, p, write)
	return err
}
func (w workerProjectAccess) AuthorizeBilling(ctx context.Context, a identity.Principal, p uuid.UUID, write bool) error {
	return w.AuthorizeOperation(ctx, a, p, write)
}

func (g generationAccess) executionActor(ctx context.Context, operationID, projectID uuid.UUID) (identity.Principal, error) {
	store := pgoperation.NewScopedStore(g.tx, nil, func(tx *gorm.DB) operation.ProjectAccess { return workerProjectAccess{tx: tx} })
	return store.SubjectExecutionActor(ctx, operationID, projectID)
}

func (g generationAccess) binding(ctx context.Context, a identity.Principal, projectID, bindingID uuid.UUID) (subject.BoundMedia, error) {
	if a.SessionID != uuid.Nil {
		if _, err := scopewire.ProvideProjectContentAccess(g.tx).Authorize(ctx, a, projectID, false); err != nil {
			return subject.BoundMedia{}, err
		}
		binding, err := NewStore(g.tx, Objects{}).BoundMedia(ctx, a, bindingID)
		if err != nil {
			return binding, err
		}
		if _, err = g.objects.query(g.tx, nil).Bound(ctx, mediaFact(binding.Media)); err != nil {
			return subject.BoundMedia{}, err
		}
		return binding, nil
	}
	if _, err := scopewire.ProvideWorkerProjectContentAccess(g.tx).Authorize(ctx, a, projectID, false); err != nil {
		return subject.BoundMedia{}, err
	}
	binding, err := workerStore(g.tx).BoundMedia(ctx, a, bindingID)
	if err != nil {
		return binding, err
	}
	if _, err = g.objects.query(g.tx, nil).Bound(ctx, mediaFact(binding.Media)); err != nil {
		return subject.BoundMedia{}, err
	}
	return binding, nil
}

func (g generationAccess) ResolveQuote(ctx context.Context, a identity.Principal, projectID, bindingID uuid.UUID) (operation.SubjectMediaFact, error) {
	if a.SessionID == uuid.Nil {
		return operation.SubjectMediaFact{}, operation.ErrFreeQuoteInputNotReady
	}
	b, err := g.binding(ctx, a, projectID, bindingID)
	if err != nil {
		return operation.SubjectMediaFact{}, err
	}
	return operation.SubjectMediaFact{BindingID: b.BindingID, BindingSHA256: b.BindingSHA256, AssetID: b.Media.AssetID, AssetRevision: b.Media.Revision, Kind: b.Media.Kind, SHA256: b.Media.SHA256, ByteSize: b.Media.ByteSize, DurationMS: b.Media.DurationMS}, nil
}

func (g generationAccess) VerifyInput(ctx context.Context, a identity.Principal, op, project uuid.UUID, in operationdomain.OperationInput) error {
	if in.RefType != "subject_media" || in.RefID == nil || in.RefVersion == nil || in.MediaAssetID == nil || in.MaskAssetID != nil {
		return operation.ErrWorkflowInputNotReady
	}
	if a.ID == uuid.Nil {
		var err error
		a, err = g.executionActor(ctx, op, project)
		if err != nil {
			return err
		}
	}
	b, err := g.binding(ctx, a, project, *in.RefID)
	if err != nil {
		return err
	}
	if b.BindingSHA256 != *in.RefVersion || b.Media.AssetID != *in.MediaAssetID {
		return operation.ErrWorkflowInputNotReady
	}
	return nil
}

func (g generationAccess) SignedInputURL(ctx context.Context, op, project uuid.UUID, in operationdomain.OperationInput, ttl time.Duration) (operation.ProviderReference, error) {
	a, err := g.executionActor(ctx, op, project)
	if err != nil {
		return operation.ProviderReference{}, err
	}
	if err = g.VerifyInput(ctx, a, op, project, in); err != nil {
		return operation.ProviderReference{}, err
	}
	b, err := g.binding(ctx, a, project, *in.RefID)
	if err != nil {
		return operation.ProviderReference{}, err
	}
	grant, err := g.objects.query(g.tx, nil).SignBound(ctx, mediaFact(b.Media), ttl)
	if err != nil {
		return operation.ProviderReference{}, err
	}
	return operation.ProviderReference{Role: in.Role, URL: grant.URL, MediaKind: b.Media.Kind}, nil
}

func sourceMedia(binding subjectdomain.Binding) promptdomain.SourceMedia {
	return promptdomain.SourceMedia{SourceID: binding.Media.AssetID.String(), SourceVersion: strconv.FormatInt(binding.Media.Revision, 10), MediaKind: binding.Media.Kind, BindingID: binding.ID.String()}
}

func (g generationAccess) ResolveSubject(ctx context.Context, a identity.Principal, project uuid.UUID, id, version string) (promptdomain.ReferenceSource, bool, error) {
	out := promptdomain.ReferenceSource{Kind: "subject", SourceID: id, SourceVersion: version, Media: []promptdomain.SourceMedia{}}
	if _, err := scopewire.ProvideProjectContentAccess(g.tx).Authorize(ctx, a, project, false); err != nil {
		return out, true, err
	}
	subjectID, e1 := uuid.Parse(id)
	versionID, e2 := uuid.Parse(version)
	if e1 != nil || e2 != nil {
		return out, true, promptdomain.ErrReferenceUnavailable
	}
	v, known, err := NewStore(g.tx, Objects{}).ReadVersionIfKnown(ctx, a, subjectID, versionID)
	if err != nil || !known {
		return out, known, err
	}
	out.Description = v.Content.Name
	if v.Content.Description != "" {
		out.Description += "：" + v.Content.Description
	}
	out.Media = append(out.Media, sourceMedia(v.Content.Primary))
	for _, ref := range v.Content.References {
		out.Media = append(out.Media, sourceMedia(ref))
	}
	if v.Content.Voice != nil {
		out.Media = append(out.Media, sourceMedia(v.Content.Voice.Binding))
	}
	return out, true, nil
}

func (g generationAccess) ValidateBinding(ctx context.Context, a identity.Principal, project uuid.UUID, m promptdomain.SourceMedia, _ bool) error {
	id, err := uuid.Parse(m.BindingID)
	if err != nil {
		return promptdomain.ErrReferenceUnavailable
	}
	b, err := g.binding(ctx, a, project, id)
	if err != nil {
		return err
	}
	if b.Media.AssetID.String() != m.SourceID || strconv.FormatInt(b.Media.Revision, 10) != m.SourceVersion || b.Media.Kind != m.MediaKind {
		return promptdomain.ErrReferenceUnavailable
	}
	return nil
}
