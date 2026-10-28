// Package subject composes subject ownership with media and creation scopes.
package subject

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	pgsubject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/postgres"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	subjectdomain "github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// Objects shares legacy and saved-profile transports without persisting access URLs.
type Objects struct {
	Legacy   *objectstorage.Client
	Routed   *mediaobjects.RoutedReadObjects
	registry *objectstorage.Registry
}

// OpenObjects checks configured profiles without contacting a storage provider.
func OpenObjects(cfg config.Config, legacy *objectstorage.Client) (Objects, error) {
	objects := Objects{Legacy: legacy}
	if cfg.ObjectStorage != nil {
		registry, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
		if err != nil {
			return Objects{}, err
		}
		objects.registry, objects.Routed = registry, mediaobjects.NewRoutedReadObjects(registry)
	}
	return objects, nil
}

// Registry exposes the shared immutable transport to composition roots only.
func (o Objects) Registry() *objectstorage.Registry { return o.registry }

// Close releases only the profile transport owned by this composition.
func (o Objects) Close() {
	if o.registry != nil {
		o.registry.Close()
	}
}

func (o Objects) query(tx *gorm.DB, library *pgmedia.LibraryStore) *media.SubjectSourceQuery {
	reader := pgmedia.NewSubjectSourceStore(tx, library)
	if o.Routed != nil {
		return media.NewRoutedSubjectSourceQuery(reader, o.Routed, o.Routed)
	}
	if o.Legacy != nil {
		return media.NewSubjectSourceQuery(reader, o.Legacy, o.Legacy)
	}
	return media.NewSubjectSourceQuery(reader, nil, nil)
}

type scopeAccess struct {
	tx     *gorm.DB
	worker bool
}

func (s scopeAccess) Resolve(ctx context.Context, actor identity.Principal, write bool) (subject.ScopeFacts, error) {
	var facts subject.ScopeFacts
	if s.worker {
		resolved, err := scopewire.ProvideScopeAccess(s.tx).Resolve(ctx, actor.ID, actor.ScopeID, write)
		if err != nil {
			return facts, err
		}
		facts = subject.ScopeFacts{ScopeID: resolved.ID, PhysicalScopeID: resolved.PhysicalScopeID}
	} else {
		resolved, err := scopewire.ProvideRequestScopeAccess(s.tx).Resolve(ctx, actor, actor.ScopeID, write)
		if err != nil {
			return facts, err
		}
		facts = subject.ScopeFacts{ScopeID: resolved.ID, PhysicalScopeID: resolved.PhysicalScopeID}
	}
	if facts.ScopeID != actor.ScopeID || facts.PhysicalScopeID != actor.OrgID {
		return subject.ScopeFacts{}, subject.ErrNotFound
	}
	return facts, nil
}

// NewStore binds request authority and independently proven source media.
func NewStore(db *gorm.DB, objects Objects) *pgsubject.Store {
	return pgsubject.NewStore(db, pgsubject.Factories{
		Voice: func(tx *gorm.DB) subject.VoiceJobs { return NewVoiceJobs(tx) },
		Scope: func(tx *gorm.DB) subject.ScopeAccess { return scopeAccess{tx: tx} },
		Media: func(tx *gorm.DB) subject.MediaAccess { return sourceAccess{tx: tx, objects: objects} },
	})
}

func workerStore(db *gorm.DB) *pgsubject.Store {
	return pgsubject.NewStore(db, pgsubject.Factories{Scope: func(tx *gorm.DB) subject.ScopeAccess { return scopeAccess{tx: tx, worker: true} }})
}

type sourceAccess struct {
	tx      *gorm.DB
	objects Objects
}

func (s sourceAccess) Reference(ctx context.Context, actor identity.Principal, selection subject.MediaSelection) (subjectdomain.MediaFact, error) {
	if selection.BindingID != nil {
		binding, err := NewStore(s.tx, s.objects).BoundMedia(ctx, actor, *selection.BindingID)
		if err != nil {
			return subjectdomain.MediaFact{}, err
		}
		if binding.Media.AssetID != selection.AssetID {
			return subjectdomain.MediaFact{}, subject.ErrNotFound
		}
		if selection.ProjectID != nil && (binding.Media.SourceProjectID == nil || *selection.ProjectID != *binding.Media.SourceProjectID) {
			return subjectdomain.MediaFact{}, subject.ErrNotFound
		}
		if _, err := s.objects.query(s.tx, nil).VerifyBound(ctx, mediaFact(binding.Media)); err != nil {
			return subjectdomain.MediaFact{}, err
		}
		return binding.Media, nil
	}
	scope := mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: selection.ProjectID}
	if selection.ProjectID == nil {
		scope.Kind = mediadomain.LibraryPersonal
		// The selected source is the caller's own personal library, even when
		// adopting it into a team. The subject save grants only this exact file.
		store := pgworkspace.NewScopeStore(s.tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })
		id, err := store.PersonalScopeID(ctx, actor.ID)
		if err != nil {
			return subjectdomain.MediaFact{}, err
		}
		resolved, err := scopewire.ProvideRequestScopeAccess(s.tx).Resolve(ctx, actor, id, false)
		if err != nil {
			return subjectdomain.MediaFact{}, err
		}
		actor.ScopeID, actor.OrgID = resolved.ID, resolved.PhysicalScopeID
	}
	library := pgmedia.NewScopedLibraryStore(s.tx, sourceProjectFactory, time.Now, sourceScopeFactory)
	file, err := s.objects.query(s.tx, library).Reference(ctx, actor, scope, selection.AssetID)
	if err != nil {
		if errors.Is(err, media.ErrNotFound) {
			return subjectdomain.MediaFact{}, errors.Join(subject.ErrNotFound, err)
		}
		return subjectdomain.MediaFact{}, errors.Join(subject.ErrUnavailable, err)
	}
	a := file.Asset
	fact := subjectdomain.MediaFact{AssetID: a.ID, Revision: a.Revision, Kind: string(a.Kind), SHA256: *a.SHA256, ByteSize: a.ByteSize, MIMEType: a.MimeType, FileName: a.FileName, Width: a.Width, Height: a.Height}
	if a.DurationMS != nil {
		v := int64(*a.DurationMS)
		fact.DurationMS = &v
	}
	if a.Personal != nil {
		fact.PersonalOrgID = &a.Personal.OrgID
		fact.PersonalActorID = &a.Personal.ActorID
	} else {
		id := a.ProjectID
		fact.SourceProjectID = &id
	}
	return fact, nil
}

type sourceAuthority struct{ tx *gorm.DB }

func sourceScopeFactory(tx *gorm.DB) media.ScopeAccess { return sourceAuthority{tx: tx} }
func (s sourceAuthority) CheckActor(ctx context.Context, a identity.Principal) error {
	return pgidentity.NewStore(s.tx).CheckSession(ctx, a, time.Now)
}
func (s sourceAuthority) AuthorizePersonal(ctx context.Context, a identity.Principal, write bool) error {
	scope, err := scopewire.ProvideRequestScopeAccess(s.tx).Resolve(ctx, a, a.ScopeID, write)
	if err != nil {
		return err
	}
	if scope.Kind != "personal" || scope.OwnerUserID == nil || *scope.OwnerUserID != a.ID || scope.PhysicalScopeID != a.OrgID {
		return media.ErrNotFound
	}
	return nil
}
func (s sourceAuthority) AuthorizeProject(ctx context.Context, a identity.Principal, id uuid.UUID, write bool) error {
	_, err := scopewire.ProvideProjectContentAccess(s.tx).Authorize(ctx, a, id, write)
	return err
}
func (s sourceAuthority) AuthorizeCopyTarget(ctx context.Context, a identity.Principal, id uuid.UUID) error {
	return s.AuthorizeProject(ctx, a, id, true)
}

type sourceProjectAccess struct{ tx *gorm.DB }

func sourceProjectFactory(tx *gorm.DB) media.LibraryProjectAccess { return sourceProjectAccess{tx: tx} }
func (s sourceProjectAccess) Authorize(ctx context.Context, a identity.Principal, id uuid.UUID, write bool) (media.LibraryProjectFacts, error) {
	f, err := scopewire.ProvideProjectContentAccess(s.tx).Authorize(ctx, a, id, write)
	return media.LibraryProjectFacts{ProjectID: f.ProjectID, OrgID: f.OrgID, Revision: f.Revision}, err
}
func (s sourceProjectAccess) TouchContent(ctx context.Context, a identity.Principal, id uuid.UUID, rev int64) (int64, error) {
	return scopewire.ProvideProjectContentAccess(s.tx).TouchContent(ctx, a, id, rev)
}

func mediaFact(f subjectdomain.MediaFact) media.SubjectSourceFact {
	out := media.SubjectSourceFact{ReferenceFact: media.ReferenceFact{AssetID: f.AssetID, Revision: f.Revision, Kind: mediadomain.Kind(f.Kind), SHA256: f.SHA256, ByteSize: f.ByteSize}}
	if f.SourceProjectID != nil {
		out.ProjectID = *f.SourceProjectID
	}
	if f.PersonalOrgID != nil && f.PersonalActorID != nil {
		out.Personal = &mediadomain.PersonalOwnership{OrgID: *f.PersonalOrgID, ActorID: *f.PersonalActorID}
	}
	return out
}
