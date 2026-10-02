package application

import (
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// Validate closes every target identity and object intent before network I/O.
// A hash of frozen JSON alone does not prove its ownership or graph closure.
func (p PackagePlan) Validate() error {
	if p.Version != 1 || p.ActorID == uuid.Nil || p.OrgID == uuid.Nil || p.Key != p.Request.Key || p.RequestID != p.Request.RequestID || p.Request.Validate() != nil || p.CreatedAt.IsZero() ||
		p.JobID != uuid.NewSHA1(p.Key, []byte("media-package/"+p.ActorID.String())) || p.Folders == nil || p.Items == nil || p.Assets == nil || p.Renditions == nil || p.Warnings == nil ||
		len(p.Items) > MaxPackageEntries || len(p.Folders) > MaxPackageEntries || len(p.Objects) != 1+len(p.Assets)+len(p.Renditions) || len(p.Objects) > 15000 ||
		(p.AspectRatio != "16:9" && p.AspectRatio != "9:16" && p.AspectRatio != "1:1") {
		return ErrInvalidPackage
	}
	library, err := p.Request.Scope.Identity(p.OrgID, p.ActorID)
	if err != nil {
		return ErrInvalidPackage
	}
	folders := make(map[uuid.UUID]bool, len(p.Folders))
	for _, f := range p.Folders {
		if f.LibraryID != library || f.Kind != p.Request.Scope.Kind || f.Revision != 1 || !f.CreatedAt.Equal(p.CreatedAt) || !f.UpdatedAt.Equal(p.CreatedAt) {
			return ErrInvalidPackage
		}
		folders[f.ID] = true
	}
	if len(p.Folders) != 0 && domain.ValidateLibraryFolderPlacement(p.Folders[0], p.Folders) != nil {
		return ErrInvalidPackage
	}
	objects := make(map[string]PackageObject, len(p.Objects))
	var size int64
	for _, object := range p.Objects {
		if !packagePath(object.Key, false) || object.ByteSize < 1 || object.ByteSize > MaxPackageExpandedBytes || !copySHA(&object.SHA256) || objects[object.Key].Key != "" {
			return ErrInvalidPackage
		}
		objects[object.Key] = object
		size += object.ByteSize
		if size > MaxPackageExpandedBytes+MaxPackageBytes {
			return ErrInvalidPackage
		}
	}
	archive := p.Objects[0]
	if archive.Key != path.Join("media-packages", p.OrgID.String(), p.ActorID.String(), p.JobID.String(), "source.zip") || archive.MIMEType != "application/zip" || archive.SHA256 != p.Request.ArchiveSHA256 || archive.ByteSize != p.Request.ArchiveBytes {
		return ErrInvalidPackage
	}
	delete(objects, archive.Key)
	assets := make(map[uuid.UUID]domain.MediaAsset, len(p.Assets))
	request := UploadRequest{}
	if p.Request.Scope.Kind == domain.LibraryPersonal {
		request.Personal = &domain.PersonalOwnership{OrgID: p.OrgID, ActorID: p.ActorID}
	} else {
		request.ProjectID = *p.Request.Scope.ProjectID
	}
	for _, a := range p.Assets {
		if a.Validate() != nil || a.Origin != domain.OriginUpload || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed || a.ContainsRealPerson || a.ConsentRecordID != nil || a.SHA256 == nil || a.IsDelete || a.UploadID != nil || a.Revision != 1 || !a.CreateTime.Equal(p.CreatedAt) || !a.UpdateTime.Equal(p.CreatedAt) || assets[a.ID].ID != uuid.Nil || a.ProjectID != request.ProjectID || !packageEqual(a.Personal, request.Personal) {
			return ErrInvalidPackage
		}
		name, err := SafeUploadFileName(a.FileName)
		if err != nil || name != a.FileName || !packageFileKind(string(a.Kind), LibraryPackageFile{MIMEType: a.MimeType, ByteSize: a.ByteSize}) {
			return ErrInvalidPackage
		}
		probe := ProbeResult{Kind: a.Kind, Extension: strings.TrimPrefix(path.Ext(a.ObjectKey), ".")}
		if a.ObjectKey != uploadObjectKey(request, probe, p.CreatedAt, a.ID) {
			return ErrInvalidPackage
		}
		object, found := objects[a.ObjectKey]
		if !found || object.MIMEType != a.MimeType || object.SHA256 != *a.SHA256 || object.ByteSize != a.ByteSize {
			return ErrInvalidPackage
		}
		var review LocalUploadReview
		if decodePackageJSON(a.ModerationDetail, &review) != nil || review.Method != "local_workspace_owner_review" || review.PrincipalID != p.ActorID || !review.ReviewedAt.Equal(p.CreatedAt) || !review.RightsConfirmed || !review.NoAuthorizationRequiredRealPerson || !copySHA(&review.SHA256) {
			return ErrInvalidPackage
		}
		if review.Normalization == nil {
			if review.SHA256 != *a.SHA256 {
				return ErrInvalidPackage
			}
		} else {
			n := review.Normalization
			source := request
			source.Key, source.RequestID, source.SHA256, source.FileName, source.ByteSize = p.Key, p.RequestID, n.Source.SHA256, n.Source.FileName, n.Source.ByteSize
			if review.SHA256 != n.Source.SHA256 || n.Validate(source, a) != nil {
				return ErrInvalidPackage
			}
		}
		delete(objects, a.ObjectKey)
		assets[a.ID] = a
	}
	ids, used := make(map[uuid.UUID]bool, len(p.Items)), make(map[uuid.UUID]bool, len(p.Assets))
	for _, item := range p.Items {
		if item.Validate() != nil || item.LibraryID != library || ids[item.ID] || item.FolderID != nil && !folders[*item.FolderID] || item.Revision != 1 || !item.CreatedAt.Equal(p.CreatedAt) || !item.UpdatedAt.Equal(p.CreatedAt) || p.Request.Scope.Kind == domain.LibraryProject && item.Favorite {
			return ErrInvalidPackage
		}
		ids[item.ID] = true
		if item.AssetID != nil {
			if *item.AssetID != item.ID || assets[item.ID].ID == uuid.Nil || used[item.ID] {
				return ErrInvalidPackage
			}
			used[item.ID] = true
		}
	}
	if len(used) != len(assets) {
		return ErrInvalidPackage
	}
	rends := make(map[uuid.UUID]map[domain.RenditionKind]bool)
	for _, r := range p.Renditions {
		a, found := assets[r.MediaAssetID]
		object, hasObject := objects[r.ObjectKey]
		if !found || r.Validate() != nil || r.IsDelete || r.ID != uuid.NewSHA1(a.ID, []byte("rendition/"+string(r.Kind))) || !r.CreateTime.Equal(p.CreatedAt) || !r.UpdateTime.Equal(p.CreatedAt) || r.ByteSize == nil || !hasObject || object.ByteSize != *r.ByteSize ||
			r.ObjectKey != path.Join(strings.TrimSuffix(a.ObjectKey, path.Ext(a.ObjectKey)), string(r.Kind)+path.Ext(r.ObjectKey)) {
			return ErrInvalidPackage
		}
		if rends[a.ID] == nil {
			rends[a.ID] = make(map[domain.RenditionKind]bool)
		}
		if rends[a.ID][r.Kind] {
			return ErrInvalidPackage
		}
		rends[a.ID][r.Kind] = true
		delete(objects, r.ObjectKey)
	}
	if len(objects) != 0 {
		return ErrInvalidPackage
	}
	for id, a := range assets {
		required := requiredRenditions(a.Kind)
		if a.Kind == domain.KindAudio {
			required = []domain.RenditionKind{domain.RenditionWaveform}
		}
		if len(rends[id]) != len(required) {
			return ErrInvalidPackage
		}
		for _, kind := range required {
			if !rends[id][kind] {
				return ErrInvalidPackage
			}
		}
	}
	return nil
}
