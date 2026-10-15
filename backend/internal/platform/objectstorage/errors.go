package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
	cos "github.com/tencentyun/cos-go-sdk-v5"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

var (
	// ErrObjectMissing is confirmed object absence, never a denied request.
	ErrObjectMissing = errors.New("object storage object missing")
	// ErrAccessDenied identifies insufficient credentials or profile permissions.
	ErrAccessDenied = errors.New("object storage access denied")
	// ErrUnsupportedCapability prevents silently weakening a storage guarantee.
	ErrUnsupportedCapability = errors.New("object storage capability unavailable")
	// ErrOutcomeUnknown requires reconciliation before destructive compensation.
	ErrOutcomeUnknown = errors.New("object storage operation outcome unknown")
)

// providerError retains SDK diagnostics for errors.As without printing SDK URLs,
// response messages, credentials or signatures in ordinary application logs.
type providerError struct {
	op, profile string
	kind, cause error
}

func (e *providerError) Error() string {
	return fmt.Sprintf("object storage %s (%s): %s", e.op, e.profile, e.kind)
}
func (e *providerError) Unwrap() []error            { return []error{e.kind, e.cause} }
func (e *providerError) Format(s fmt.State, _ rune) { _, _ = fmt.Fprint(s, e.Error()) }

func storageError(op, profile string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) {
		return err
	}
	kind := ErrOutcomeUnknown
	status, code := 0, ""
	var m minio.ErrorResponse
	var c *cos.ErrorResponse
	var t *tos.TosServerError
	if errors.As(err, &m) {
		status, code = m.StatusCode, m.Code
	}
	if errors.As(err, &c) {
		code = c.Code
		if c.Response != nil {
			status = c.Response.StatusCode
		}
	}
	if errors.As(err, &t) {
		status, code = t.StatusCode, t.Code
	}
	switch {
	case errors.Is(err, context.Canceled):
		kind = context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		kind = context.DeadlineExceeded
	case errors.Is(err, ErrObjectExists):
		kind = ErrObjectExists
	case errors.Is(err, ErrObjectMissing):
		kind = ErrObjectMissing
	case errors.Is(err, ErrInventoryLimit):
		kind = ErrInventoryLimit
	case errors.Is(err, ErrInvalidObject):
		kind = ErrInvalidObject
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		kind = ErrAccessDenied
	case status == http.StatusPreconditionFailed || (status == http.StatusConflict && (code == "ObjectAlreadyExists" || code == "FileAlreadyExists" || code == "ObjectAlreadyExist")):
		kind = ErrObjectExists
	case status == http.StatusNotFound && (code == "NoSuchKey" || code == "NoSuchObject" || code == "NotFound"):
		kind = ErrObjectMissing
	case status == http.StatusNotFound && code == "NoSuchBucket":
		kind = ErrBucketMissing
	}
	return &providerError{op: op, profile: profile, kind: kind, cause: err}
}
