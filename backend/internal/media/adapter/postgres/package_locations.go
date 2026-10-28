package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// WithObjectLocations enables native work admission. Existing batches are never repinned.
func (s *PackageStore) WithObjectLocations() *PackageStore {
	out := *s
	out.locations = true
	return &out
}
func packageWorkOwner(plan application.PackagePlan) application.StorageWorkOwner {
	owner := plan.ActorID
	if plan.Request.Scope.ProjectID != nil {
		owner = *plan.Request.Scope.ProjectID
	}
	return application.StorageWorkOwner{Kind: "package", OwnerID: owner, WorkID: plan.JobID}
}
func admitPackageLocations(ctx context.Context, tx *gorm.DB, plan application.PackagePlan) (map[string]domain.ObjectLocation, error) {
	routes := NewStorageRoutes(tx)
	intent, err := routes.PinStorageWork(ctx, packageWorkOwner(plan))
	if err != nil {
		return nil, err
	}
	result := map[string]domain.ObjectLocation{}
	for _, o := range plan.Objects {
		l, err := routes.ReserveWorkLocation(ctx, intent.ID, o.Key)
		if err != nil {
			return nil, err
		}
		result[o.Key] = l
	}
	return result, nil
}
func hydratePackageLocations(ctx context.Context, tx *gorm.DB, plan *application.PackagePlan) error {
	var rows []struct {
		ObjectKey        string
		ObjectLocationID *uuid.UUID
	}
	if err := tx.Raw(`SELECT object_key,object_location_id FROM media.package_object WHERE job_id=? ORDER BY object_index`, plan.JobID).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(plan.Objects) {
		return application.ErrObjectMismatch
	}
	var native bool
	for _, row := range rows {
		native = native || row.ObjectLocationID != nil
	}
	if !native {
		return nil
	}
	owner := packageWorkOwner(*plan)
	var work storageWorkRow
	read := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE kind=? AND owner_id=? AND work_id=?`, owner.Kind, owner.OwnerID, owner.WorkID).Scan(&work)
	if read.Error != nil {
		return read.Error
	}
	intent := work.intent()
	locations := map[string]*domain.ObjectLocation{}
	for i, row := range rows {
		if row.ObjectKey != plan.Objects[i].Key || row.ObjectLocationID == nil {
			return application.ErrObjectMismatch
		}
		l, err := NewStorageRoutes(tx).ReadLocation(ctx, *row.ObjectLocationID)
		if err != nil {
			return err
		}
		if l.Ref.Key != row.ObjectKey {
			return application.ErrObjectMismatch
		}
		plan.Objects[i].Location = &l
		plan.Objects[i].StorageIntentID = intent.ID
		locations[row.ObjectKey] = &l
	}
	for i := range plan.Assets {
		plan.Assets[i].ObjectLocation = locations[plan.Assets[i].ObjectKey]
	}
	for i := range plan.Renditions {
		plan.Renditions[i].ObjectLocation = locations[plan.Renditions[i].ObjectKey]
	}
	return nil
}
func acceptPackageLocations(ctx context.Context, tx *gorm.DB, plan *application.PackagePlan) error {
	if err := hydratePackageLocations(ctx, tx, plan); err != nil {
		return err
	}
	if len(plan.Objects) == 0 || plan.Objects[0].Location == nil {
		return nil
	}
	intentID := plan.Objects[0].StorageIntentID
	if intentID == uuid.Nil {
		return application.ErrStorageNotReady
	}
	ids := make([]uuid.UUID, 0, len(plan.Objects))
	for _, o := range plan.Objects {
		if o.Location == nil || o.StorageIntentID != intentID || o.Location.ByteSize == nil || *o.Location.ByteSize != o.ByteSize || o.Location.SHA256 == nil || *o.Location.SHA256 != o.SHA256 || o.Location.MIMEType == nil || *o.Location.MIMEType != o.MIMEType {
			return application.ErrObjectMismatch
		}
		ids = append(ids, o.Location.ID)
	}
	return NewStorageRoutes(tx).AcceptStorageWork(ctx, intentID, ids)
}

func locationIDForKey(locations map[string]domain.ObjectLocation, key string) *uuid.UUID {
	l, ok := locations[key]
	if !ok {
		return nil
	}
	return &l.ID
}
