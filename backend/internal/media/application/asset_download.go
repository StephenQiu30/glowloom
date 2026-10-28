package application

import (
	"context"
	"errors"
	"io"
	"mime"
	"path"
	"strings"
	"unicode"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ErrOriginalChanged fences a replacement or revocation during object admission.
var ErrOriginalChanged = errors.New("media original changed")

// AssetOriginal holds one admitted original stream and its public attachment metadata.
// The caller owns Body and must close it, including on an HTTP write failure.
type AssetOriginal struct {
	Body     io.ReadCloser
	FileName string
	MIMEType string
	Size     int64
}

// AssetDownloads reauthorizes a project original after opening its exact saved object.
type AssetDownloads struct {
	store   AssetReader
	objects ObjectReader
	legacy  DocumentObjectGetter
}

// NewAssetDownloads uses persisted routes when present. An explicitly injected
// legacy reader serves only existing rows with no persisted location.
func NewAssetDownloads(store AssetReader, objects ObjectReader, legacy ...DocumentObjectGetter) *AssetDownloads {
	service := &AssetDownloads{store: store, objects: objects}
	if len(legacy) > 0 {
		service.legacy = legacy[0]
	}
	return service
}

func eligibleOriginal(asset domain.MediaAsset, project, id uuid.UUID) bool {
	return asset.ID == id && asset.ProjectID == project && eligible(asset) && asset.Kind != domain.KindModel && asset.ByteSize >= 0 && asset.Revision > 0
}

func sameOriginal(a, b domain.MediaAsset) bool {
	sameLocation := a.ObjectLocation == nil && b.ObjectLocation == nil || sameObjectLocation(a.ObjectLocation, b.ObjectLocation) && a.ObjectLocation.State == b.ObjectLocation.State
	return a.ID == b.ID && a.ProjectID == b.ProjectID && a.Revision == b.Revision && a.Kind == b.Kind && a.ObjectKey == b.ObjectKey && a.ByteSize == b.ByteSize && a.MimeType == b.MimeType && a.FileName == b.FileName && sameLocation
}

type originalStream struct {
	io.Reader
	io.Closer
}

// Open never selects a default route for an existing persisted location. The
// second owner read fences scope/session revocation, replacement and deletion
// before the HTTP handler emits headers or bytes.
func (s *AssetDownloads) Open(ctx context.Context, actor identity.Principal, project, id uuid.UUID) (*AssetOriginal, error) {
	if s == nil || s.store == nil || s.objects == nil && s.legacy == nil {
		return nil, ErrUnavailable
	}
	if project == uuid.Nil || id == uuid.Nil {
		return nil, ErrInvalidQuery
	}
	asset, err := s.store.FindAsset(ctx, actor, project, id)
	if err != nil {
		return nil, err
	}
	if !eligibleOriginal(asset, project, id) {
		return nil, ErrNotFound
	}
	mediaType, _, err := mime.ParseMediaType(asset.MimeType)
	if err != nil {
		return nil, ErrUnavailable
	}
	reader := s.objects
	if asset.ObjectLocation == nil {
		reader = nil
	}
	body, err := readMediaObject(ctx, asset.ObjectLocation, asset.ObjectKey, reader, s.legacy)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	if body == nil {
		return nil, ErrUnavailable
	}
	current, err := s.store.FindAsset(ctx, actor, project, id)
	if err != nil {
		_ = body.Close()
		return nil, err
	}
	if !eligibleOriginal(current, project, id) || !sameOriginal(asset, current) {
		_ = body.Close()
		return nil, ErrOriginalChanged
	}
	if err := ctx.Err(); err != nil {
		_ = body.Close()
		return nil, err
	}
	filename := path.Base(strings.ReplaceAll(asset.FileName, "\\", "/"))
	filename = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '-'
		}
		return r
	}, filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = "original"
	}
	return &AssetOriginal{Body: originalStream{Reader: io.LimitReader(body, asset.ByteSize), Closer: body}, FileName: filename, MIMEType: mediaType, Size: asset.ByteSize}, nil
}
