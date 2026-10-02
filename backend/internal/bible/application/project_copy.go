package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ProjectCopyBinding identifies one trusted workspace job and its private unpublished target.
type ProjectCopyBinding struct{ JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID }

// ProjectCopyAccess rechecks actual admission or the claimed worker within each owning transaction.
type ProjectCopyAccess interface {
	Authorize(context.Context, identityapp.Principal, ProjectCopyBinding, bool) error
}

// ReferenceMapping carries the media owner's exact source and target asset/rendition facts.
// Source revisions remain historical; target revisions belong to the independent copied asset.
type ReferenceMapping struct {
	Source domain.MediaFact `json:"source"`
	Target domain.MediaFact `json:"target"`
}

// ScopeMapping carries the script owner's exact historical formal identity mapping.
type ScopeMapping struct {
	Source domain.LookScope `json:"source"`
	Target domain.LookScope `json:"target"`
}

// ProjectCopyMedia reads only the media owner's frozen copy plan; admission never performs remote I/O.
type ProjectCopyMedia interface {
	FreezeReferences(context.Context, identityapp.Principal, ProjectCopyBinding, []domain.MediaFact, map[uuid.UUID]uuid.UUID) ([]ReferenceMapping, error)
}

// ProjectCopyScopes plans exact script applicability identities before either content owner publishes.
type ProjectCopyScopes interface {
	RemapLookScopes(context.Context, identityapp.Principal, ProjectCopyBinding, []domain.LookScope) ([]ScopeMapping, error)
}

// TransferredReferences verifies actual independent private bytes under the current claimed worker.
// This method runs outside the Bible SQL transaction and cannot publish the target.
type TransferredReferences interface {
	VerifyTransferred(context.Context, identityapp.Principal, ProjectCopyBinding, []ReferenceMapping) error
}

// CopyCounts enumerates every actual historical content entity, excluding commands and execution facts.
type CopyCounts struct {
	Characters             int `json:"characters"`
	CharacterVersions      int `json:"character_versions"`
	CharacterConfirmations int `json:"character_confirmations"`
	Locations              int `json:"locations"`
	LocationVersions       int `json:"location_versions"`
	LocationConfirmations  int `json:"location_confirmations"`
	Props                  int `json:"props"`
	PropVersions           int `json:"prop_versions"`
	PropConfirmations      int `json:"prop_confirmations"`
	Looks                  int `json:"looks"`
	LookVersions           int `json:"look_versions"`
	References             int `json:"references"`
	Voices                 int `json:"voices"`
	Redirects              int `json:"redirects"`
	Splits                 int `json:"splits"`
}

// IdentityMapping remaps one own stable identity with its closed Bible kind.
type IdentityMapping struct {
	Kind     domain.Kind `json:"kind"`
	SourceID uuid.UUID   `json:"source_id"`
	TargetID uuid.UUID   `json:"target_id"`
}

// VersionMapping remaps one own immutable version without mixing identity kinds.
type VersionMapping struct {
	Kind     domain.Kind `json:"kind"`
	SourceID uuid.UUID   `json:"source_id"`
	TargetID uuid.UUID   `json:"target_id"`
}

// ProjectCopySnapshot contains only complete proof/count facts and declared cross-owner identities.
type ProjectCopySnapshot struct {
	ID                            uuid.UUID
	ManifestSHA256, ContentSHA256 string
	Counts                        CopyCounts
	Identities                    []IdentityMapping
	Versions                      []VersionMapping
}

// ProjectCopyReceipt proves that all own target history agrees with the frozen complete manifest.
type ProjectCopyReceipt struct {
	ManifestSHA256, ContentSHA256 string
	Counts                        CopyCounts
}

// CopyConfirmation preserves a kind-specific explicit historical confirmation.
type CopyConfirmation struct {
	Kind         domain.Kind         `json:"kind"`
	Confirmation domain.Confirmation `json:"confirmation"`
}

// CopyLook preserves an actual stable appearance identity and creation time.
type CopyLook struct {
	ID, CharacterID uuid.UUID
	CreatedAt       time.Time
}

// CopyHistory includes complete versions, old heads, historical confirmations and redirect/split records.
type CopyHistory struct {
	Heads         []domain.Head      `json:"heads"`
	Versions      []domain.Version   `json:"versions"`
	Looks         []CopyLook         `json:"looks"`
	Confirmations []CopyConfirmation `json:"confirmations"`
	Redirects     []domain.Redirect  `json:"redirects"`
	Splits        []domain.Split     `json:"splits"`
}

// CopyManifest contains only typed own content and actual foreign owner mappings, never object keys.
type CopyManifest struct {
	Binding       ProjectCopyBinding `json:"binding"`
	Source        CopyHistory        `json:"source"`
	Target        CopyHistory        `json:"target"`
	References    []ReferenceMapping `json:"references"`
	Scopes        []ScopeMapping     `json:"scopes"`
	Counts        CopyCounts         `json:"counts"`
	ContentSHA256 string             `json:"content_sha256"`
	Identities    []IdentityMapping  `json:"identities"`
	Versions      []VersionMapping   `json:"versions"`
}

// ProjectCopyStore owns private content planning/proof and exact caller transaction registration.
type ProjectCopyStore interface {
	ReferencedMedia(context.Context, identityapp.Principal, ProjectCopyBinding) ([]uuid.UUID, error)
	Freeze(context.Context, identityapp.Principal, ProjectCopyBinding, map[uuid.UUID]uuid.UUID, time.Time) (ProjectCopySnapshot, error)
	Manifest(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) (CopyManifest, error)
	ConfirmTransfer(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) error
	Register(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) (ProjectCopyReceipt, error)
	Verify(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) (ProjectCopyReceipt, error)
	Cleanup(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) error
}

// ProjectCopy verifies foreign-owned physical media before SQL-only own history registration.
type ProjectCopy struct {
	store ProjectCopyStore
	media TransferredReferences
}

// ProjectCopyTransferError preserves an unresolved foreign-object proof without exposing private facts.
type ProjectCopyTransferError struct {
	Code                string
	NeedsReconciliation bool
	Cause               error
}

func (e *ProjectCopyTransferError) Error() string { return "Bible copy transfer: " + e.Code }
func (e *ProjectCopyTransferError) Unwrap() error { return e.Cause }

// NewProjectCopy injects the own persistence and actual claimed-worker media verifier.
func NewProjectCopy(store ProjectCopyStore, media TransferredReferences) *ProjectCopy {
	return &ProjectCopy{store: store, media: media}
}

// Transfer verifies actual target bytes, then records proof under a fresh current worker authorization.
func (c *ProjectCopy) Transfer(ctx context.Context, actor identityapp.Principal, b ProjectCopyBinding, s ProjectCopySnapshot) error {
	if c == nil || c.store == nil {
		return ErrUnavailable
	}
	manifest, err := c.store.Manifest(ctx, actor, b, s)
	if err != nil {
		return err
	}
	if len(manifest.References) > 0 {
		if c.media == nil {
			return ErrUnavailable
		}
		if err := c.media.VerifyTransferred(ctx, actor, b, manifest.References); err != nil {
			return &ProjectCopyTransferError{Code: "bible_reference_verification_failed", NeedsReconciliation: true, Cause: err}
		}
	}
	return c.store.ConfirmTransfer(ctx, actor, b, s)
}

// Cleanup seals only unpublished own history; physical objects remain solely media-owned.
func (c *ProjectCopy) Cleanup(ctx context.Context, actor identityapp.Principal, b ProjectCopyBinding, s ProjectCopySnapshot) error {
	if c == nil || c.store == nil {
		return ErrUnavailable
	}
	return c.store.Cleanup(ctx, actor, b, s)
}
