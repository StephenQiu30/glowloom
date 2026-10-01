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
)

// ProjectCopyTransferError separates a known validation failure from an unknown write.
type ProjectCopyTransferError struct {
	Code                string
	NeedsReconciliation bool
	Cause               error
}

func (e *ProjectCopyTransferError) Error() string { return "project copy transfer: " + e.Code }
func (e *ProjectCopyTransferError) Unwrap() error { return e.Cause }

// ProjectCopyObjectRepository commits each receipt through a live workspace worker fence.
// Production wiring must inject the coordinator's fenced transaction bridge;
// a media store on an unrestricted database is not an authorized worker.
type ProjectCopyObjectRepository interface {
	Objects(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) ([]ProjectCopyObject, error)
	RecordObjectDigest(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, uuid.UUID, string, string, int64) error
	BeginObjectWrite(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, uuid.UUID, string) error
	ConfirmObject(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, uuid.UUID, string, string, int64) error
	AuthorizeObjectRemoval(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, uuid.UUID, string) error
	ConfirmObjectRemoved(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, uuid.UUID, string) error
}

// ProjectCopyObjects reads and creates immutable private objects and removes exact owned keys.
// Adapters translate conditional existence into ErrObjectAlreadyExists.
type ProjectCopyObjects interface {
	Get(context.Context, string) (io.ReadCloser, error)
	Exists(context.Context, string) (bool, error)
	PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error
	Remove(context.Context, string) error
}

// ProjectCopyTransfer owns bounded bytes and closes/removes every temporary file.
type ProjectCopyTransfer struct {
	repo    ProjectCopyObjectRepository
	objects ProjectCopyObjects
	tempDir string
}

// NewProjectCopyTransfer injects fenced receipts, object access and private temporary storage.
func NewProjectCopyTransfer(repo ProjectCopyObjectRepository, objects ProjectCopyObjects, tempDir string) *ProjectCopyTransfer {
	return &ProjectCopyTransfer{repo: repo, objects: objects, tempDir: tempDir}
}

type copyContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r copyContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func copyObjectBytes(ctx context.Context, reader io.Reader, output io.Writer, expectedSize *int64, expectedSHA *string) (int64, string, error) {
	limit := int64(2 << 30)
	if expectedSize != nil {
		if *expectedSize < 1 || *expectedSize > limit {
			return 0, "", ErrObjectMismatch
		}
		limit = *expectedSize
	}
	hash := sha256.New()
	size, err := io.CopyBuffer(io.MultiWriter(output, hash), io.LimitReader(copyContextReader{ctx: ctx, reader: reader}, limit+1), make([]byte, 64<<10))
	if err != nil {
		return 0, "", err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if size < 1 || size > limit || expectedSize != nil && size != *expectedSize || expectedSHA != nil && digest != *expectedSHA {
		return 0, "", ErrObjectMismatch
	}
	return size, digest, nil
}

func (s *ProjectCopyTransfer) verifyTarget(ctx context.Context, object ProjectCopyObject) error {
	if object.SHA256 == nil || object.ByteSize == nil || !object.SourceVerified {
		return ErrObjectMismatch
	}
	reader, err := s.objects.Get(ctx, object.TargetObjectKey)
	if err != nil {
		return err
	}
	_, _, verifyErr := copyObjectBytes(ctx, reader, io.Discard, object.ByteSize, object.SHA256)
	return errors.Join(verifyErr, reader.Close())
}

func (s *ProjectCopyTransfer) transferObject(ctx context.Context, actor identityapp.Principal, binding ProjectCopyBinding, snapshot ProjectCopySnapshot, object ProjectCopyObject) (resultErr error) {
	if object.Status == "verified" {
		if err := s.verifyTarget(ctx, object); err != nil {
			return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
		}
		return nil
	}
	if object.Status != "pending" {
		return ErrProjectCopyMediaUnavailable
	}
	if object.WriteStarted {
		if err := s.verifyTarget(ctx, object); err != nil {
			return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
		}
		return s.repo.ConfirmObject(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind, *object.SHA256, *object.ByteSize)
	}
	reader, err := s.objects.Get(ctx, object.SourceObjectKey)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.Close()) }()
	file, err := os.CreateTemp(s.tempDir, "lanverse-copy-*")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close(), os.Remove(file.Name())) }()
	size, digest, err := copyObjectBytes(ctx, reader, file, object.ByteSize, object.SHA256)
	if err != nil {
		return err
	}
	if err := s.repo.RecordObjectDigest(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind, digest, size); err != nil {
		return err
	}
	object.SHA256, object.ByteSize, object.SourceVerified = &digest, &size, true
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := s.repo.BeginObjectWrite(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind); err != nil {
		return err
	}
	err = s.objects.PutIfAbsent(ctx, object.TargetObjectKey, file, size, object.ContentType, digest)
	if err != nil && !errors.Is(err, ErrObjectAlreadyExists) {
		return &ProjectCopyTransferError{Code: "object_write_unknown", NeedsReconciliation: true, Cause: err}
	}
	if err := s.verifyTarget(ctx, object); err != nil {
		return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
	}
	if err := s.repo.ConfirmObject(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind, digest, size); err != nil {
		return &ProjectCopyTransferError{Code: "object_receipt_unknown", NeedsReconciliation: true, Cause: err}
	}
	return nil
}

// Transfer copies the entire frozen object set with receipt-before-write ordering.
// Recovery calls use exactly these same keys after workspace claims reconciliation.
func (s *ProjectCopyTransfer) Transfer(ctx context.Context, actor identityapp.Principal, binding ProjectCopyBinding, snapshot ProjectCopySnapshot) error {
	if s == nil || s.repo == nil || s.objects == nil || binding.Validate() != nil || binding.OrgID != actor.OrgID {
		return ErrUnavailable
	}
	objects, err := s.repo.Objects(ctx, actor, binding, snapshot)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if err := s.transferObject(ctx, actor, binding, snapshot, object); err != nil {
			return fmt.Errorf("copy media object: %w", err)
		}
	}
	return nil
}

// Cleanup removes only this snapshot's private keys after ownership/digest checks.
// Missing objects still require a durable removal receipt; an unknown collision fails closed.
func (s *ProjectCopyTransfer) Cleanup(ctx context.Context, actor identityapp.Principal, binding ProjectCopyBinding, snapshot ProjectCopySnapshot) error {
	if s == nil || s.repo == nil || s.objects == nil || binding.Validate() != nil || binding.OrgID != actor.OrgID {
		return ErrUnavailable
	}
	objects, err := s.repo.Objects(ctx, actor, binding, snapshot)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if err := s.repo.AuthorizeObjectRemoval(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind); err != nil {
			return err
		}
		exists, err := s.objects.Exists(ctx, object.TargetObjectKey)
		if err != nil {
			return err
		}
		if !exists && object.Status == "pending" && object.WriteStarted {
			return &ProjectCopyTransferError{Code: "object_absence_unknown", NeedsReconciliation: true, Cause: ErrObjectMismatch}
		}
		if exists {
			if err := s.verifyTarget(ctx, object); err != nil {
				return &ProjectCopyTransferError{Code: "cleanup_ownership_unverified", NeedsReconciliation: true, Cause: err}
			}
			if err := s.objects.Remove(ctx, object.TargetObjectKey); err != nil {
				return &ProjectCopyTransferError{Code: "object_remove_unknown", NeedsReconciliation: true, Cause: err}
			}
			if exists, err := s.objects.Exists(ctx, object.TargetObjectKey); err != nil || exists {
				return &ProjectCopyTransferError{Code: "object_remove_unverified", NeedsReconciliation: true, Cause: err}
			}
		}
		if err := s.repo.ConfirmObjectRemoved(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind); err != nil {
			return err
		}
	}
	return nil
}
