package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ErrDocumentSourceConflict means current facts no longer match a frozen source.
var ErrDocumentSourceConflict = errors.New("document source changed")

// DocumentSource is an immutable original-file reference without private location.
type DocumentSource struct {
	AssetID   uuid.UUID `json:"asset_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Revision  int64     `json:"revision"`
	SHA256    string    `json:"sha256"`
	ByteSize  int64     `json:"byte_size"`
	MIME      string    `json:"mime"`
	FileName  string    `json:"file_name"`
}

// DocumentSourceStore freezes source rows in an injected owning transaction.
// Implementations hold asset, actor and project SHARE locks until its commit.
type DocumentSourceStore interface {
	FreezeDocumentSources(context.Context, identityapp.Principal, uuid.UUID, []uuid.UUID) ([]domain.MediaAsset, error)
	FindAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.MediaAsset, error)
}

// DocumentObjectGetter reads only a media-owner-selected private object key.
type DocumentObjectGetter interface {
	Get(context.Context, string) (io.ReadCloser, error)
}

// DocumentSources owns authorization and exact bounded original-byte reads.
type DocumentSources struct {
	store   DocumentSourceStore
	objects DocumentObjectGetter
}

// NewDocumentSources injects current project facts and the private object reader.
func NewDocumentSources(store DocumentSourceStore, objects DocumentObjectGetter) *DocumentSources {
	return &DocumentSources{store: store, objects: objects}
}

func documentSource(asset domain.MediaAsset) (DocumentSource, error) {
	if asset.Kind != domain.KindDocument || !asset.CanReference() || asset.ContainsRealPerson || asset.ConsentRecordID != nil || asset.SHA256 == nil {
		return DocumentSource{}, ErrNotFound
	}
	return DocumentSource{AssetID: asset.ID, ProjectID: asset.ProjectID, Revision: asset.Revision, SHA256: *asset.SHA256,
		ByteSize: asset.ByteSize, MIME: asset.MimeType, FileName: asset.FileName}, nil
}

// Freeze returns 1..200 unique eligible sources in the requested order.
// Script injects its admission transaction to retain all row locks through commit.
func (s *DocumentSources) Freeze(ctx context.Context, actor identityapp.Principal, project uuid.UUID, ids []uuid.UUID) ([]DocumentSource, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if project == uuid.Nil || len(ids) < 1 || len(ids) > 200 {
		return nil, ErrInvalidQuery
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			return nil, ErrInvalidQuery
		}
		seen[id] = true
	}
	assets, err := s.store.FreezeDocumentSources(ctx, actor, project, ids)
	if err != nil {
		return nil, err
	}
	if len(assets) != len(ids) {
		return nil, ErrUnavailable
	}
	sources := make(map[uuid.UUID]DocumentSource, len(ids))
	for _, asset := range assets {
		if asset.ProjectID != project || !seen[asset.ID] {
			return nil, ErrUnavailable
		}
		source, err := documentSource(asset)
		if err != nil {
			return nil, err
		}
		if _, found := sources[asset.ID]; found {
			return nil, ErrUnavailable
		}
		sources[asset.ID] = source
	}
	result := make([]DocumentSource, len(ids))
	for i, id := range ids {
		result[i] = sources[id]
	}
	return result, ctx.Err()
}

// Open reauthorizes and compares every frozen fact before fetching private bytes.
// The caller owns the returned temporary original and must Close it.
func (s *DocumentSources) Open(ctx context.Context, actor identityapp.Principal, project uuid.UUID, source DocumentSource) (*Downloaded, error) {
	if s == nil || s.store == nil || s.objects == nil {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if project == uuid.Nil || source.ProjectID != project || source.AssetID == uuid.Nil {
		return nil, ErrNotFound
	}
	asset, err := s.store.FindAsset(ctx, actor, project, source.AssetID)
	if err != nil {
		return nil, err
	}
	current, err := documentSource(asset)
	if err != nil || asset.ProjectID != project || asset.ID != source.AssetID {
		return nil, ErrNotFound
	}
	if current != source {
		return nil, ErrDocumentSourceConflict
	}
	reader, err := s.objects.Get(ctx, asset.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("get document original: %w", errors.Join(ErrUnavailable, err))
	}
	if reader == nil {
		return nil, ErrUnavailable
	}
	defer func() { _ = reader.Close() }()
	file, err := os.CreateTemp("", "lanverse-document-original-*")
	if err != nil {
		return nil, fmt.Errorf("create document staging file: %w", err)
	}
	downloaded := &Downloaded{File: file, MIMEType: current.MIME}
	keep := false
	defer func() {
		if !keep {
			_ = downloaded.Close()
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(uploadContextReader{ctx: ctx, reader: reader}, current.ByteSize+1))
	if err != nil {
		return nil, fmt.Errorf("read document original: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	actualHash := hex.EncodeToString(hash.Sum(nil))
	if size != current.ByteSize || actualHash != current.SHA256 {
		return nil, ErrObjectMismatch
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	downloaded.Size, downloaded.SHA256 = size, actualHash
	keep = true
	return downloaded, nil
}
