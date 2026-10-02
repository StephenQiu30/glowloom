package application

import (
	"context"
	"errors"
	"io"
	"os"
)

func verifyTransferredObject(ctx context.Context, objects ProjectCopyObjects, object ProjectCopyObject) error {
	if object.SHA256 == nil || object.ByteSize == nil || !object.SourceVerified {
		return ErrObjectMismatch
	}
	reader, err := objects.Get(ctx, object.TargetObjectKey)
	if err != nil {
		return err
	}
	_, _, verifyErr := copyObjectBytes(ctx, reader, io.Discard, object.ByteSize, object.SHA256)
	return errors.Join(verifyErr, reader.Close())
}

func transferPrivateObject(ctx context.Context, objects ProjectCopyObjects, tempDir string, object ProjectCopyObject, recordDigest func(string, int64) error, beginWrite func() error, confirm func(string, int64) error) (resultErr error) {
	if object.Status == "verified" {
		if err := verifyTransferredObject(ctx, objects, object); err != nil {
			return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
		}
		return nil
	}
	if object.Status != "pending" {
		return ErrProjectCopyMediaUnavailable
	}
	if object.WriteStarted {
		if err := verifyTransferredObject(ctx, objects, object); err != nil {
			return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
		}
		return confirm(*object.SHA256, *object.ByteSize)
	}
	reader, err := objects.Get(ctx, object.SourceObjectKey)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.Close()) }()
	file, err := os.CreateTemp(tempDir, "lanverse-copy-*")
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
	err = objects.PutIfAbsent(ctx, object.TargetObjectKey, file, size, object.ContentType, digest)
	if err != nil && !errors.Is(err, ErrObjectAlreadyExists) {
		return &ProjectCopyTransferError{Code: "object_write_unknown", NeedsReconciliation: true, Cause: err}
	}
	if err := verifyTransferredObject(ctx, objects, object); err != nil {
		return &ProjectCopyTransferError{Code: "object_readback_unverified", NeedsReconciliation: true, Cause: err}
	}
	if err := confirm(digest, size); err != nil {
		return &ProjectCopyTransferError{Code: "object_receipt_unknown", NeedsReconciliation: true, Cause: err}
	}
	return nil
}
