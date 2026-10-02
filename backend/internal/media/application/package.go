package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ErrPackageConflict preserves a stale scope or a changed permanent request.
var ErrPackageConflict = errors.New("media package revision or request conflict")

// PackageImportRequest is a closed scope and byte-bound human declaration.
type PackageImportRequest struct {
	Scope                   domain.LibraryScope `json:"scope"`
	ExpectedRevision        int64               `json:"expected_revision"`
	ExpectedProjectRevision int64               `json:"expected_project_revision"`
	LocalReviewConfirmed    bool                `json:"local_review_confirmed"`
	Key                     uuid.UUID           `json:"-"`
	RequestID               uuid.UUID           `json:"-"`
	ArchiveSHA256           string              `json:"-"`
	ArchiveBytes            int64               `json:"-"`
}

// Validate rejects any ambiguous target before archive processing.
func (r PackageImportRequest) Validate() error {
	if r.Scope.Validate() != nil || r.Key == uuid.Nil || r.RequestID == uuid.Nil || !r.LocalReviewConfirmed ||
		r.ExpectedRevision < 0 || r.ExpectedRevision > 2147483646 || r.ArchiveBytes < 1 || r.ArchiveBytes > MaxPackageBytes || !copySHA(&r.ArchiveSHA256) ||
		r.Scope.Kind == domain.LibraryProject && r.ExpectedProjectRevision < 1 || r.Scope.Kind == domain.LibraryPersonal && r.ExpectedProjectRevision != 0 {
		return ErrInvalidPackage
	}
	return nil
}

// PackageJob has no private object paths, source URLs or alleged consent facts.
type PackageJob struct {
	ID              uuid.UUID           `json:"id"`
	Scope           domain.LibraryScope `json:"scope"`
	Status          string              `json:"status"`
	Revision        int64               `json:"revision"`
	ItemCount       int                 `json:"item_count"`
	FolderCount     int                 `json:"folder_count"`
	Warnings        []PackageWarning    `json:"warnings"`
	LibraryRevision int64               `json:"library_revision"`
	ProjectRevision int64               `json:"project_revision"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// PackageControl binds an explicit recovery/cancellation to the displayed head.
type PackageControl struct {
	ExpectedRevision int64     `json:"expected_revision"`
	Key              uuid.UUID `json:"-"`
	RequestID        uuid.UUID `json:"-"`
}

// PackagePage is a current-identity, fully scoped durable import history.
type PackagePage struct {
	CurrentActorID uuid.UUID           `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID           `json:"current_org_id"`
	Scope          domain.LibraryScope `json:"scope"`
	Page           int                 `json:"page"`
	PageSize       int                 `json:"page_size"`
	Total          int64               `json:"total"`
	Jobs           []PackageJob        `json:"jobs"`
}

// Validate rejects missing public command identity or a stale-shaped head.
func (c PackageControl) Validate() error {
	if c.Key == uuid.Nil || c.RequestID == uuid.Nil || c.ExpectedRevision < 1 {
		return ErrInvalidPackage
	}
	return nil
}

// PackageObject is immutable, private write intent before any network write.
type PackageObject struct {
	Key      string `json:"key"`
	MIMEType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	ByteSize int64  `json:"byte_size"`
}

// PackagePlan contains only media-owned target facts and complete byte intents.
// Its archive is retained for recovery; it never becomes an ordinary asset.
type PackagePlan struct {
	Version     int                    `json:"version"`
	JobID       uuid.UUID              `json:"job_id"`
	ActorID     uuid.UUID              `json:"actor_id"`
	OrgID       uuid.UUID              `json:"org_id"`
	Key         uuid.UUID              `json:"key"`
	RequestID   uuid.UUID              `json:"request_id"`
	Request     PackageImportRequest   `json:"request"`
	CreatedAt   time.Time              `json:"created_at"`
	AspectRatio string                 `json:"aspect_ratio"`
	Folders     []domain.LibraryFolder `json:"folders"`
	Items       []domain.LibraryItem   `json:"items"`
	Assets      []domain.MediaAsset    `json:"assets"`
	Renditions  []domain.Rendition     `json:"renditions"`
	Objects     []PackageObject        `json:"objects"`
	Warnings    []PackageWarning       `json:"warnings"`
}

// PackageRepository serializes current authorization with complete publication.
// Admission is committed before any object I/O. WithPackageImport retains the
// scope/job locks until the callback and one atomic publication transaction end.
type PackageRepository interface {
	AuthorizePackage(context.Context, identityapp.Principal, domain.LibraryScope, bool) (string, error)
	FindPackageImport(context.Context, identityapp.Principal, PackageImportRequest) (PackageJob, *PackagePlan, bool, error)
	AdmitPackageImport(context.Context, identityapp.Principal, PackageImportRequest, PackagePlan) (PackageJob, error)
	WithPackageImport(context.Context, identityapp.Principal, uuid.UUID, func(PackagePlan) error) (PackageJob, error)
	ControlPackageImport(context.Context, identityapp.Principal, uuid.UUID, PackageControl, bool, func(PackagePlan) error) (PackageJob, error)
	GetPackage(context.Context, identityapp.Principal, uuid.UUID) (PackageJob, error)
	ListPackages(context.Context, identityapp.Principal, domain.LibraryScope, int, int) (PackagePage, error)
	InspectPackageExport(context.Context, identityapp.Principal, domain.LibraryScope, func(PackageExport) error) error
}

// PackageExport freezes the complete scoped catalog and original-file facts.
type PackageExport struct {
	Manifest LibraryPackageManifest
	Objects  map[string]PackageObject
}

// PackageObjects permits only server-frozen private keys, never package URLs.
type PackageObjects interface {
	ProjectCopyObjects
}

func packageEqual(a, b any) bool {
	x, xe := json.Marshal(a)
	y, ye := json.Marshal(b)
	return xe == nil && ye == nil && string(x) == string(y)
}
