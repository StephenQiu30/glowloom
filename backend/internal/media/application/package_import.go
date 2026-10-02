package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ReadPackageUpload spools a bounded actual ZIP, keeping its immutable input SHA.
func ReadPackageUpload(ctx context.Context, input io.Reader) (*Downloaded, error) {
	if input == nil {
		return nil, ErrInvalidPackage
	}
	f, err := os.CreateTemp("", "lanverse-package-input-*.zip")
	if err != nil {
		return nil, err
	}
	out := &Downloaded{File: f, MIMEType: "application/zip"}
	keep := false
	defer func() {
		if !keep {
			_ = out.Close()
		}
	}()
	h := sha256.New()
	out.Size, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(uploadContextReader{ctx: ctx, reader: input}, MaxPackageBytes+1))
	if err != nil {
		return nil, err
	}
	if out.Size < 1 {
		return nil, ErrInvalidPackage
	}
	if out.Size > MaxPackageBytes {
		return nil, ErrUploadTooLarge
	}
	out.SHA256 = hex.EncodeToString(h.Sum(nil))
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return out, nil
}

// LibraryPackageService uses the same actual upload probe/normalize/render rules.
// A package becomes visible only after every immutable object was read back.
type LibraryPackageService struct {
	repo    PackageRepository
	uploads *UploadService
	objects PackageObjects
	clock   func() time.Time
}

// NewLibraryPackageService explicitly injects current ownership and media tools.
func NewLibraryPackageService(repo PackageRepository, uploads *UploadService, objects PackageObjects, clock func() time.Time) *LibraryPackageService {
	if clock == nil {
		clock = time.Now
	}
	return &LibraryPackageService{repo: repo, uploads: uploads, objects: objects, clock: clock}
}

// Authorize checks current access before reading a potentially large archive.
func (s *LibraryPackageService) Authorize(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, write bool) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	_, err := s.repo.AuthorizePackage(ctx, actor, scope, write)
	return err
}

// Get exposes current durable state with current actor/scope authorization.
func (s *LibraryPackageService) Get(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (PackageJob, error) {
	if s == nil || s.repo == nil {
		return PackageJob{}, ErrUnavailable
	}
	return s.repo.GetPackage(ctx, actor, id)
}

// List returns the current actor's durable import history in one authorized scope.
func (s *LibraryPackageService) List(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, page, size int) (PackagePage, error) {
	if s == nil || s.repo == nil {
		return PackagePage{}, ErrUnavailable
	}
	return s.repo.ListPackages(ctx, actor, scope, page, size)
}

// DecodePackageImportRequest rejects duplicate and unknown fields in multipart JSON.
func DecodePackageImportRequest(data []byte) (PackageImportRequest, error) {
	var in PackageImportRequest
	err := decodePackageJSON(data, &in)
	return in, err
}

// DecodePackageControl rejects duplicate and unknown recovery command fields.
func DecodePackageControl(data []byte) (PackageControl, error) {
	var command PackageControl
	err := decodePackageJSON(data, &command)
	return command, err
}

// Import validates the whole input before durable admission or any object write.
// Unknown I/O returns the original needs_reconciliation job, never partial assets.
func (s *LibraryPackageService) Import(ctx context.Context, actor identityapp.Principal, in PackageImportRequest, file *Downloaded) (PackageJob, error) {
	if s == nil || s.repo == nil || s.uploads == nil || s.objects == nil || file == nil || file.File == nil {
		return PackageJob{}, ErrUnavailable
	}
	in.ArchiveSHA256, in.ArchiveBytes = file.SHA256, file.Size
	if in.Validate() != nil {
		return PackageJob{}, ErrInvalidPackage
	}
	_, err := s.repo.AuthorizePackage(ctx, actor, in.Scope, false)
	if err != nil {
		return PackageJob{}, err
	}
	job, stored, found, err := s.repo.FindPackageImport(ctx, actor, in)
	if err != nil {
		return job, err
	}
	if found {
		current, err := s.repo.GetPackage(ctx, actor, job.ID)
		if err != nil {
			return job, err
		}
		if current.Status != "needs_reconciliation" {
			return job, nil
		}
	}
	aspect, err := s.repo.AuthorizePackage(ctx, actor, in.Scope, true)
	if err != nil {
		return PackageJob{}, err
	}
	now := s.clock().UTC().Truncate(time.Microsecond)
	if found {
		now = stored.CreatedAt
		in.RequestID = stored.RequestID
		aspect = stored.AspectRatio
	}
	prepared, err := s.uploads.preparePackage(ctx, actor, in, file, aspect, now)
	if err != nil {
		return PackageJob{}, err
	}
	defer prepared.close()
	if found {
		if !packageEqual(*stored, prepared.plan) {
			return job, ErrObjectMismatch
		}
	} else {
		job, err = s.repo.AdmitPackageImport(ctx, actor, in, prepared.plan)
		if err != nil {
			return job, err
		}
	}
	return s.repo.WithPackageImport(ctx, actor, job.ID, func(plan PackagePlan) error {
		if !packageEqual(plan, prepared.plan) {
			return ErrObjectMismatch
		}
		return s.writePackageObjects(ctx, plan, prepared.files)
	})
}

// Reconcile resumes from the exact retained source ZIP and immutable output plan.
func (s *LibraryPackageService) Reconcile(ctx context.Context, actor identityapp.Principal, id uuid.UUID, commands ...PackageControl) (PackageJob, error) {
	if s == nil || s.repo == nil || s.uploads == nil || s.objects == nil {
		return PackageJob{}, ErrUnavailable
	}
	command, err := s.packageControl(ctx, actor, id, commands)
	if err != nil {
		return PackageJob{}, err
	}
	return s.repo.ControlPackageImport(ctx, actor, id, command, false, func(plan PackagePlan) error {
		if len(plan.Objects) == 0 {
			return ErrUnavailable
		}
		input, err := s.readPackageObject(ctx, plan.Objects[0])
		if err != nil {
			return err
		}
		defer func() { _ = input.Close() }()
		prepared, err := s.uploads.preparePackage(ctx, actor, plan.Request, input, plan.AspectRatio, plan.CreatedAt)
		if err != nil {
			return err
		}
		defer prepared.close()
		if !packageEqual(plan, prepared.plan) {
			return ErrObjectMismatch
		}
		return s.writePackageObjects(ctx, plan, prepared.files)
	})
}

// Cancel removes only the unpublished batch's predeclared immutable keys.
// Any uncertain removal leaves the batch reserved and recoverable.
func (s *LibraryPackageService) Cancel(ctx context.Context, actor identityapp.Principal, id uuid.UUID, commands ...PackageControl) (PackageJob, error) {
	if s == nil || s.repo == nil || s.objects == nil {
		return PackageJob{}, ErrUnavailable
	}
	command, err := s.packageControl(ctx, actor, id, commands)
	if err != nil {
		return PackageJob{}, err
	}
	return s.repo.ControlPackageImport(ctx, actor, id, command, true, func(plan PackagePlan) error {
		for _, object := range plan.Objects {
			if err := s.objects.Remove(ctx, object.Key); err != nil {
				return err
			}
			exists, err := s.objects.Exists(ctx, object.Key)
			if err != nil {
				return err
			}
			if exists {
				return ErrObjectMismatch
			}
		}
		return nil
	})
}

func (s *LibraryPackageService) packageControl(ctx context.Context, actor identityapp.Principal, id uuid.UUID, commands []PackageControl) (PackageControl, error) {
	if len(commands) > 1 {
		return PackageControl{}, ErrInvalidPackage
	}
	if len(commands) == 1 {
		return commands[0], commands[0].Validate()
	}
	job, err := s.repo.GetPackage(ctx, actor, id)
	if err != nil {
		return PackageControl{}, err
	}
	return PackageControl{ExpectedRevision: job.Revision, Key: uuid.New(), RequestID: uuid.New()}, nil
}

type preparedPackage struct {
	plan  PackagePlan
	files map[string]*Downloaded
	owned []*Downloaded
}

func (p *preparedPackage) close() {
	for _, f := range p.owned {
		_ = f.Close()
	}
}

func (s *UploadService) preparePackage(ctx context.Context, actor identityapp.Principal, in PackageImportRequest, input *Downloaded, aspect string, now time.Time) (*preparedPackage, error) {
	if s == nil || s.prober == nil || s.normalizer == nil || s.renderer == nil || now.IsZero() {
		return nil, ErrUnavailable
	}
	archive, err := ReadLibraryPackage(ctx, input.File, input.Size)
	if err != nil {
		return nil, err
	}
	id := uuid.NewSHA1(in.Key, []byte("media-package/"+actor.ID.String()))
	library, err := in.Scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return nil, err
	}
	p := &preparedPackage{plan: PackagePlan{Version: 1, JobID: id, ActorID: actor.ID, OrgID: actor.OrgID, Key: in.Key, RequestID: in.RequestID, Request: in, CreatedAt: now, AspectRatio: aspect, Folders: []domain.LibraryFolder{}, Items: []domain.LibraryItem{}, Assets: []domain.MediaAsset{}, Renditions: []domain.Rendition{}, Objects: []PackageObject{}, Warnings: append([]PackageWarning{}, archive.Warnings...)}, files: make(map[string]*Downloaded)}
	keep := false
	defer func() {
		if !keep {
			p.close()
		}
	}()
	archiveKey := path.Join("media-packages", actor.OrgID.String(), actor.ID.String(), id.String(), "source.zip")
	p.plan.Objects = append(p.plan.Objects, PackageObject{Key: archiveKey, MIMEType: "application/zip", SHA256: input.SHA256, ByteSize: input.Size})
	p.files[archiveKey] = input
	folders := make(map[uuid.UUID]uuid.UUID, len(archive.Manifest.Folders))
	for _, f := range archive.Manifest.Folders {
		folders[f.ID] = uuid.NewSHA1(id, []byte("folder/"+f.ID.String()))
	}
	for _, f := range archive.Manifest.Folders {
		out := domain.LibraryFolder{ID: folders[f.ID], LibraryID: library, Kind: in.Scope.Kind, Name: f.Name, Style: f.Style, Theme: f.Theme, Position: f.Position, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if f.ParentID != nil {
			parent := folders[*f.ParentID]
			out.ParentID = &parent
		}
		if in.Scope.Kind == domain.LibraryPersonal && (f.Style != "" || f.Theme != "") {
			out.Style, out.Theme = "", ""
			p.plan.Warnings = append(p.plan.Warnings, PackageWarning{ItemID: f.ID.String(), Code: "personal_folder_has_no_appearance"})
		}
		if in.Scope.Kind == domain.LibraryProject && archive.Manifest.LibraryKind == domain.LibraryPersonal {
			out.Style, out.Theme = "cinema", "obsidian"
		}
		p.plan.Folders = append(p.plan.Folders, out)
	}
	if len(p.plan.Folders) > 0 && domain.ValidateLibraryFolderPlacement(p.plan.Folders[0], p.plan.Folders) != nil {
		return nil, ErrInvalidPackage
	}
	for _, entry := range archive.Manifest.Items {
		itemID := uuid.NewSHA1(id, []byte("item/"+entry.ID))
		meta := entry.Metadata
		item := domain.LibraryItem{ID: itemID, LibraryID: library, PlainText: meta.PlainText, Title: meta.Title, Category: meta.Category, Tags: meta.Tags, SourceLabel: meta.SourceLabel, Note: meta.Note, Favorite: meta.Favorite, State: entry.State, TrashedAt: entry.TrashedAt, Position: entry.Position, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if in.Scope.Kind == domain.LibraryProject && item.Favorite {
			item.Favorite = false
			p.plan.Warnings = append(p.plan.Warnings, PackageWarning{ItemID: entry.ID, Code: "personal_favorite_not_project_fact"})
		}
		if meta.FolderID != nil {
			folder := folders[*meta.FolderID]
			item.FolderID = &folder
		}
		if entry.Kind != "text" {
			var expected LibraryPackageFile
			for _, f := range archive.Manifest.Files {
				if f.Path == *entry.FilePath {
					expected = f
					break
				}
			}
			raw, err := archive.Open(ctx, *entry.FilePath)
			if err != nil {
				return nil, err
			}
			actual, err := ReadUpload(ctx, raw.File, expected.FileName)
			closeErr := raw.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				_ = actual.Close()
				return nil, closeErr
			}
			p.owned = append(p.owned, actual)
			if actual.MIMEType != expected.MIMEType || actual.SHA256 != expected.SHA256 || actual.Size != expected.ByteSize {
				return nil, ErrObjectMismatch
			}
			name := expected.FileName
			var normalization *UploadNormalization
			if actual.MIMEType == "video/webm" {
				normalized, err := s.normalizer.Normalize(ctx, actual)
				if err != nil {
					return nil, err
				}
				if normalized.File == nil || normalized.File.File == nil || normalized.File == actual {
					return nil, ErrUnavailable
				}
				p.owned = append(p.owned, normalized.File)
				normalization = &UploadNormalization{Version: 1, Method: "webm_vp8_vp9_to_mp4_h264", Source: UploadSourceFacts{SHA256: actual.SHA256, FileName: name, ByteSize: actual.Size, MIMEType: actual.MIMEType, Codec: normalized.SourceCodec}}
				actual = normalized.File
				name = canonicalUploadName(name)
			}
			probe, err := s.prober.Probe(ctx, actual)
			if err != nil {
				return nil, err
			}
			if !validUploadProbe(actual, probe) || string(probe.Kind) != entry.Kind {
				return nil, ErrUnsupportedUpload
			}
			if normalization != nil {
				normalization.Canonical = UploadCanonicalFacts{SHA256: actual.SHA256, ByteSize: actual.Size, MIMEType: actual.MIMEType, Codec: *probe.Codec, Width: *probe.Width, Height: *probe.Height, DurationMS: *probe.DurationMS}
			}
			rendered, err := s.renderer.Render(ctx, actual, probe, aspect)
			for _, r := range rendered {
				if r.Result != nil {
					p.owned = append(p.owned, r.Result)
				}
			}
			if err != nil {
				return nil, err
			}
			if !completeUploadRendered(probe.Kind, rendered) {
				return nil, ErrUnavailable
			}
			r := UploadRequest{ProjectID: uuid.Nil}
			if in.Scope.Kind == domain.LibraryProject {
				r.ProjectID = *in.Scope.ProjectID
			} else {
				r.Personal = &domain.PersonalOwnership{OrgID: actor.OrgID, ActorID: actor.ID}
			}
			key := uploadObjectKey(r, probe, now, itemID)
			digest := actual.SHA256
			review, err := json.Marshal(LocalUploadReview{Method: "local_workspace_owner_review", PrincipalID: actor.ID, ReviewedAt: now, SHA256: expected.SHA256, RightsConfirmed: true, NoAuthorizationRequiredRealPerson: true, Normalization: normalization})
			if err != nil {
				return nil, err
			}
			asset := domain.MediaAsset{ID: itemID, ProjectID: r.ProjectID, Personal: r.Personal, Kind: probe.Kind, Origin: domain.OriginUpload, Status: domain.StatusReady, ObjectKey: key, FileName: name, MimeType: actual.MIMEType, ByteSize: actual.Size, SHA256: &digest, Width: probe.Width, Height: probe.Height, DurationMS: probe.DurationMS, FPS: probe.FPS, AudioChannels: probe.AudioChannels, Codec: probe.Codec, ModerationStatus: domain.ModerationPassed, ModerationDetail: review, Revision: 1, CreateTime: now, UpdateTime: now}
			if asset.Validate() != nil {
				return nil, ErrInvalidPackage
			}
			p.plan.Assets = append(p.plan.Assets, asset)
			item.AssetID = &itemID
			p.plan.Objects = append(p.plan.Objects, PackageObject{Key: key, MIMEType: actual.MIMEType, SHA256: actual.SHA256, ByteSize: actual.Size})
			p.files[key] = actual
			for _, rend := range rendered {
				key := path.Join(strings.TrimSuffix(asset.ObjectKey, path.Ext(asset.ObjectKey)), string(rend.Kind)+"."+rend.Ext)
				width, height, size := rend.Width, rend.Height, rend.Result.Size
				p.plan.Renditions = append(p.plan.Renditions, domain.Rendition{ID: uuid.NewSHA1(itemID, []byte("rendition/"+string(rend.Kind))), MediaAssetID: itemID, Kind: rend.Kind, ObjectKey: key, Width: &width, Height: &height, ByteSize: &size, CreateTime: now, UpdateTime: now})
				p.plan.Objects = append(p.plan.Objects, PackageObject{Key: key, MIMEType: rend.Result.MIMEType, SHA256: rend.Result.SHA256, ByteSize: size})
				p.files[key] = rend.Result
			}
		}
		if item.Validate() != nil {
			return nil, ErrInvalidPackage
		}
		p.plan.Items = append(p.plan.Items, item)
	}
	if err := p.plan.Validate(); err != nil {
		return nil, err
	}
	keep = true
	return p, nil
}

func (s *LibraryPackageService) writePackageObjects(ctx context.Context, plan PackagePlan, files map[string]*Downloaded) error {
	for _, object := range plan.Objects {
		file := files[object.Key]
		if file == nil || file.File == nil || file.Size != object.ByteSize || file.SHA256 != object.SHA256 || file.MIMEType != object.MIMEType {
			return ErrObjectMismatch
		}
		if _, err := file.File.Seek(0, io.SeekStart); err != nil {
			return err
		}
		err := s.objects.PutIfAbsent(ctx, object.Key, file.File, object.ByteSize, object.MIMEType, object.SHA256)
		if err != nil && !errors.Is(err, ErrObjectAlreadyExists) {
			return err
		}
		verified, err := s.readPackageObject(ctx, object)
		if err != nil {
			return err
		}
		if err := verified.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (s *LibraryPackageService) readPackageObject(ctx context.Context, object PackageObject) (*Downloaded, error) {
	input, err := s.objects.Get(ctx, object.Key)
	if err != nil {
		return nil, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = input.Close()
		}
	}()
	f, err := os.CreateTemp("", "lanverse-package-object-*")
	if err != nil {
		return nil, err
	}
	out := &Downloaded{File: f, MIMEType: object.MIMEType}
	keep := false
	defer func() {
		if !keep {
			_ = out.Close()
		}
	}()
	h := sha256.New()
	out.Size, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(uploadContextReader{ctx: ctx, reader: input}, object.ByteSize+1))
	if err != nil {
		return nil, err
	}
	closed = true
	if err := input.Close(); err != nil {
		return nil, err
	}
	out.SHA256 = hex.EncodeToString(h.Sum(nil))
	if out.Size != object.ByteSize || out.SHA256 != object.SHA256 {
		return nil, ErrObjectMismatch
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return out, nil
}
