package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func exportFixtureInfo(data []byte) identity.ExportObjectInfo {
	digest := sha256.Sum256(data)
	return identity.ExportObjectInfo{Size: int64(len(data)), ContentType: "application/octet-stream", SHA256: hex.EncodeToString(digest[:])}
}
func (o *directoryTestObjects) Stat(_ context.Context, location identity.ExportObjectLocation) (identity.ExportObjectInfo, error) {
	data, ok := o.values[location.Key]
	if !ok {
		return identity.ExportObjectInfo{}, errors.New("fixture object missing")
	}
	return exportFixtureInfo(data), nil
}
func (o *exportMemoryObjects) Stat(_ context.Context, location identity.ExportObjectLocation) (identity.ExportObjectInfo, error) {
	data, ok := o.values[location.Key]
	if !ok {
		return identity.ExportObjectInfo{}, errors.New("fixture object missing")
	}
	return exportFixtureInfo(data), nil
}
func (f *exportMemoryStore) PinExportObject(_ context.Context, job identity.DirectoryExportJob) (identity.ExportObjectLocation, error) {
	return identity.ExportObjectLocation{ID: uuid.New(), IntentID: uuid.New(), ProfileID: "export-fixture", Bucket: "export-fixture", Key: "private/identity/directory-exports/" + job.ObjectID.String() + ".enc"}, nil
}
func (f *exportMemoryStore) BeginExportObjectWrite(_ context.Context, job identity.DirectoryExportJob, _ time.Time) error {
	if job.ObjectLocation == nil || job.ObjectLocation.ID == uuid.Nil {
		return identity.ErrExportUnavailable
	}
	return nil
}
func (f *exportMemoryStore) ConfirmExportObject(_ context.Context, job identity.DirectoryExportJob, info identity.ExportObjectInfo) error {
	if job.ObjectLocation == nil || info.Size < 1 || len(info.SHA256) != 64 {
		return identity.ErrExportUnavailable
	}
	return nil
}
func (f *exportMemoryStore) AcceptExportObject(_ context.Context, job identity.DirectoryExportJob) error {
	if job.ObjectLocation == nil || job.ObjectLocation.IntentID == uuid.Nil {
		return identity.ErrExportUnavailable
	}
	return nil
}
func (f *exportMemoryStore) CancelExportObject(context.Context, identity.DirectoryExportJob) error {
	return nil
}
