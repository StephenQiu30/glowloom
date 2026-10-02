package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

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
	return verifyTransferredObject(ctx, s.objects, object)
}

func (s *ProjectCopyTransfer) transferObject(ctx context.Context, actor identityapp.Principal, binding ProjectCopyBinding, snapshot ProjectCopySnapshot, object ProjectCopyObject) error {
	return transferPrivateObject(ctx, s.objects, s.tempDir, object,
		func(sha string, size int64) error {
			return s.repo.RecordObjectDigest(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind, sha, size)
		},
		func() error {
			return s.repo.BeginObjectWrite(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind)
		},
		func(sha string, size int64) error {
			return s.repo.ConfirmObject(ctx, actor, binding, snapshot, object.TargetAssetID, object.RenditionKind, sha, size)
		},
	)
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
