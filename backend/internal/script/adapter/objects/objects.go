// Package objects converts storage-specific failures at the private-byte boundary.
package objects

import (
	"errors"
	"io"

	"github.com/minio/minio-go/v7"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, objectstorage.ErrObjectExists) {
		return errors.Join(application.ErrObjectExists, err)
	}
	if errors.Is(err, objectstorage.ErrObjectMissing) {
		return errors.Join(application.ErrObjectMissing, err)
	}
	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.Code == "NotFound" {
		return errors.Join(application.ErrObjectMissing, err)
	}
	return err
}

type privateReader struct{ io.ReadCloser }

func (r *privateReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	return n, storageError(err)
}
