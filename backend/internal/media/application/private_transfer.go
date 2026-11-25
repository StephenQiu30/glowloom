package application

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func verifyTransferredObject(ctx context.Context, _ ProjectCopyObjects, physical PhysicalTransferObjects, object ProjectCopyObject) error {
	if object.SHA256 == nil || object.ByteSize == nil || !object.SourceVerified {
		return ErrObjectMismatch
	}
	reader, err := readCopyObject(ctx, physical, object.TargetLocation, object.TargetObjectKey)
	if err != nil {
		return err
	}
	_, _, verifyErr := copyObjectBytes(ctx, reader, io.Discard, object.ByteSize, object.SHA256)
	return errors.Join(verifyErr, reader.Close())
}

func transferPrivateObject(ctx context.Context, objects ProjectCopyObjects, physical PhysicalTransferObjects, tempDir string, object ProjectCopyObject, recordDigest func(string, int64) error, beginWrite func() error, confirm func(string, int64) error) (resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if object.Status == ProjectCopyObjectVerified {
		if err := verifyTransferredObject(ctx, objects, physical, object); err != nil {
			return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
		}
		return nil
	}
	if object.Status != ProjectCopyObjectPending {
		return ErrProjectCopyMediaUnavailable
	}
	if object.WriteStarted {
		if err := verifyTransferredObject(ctx, objects, physical, object); err != nil {
			return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
		}
		return confirm(*object.SHA256, *object.ByteSize)
	}
	reader, err := readCopyObject(ctx, physical, object.SourceLocation, object.SourceObjectKey)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.Close()) }()
	file, err := os.CreateTemp(tempDir, "glowloom-copy-*")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close(), os.Remove(file.Name())) }()
	size, digest, err := copyObjectBytes(ctx, reader, file, object.ByteSize, object.SHA256)
	if err != nil {
		return err
	}
	if err := recordDigest(digest, size); err != nil {
		return err
	}
	object.SHA256, object.ByteSize, object.SourceVerified = &digest, &size, true
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := beginWrite(); err != nil {
		return err
	}
	err = putCopyObject(ctx, physical, object.TargetLocation, object.TargetObjectKey, file, size, object.ContentType, digest)
	if err != nil && !errors.Is(err, ErrObjectAlreadyExists) {
		return &ProjectCopyTransferError{Code: "object_write_unknown", NeedsReconciliation: true, Cause: err}
	}
	if err := verifyTransferredObject(ctx, objects, physical, object); err != nil {
		return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
	}
	if err := confirm(digest, size); err != nil {
		return &ProjectCopyTransferError{Code: "object_receipt_unknown", NeedsReconciliation: true, Cause: err}
	}
	return nil
}

func copyRef(location *domain.ObjectLocation, key string) (domain.ObjectRef, error) {
	if location == nil || location.ID == uuid.Nil || location.Ref.Validate() != nil || location.Ref.Key != key {
		return domain.ObjectRef{}, ErrStorageNotReady
	}
	return location.Ref, nil
}
func readCopyObject(ctx context.Context, physical PhysicalTransferObjects, location *domain.ObjectLocation, key string) (io.ReadCloser, error) {
	ref, err := copyRef(location, key)
	if err != nil || physical == nil {
		return nil, ErrStorageNotReady
	}
	return physical.Get(ctx, ref)
}
func copyObjectExists(ctx context.Context, physical PhysicalTransferObjects, location *domain.ObjectLocation, key string) (bool, error) {
	ref, err := copyRef(location, key)
	if err != nil || physical == nil {
		return false, ErrStorageNotReady
	}
	return physical.Exists(ctx, ref)
}
func putCopyObject(ctx context.Context, physical PhysicalTransferObjects, location *domain.ObjectLocation, key string, reader io.Reader, size int64, mime, sha string) error {
	ref, err := copyRef(location, key)
	if err != nil || physical == nil {
		return ErrStorageNotReady
	}
	return physical.PutIfAbsent(ctx, ref, reader, size, mime, sha)
}
func removeCopyObject(ctx context.Context, physical PhysicalTransferObjects, location *domain.ObjectLocation, key string) error {
	ref, err := copyRef(location, key)
	if err != nil || physical == nil {
		return ErrStorageNotReady
	}
	return physical.Remove(ctx, ref)
}
