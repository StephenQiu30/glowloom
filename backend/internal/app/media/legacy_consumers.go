package media

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// LegacyAssetReader adapts an old Principal consumer at composition only.
// Native Media never imports or constructs this adapter.
func LegacyAssetReader(query *mediaapp.AssetQuery) *LegacyAssetAdapter {
	return &LegacyAssetAdapter{query: query}
}

// LegacyAssetAdapter preserves the existing Principal-based media reader contract.
type LegacyAssetAdapter struct{ query *mediaapp.AssetQuery }

// Reference resolves media through the unchanged owner policy.
func (a LegacyAssetAdapter) Reference(ctx context.Context, p identityapp.Principal, project, asset uuid.UUID) (mediaapp.AssetSummary, error) {
	return a.query.Reference(ctx, Actor(p), project, asset)
}

// LegacyDerivedMedia adapts the existing tool consumer without changing its policy.
func LegacyDerivedMedia(service *mediaapp.DerivedService) *LegacyDerivedAdapter {
	return &LegacyDerivedAdapter{service: service}
}

// LegacyDerivedAdapter projects authenticated facts for existing derived-media consumers.
type LegacyDerivedAdapter struct{ service *mediaapp.DerivedService }

// Asset reads the original asset after owner authorization.
func (a LegacyDerivedAdapter) Asset(ctx context.Context, p identityapp.Principal, project, id uuid.UUID) (mediadomain.MediaAsset, error) {
	return a.service.Asset(ctx, Actor(p), project, id)
}

// Renditions reads verified renditions through the Media owner.
func (a LegacyDerivedAdapter) Renditions(ctx context.Context, p identityapp.Principal, project, id uuid.UUID) ([]mediadomain.Rendition, error) {
	return a.service.Renditions(ctx, Actor(p), project, id)
}

// AuthorizeProject retains the existing read or write authorization.
func (a LegacyDerivedAdapter) AuthorizeProject(ctx context.Context, p identityapp.Principal, project uuid.UUID, write bool) error {
	return a.service.AuthorizeProject(ctx, Actor(p), project, write)
}

// Sources resolves the requested original source assets.
func (a LegacyDerivedAdapter) Sources(ctx context.Context, p identityapp.Principal, project uuid.UUID, ids []uuid.UUID) ([]mediadomain.MediaAsset, error) {
	return a.service.Sources(ctx, Actor(p), project, ids)
}

// Store persists derived metadata through the existing owner transaction.
func (a LegacyDerivedAdapter) Store(ctx context.Context, p identityapp.Principal, asset mediadomain.MediaAsset, rends []mediadomain.Rendition) error {
	return a.service.Store(ctx, Actor(p), asset, rends)
}

// Review applies the original digest-bound review decision.
func (a LegacyDerivedAdapter) Review(ctx context.Context, p identityapp.Principal, project, id uuid.UUID, sha string, now time.Time) error {
	return a.service.Review(ctx, Actor(p), project, id, sha, now)
}

// Reject records the original owner rejection.
func (a LegacyDerivedAdapter) Reject(ctx context.Context, p identityapp.Principal, project, id uuid.UUID, reason string, now time.Time) error {
	return a.service.Reject(ctx, Actor(p), project, id, reason, now)
}

// AuthorizeRemoval retains all original removal checks.
func (a LegacyDerivedAdapter) AuthorizeRemoval(ctx context.Context, p identityapp.Principal, in mediaapp.DerivedRemoval) error {
	return a.service.AuthorizeRemoval(ctx, Actor(p), in)
}

type legacyReferenceReader interface {
	HasMediaReferences(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (bool, error)
}
type legacyMediaReference struct{ owner legacyReferenceReader }

func (a legacyMediaReference) HasMediaReferences(ctx context.Context, p mediaapp.Actor, project, asset uuid.UUID) (bool, error) {
	used, err := a.owner.HasMediaReferences(ctx, IdentityActor(p), project, asset)
	return used, OwningError(err)
}

type scriptReferenceAccess struct {
	access *pgworkspace.ProjectContentAccessStore
}

func (a scriptReferenceAccess) Authorize(ctx context.Context, p scriptapp.Actor, project uuid.UUID, write bool) (scriptapp.ProjectFacts, error) {
	facts, err := a.access.Authorize(ctx, identityapp.Principal{ID: p.ID, OrgID: p.OrgID, ScopeID: p.ScopeID, SessionID: p.SessionID, CredentialEpoch: p.CredentialEpoch}, project, write)
	return scriptapp.ProjectFacts{OwnerScopeID: facts.OwnerScopeID, ProjectID: facts.ProjectID, OrgID: facts.OrgID, Revision: facts.Revision}, scriptReferenceError(err)
}
func (a scriptReferenceAccess) TouchContent(ctx context.Context, p scriptapp.Actor, project uuid.UUID, revision int64) (int64, error) {
	result, err := a.access.TouchContent(ctx, identityapp.Principal{ID: p.ID, OrgID: p.OrgID, ScopeID: p.ScopeID, SessionID: p.SessionID, CredentialEpoch: p.CredentialEpoch}, project, revision)
	return result, scriptReferenceError(err)
}
func scriptReferenceError(err error) error {
	switch {
	case errors.Is(err, identityapp.ErrSessionInvalid):
		return errors.Join(scriptapp.ErrSessionInvalid, err)
	case errors.Is(err, identityapp.ErrForbidden):
		return errors.Join(scriptapp.ErrForbidden, err)
	default:
		return err
	}
}

type scriptReferenceReader interface {
	HasMediaReferences(context.Context, scriptapp.Actor, uuid.UUID, uuid.UUID) (bool, error)
}
type scriptMediaReference struct{ owner scriptReferenceReader }

func (a scriptMediaReference) HasMediaReferences(ctx context.Context, p mediaapp.Actor, project, asset uuid.UUID) (bool, error) {
	return a.owner.HasMediaReferences(ctx, scriptapp.Actor{ID: p.ID, OrgID: p.OrgID, ScopeID: p.ScopeID, SessionID: p.SessionID, CredentialEpoch: p.CredentialEpoch}, project, asset)
}
