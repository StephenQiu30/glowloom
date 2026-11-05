package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// transferOutputAttemptRow is append-only physical output identity. Execution
// reconciliation can advance the job attempt without replacing this output.
type transferOutputAttemptRow struct {
	JobID         uuid.UUID
	ItemIndex     int
	OutputAttempt int
	WorkIntentID  *uuid.UUID
	Frozen        []byte
	CreatedAt     time.Time
}

func (transferOutputAttemptRow) TableName() string { return "media.transfer_output_attempt" }

// transferAttemptItem retains admission/source identity and derives independent
// physical keys for every output attempt, including the first one.
func transferAttemptItem(initial application.FrozenTransferItem, attempt int) (application.FrozenTransferItem, error) {
	var item application.FrozenTransferItem
	if attempt < 1 {
		return item, application.ErrUnavailable
	}
	encoded, err := json.Marshal(initial)
	if err != nil || strictCopyMedia(encoded, &item) != nil {
		return item, application.ErrUnavailable
	}
	if item.TargetAsset == nil {
		if len(item.Objects) != 0 || len(item.TargetRenditions) != 0 {
			return item, application.ErrUnavailable
		}
		return item, nil
	}
	a := item.TargetAsset
	owner := "projects/" + a.ProjectID.String()
	if a.Personal != nil {
		owner = "personal/" + a.Personal.OrgID.String() + "/" + a.Personal.ActorID.String()
	}
	ext := path.Ext(a.ObjectKey)
	a.ObjectKey = path.Join(owner, string(a.Kind), fmt.Sprintf("%04d/%02d", a.CreateTime.Year(), a.CreateTime.Month()), a.ID.String()+"-transfer-"+strconv.Itoa(attempt)+ext)
	a.ObjectLocation = nil
	keys := map[string]string{"": a.ObjectKey}
	for i := range item.TargetRenditions {
		r := &item.TargetRenditions[i]
		r.ObjectKey = strings.TrimSuffix(a.ObjectKey, ext) + "/" + string(r.Kind) + path.Ext(r.ObjectKey)
		r.ObjectLocation = nil
		keys[string(r.Kind)] = r.ObjectKey
	}
	if len(item.Objects) != len(keys) {
		return item, application.ErrUnavailable
	}
	for i := range item.Objects {
		o := &item.Objects[i]
		key, ok := keys[o.RenditionKind]
		if !ok || o.TargetAssetID != a.ID {
			return item, application.ErrUnavailable
		}
		o.TargetObjectKey, o.TargetLocation = key, nil
		o.Status, o.SourceVerified, o.WriteStarted = application.ProjectCopyObjectPending, false, false
		delete(keys, o.RenditionKind)
	}
	return item, nil
}

// bindTransferAttempt pins a new work on the prior output's frozen route. Only
// admission chooses the current environment route; retries never reselect it.
func bindTransferAttempt(ctx context.Context, tx *gorm.DB, job, library uuid.UUID, index, attempt int, prior *storageWorkRow, item *application.FrozenTransferItem) (*uuid.UUID, error) {
	if len(item.Objects) == 0 {
		return nil, nil
	}
	owner := transferWorkOwner(job, library, index, attempt)
	routes := NewStorageRoutes(tx)
	var intent application.StorageWorkIntent
	if prior == nil {
		var err error
		intent, err = routes.PinStorageWork(ctx, owner)
		if err != nil {
			return nil, err
		}
	} else {
		work := storageWorkRow{ID: uuid.New(), Kind: owner.Kind, OwnerID: owner.OwnerID, WorkID: owner.WorkID, ProfileID: prior.ProfileID, Bucket: prior.Bucket, ConfigRevision: prior.ConfigRevision, State: application.StorageWorkOpen}
		if err := requireOne(tx.Create(&work), application.ErrUnavailable); err != nil {
			return nil, err
		}
		intent = work.intent()
	}
	if intent.State != application.StorageWorkOpen {
		return nil, application.ErrStorageRouteConflict
	}
	for i := range item.Objects {
		o := &item.Objects[i]
		if o.SourceLocation == nil || o.SourceLocation.ID == uuid.Nil || o.SourceLocation.Ref.Validate() != nil || o.SourceLocation.Ref.Key != o.SourceObjectKey || o.SourceLocation.State != domain.ObjectLocationAccepted {
			return nil, application.ErrStorageNotReady
		}
		location, err := routes.ReserveWorkLocation(ctx, intent.ID, o.TargetObjectKey)
		if err != nil {
			return nil, err
		}
		o.TargetLocation = &location
	}
	assets := []application.ProjectCopySourceAsset{{Asset: *item.TargetAsset, Renditions: item.TargetRenditions}}
	if err := attachCopyAssetLocations(assets, item.Objects); err != nil {
		return nil, err
	}
	item.TargetAsset, item.TargetRenditions = &assets[0].Asset, assets[0].Renditions
	return &intent.ID, nil
}

func insertTransferAttempt(tx *gorm.DB, job uuid.UUID, index, attempt int, work *uuid.UUID, item application.FrozenTransferItem) error {
	frozen, err := json.Marshal(item)
	if err != nil || len(frozen) > 1<<20 {
		return application.ErrUnavailable
	}
	record := transferOutputAttemptRow{JobID: job, ItemIndex: index, OutputAttempt: attempt, WorkIntentID: work, Frozen: frozen}
	if err := requireOne(tx.Create(&record), application.ErrUnavailable); err != nil {
		return err
	}
	for _, object := range item.Objects {
		record := transferObjectRow{JobID: job, ItemIndex: index, OutputAttempt: attempt, RenditionKind: object.RenditionKind, SourceObjectKey: object.SourceObjectKey, TargetObjectKey: object.TargetObjectKey, ByteSize: object.ByteSize, ContentType: object.ContentType, SHA256: object.SHA256, Status: application.ProjectCopyObjectPending, SourceLocationID: locationID(object.SourceLocation), TargetLocationID: locationID(object.TargetLocation)}
		if err := requireOne(tx.Create(&record), application.ErrUnavailable); err != nil {
			return err
		}
	}
	return nil
}

// readTransferAttempt requires current physical facts, even for attempt one.
// The immutable admission is used only to verify source and logical identity.
func readTransferAttempt(tx *gorm.DB, job transferJobRow, index int, initial application.FrozenTransferItem) (transferOutputAttemptRow, application.FrozenTransferItem, error) {
	var output transferOutputAttemptRow
	var item application.FrozenTransferItem
	read := tx.Where("job_id = ? AND item_index = ?", job.ID, index).Order("output_attempt DESC").Take(&output)
	if read.Error != nil {
		return output, item, read.Error
	}
	if output.OutputAttempt < 1 || output.OutputAttempt > job.Attempt || strictCopyMedia(output.Frozen, &item) != nil {
		return output, item, application.ErrUnavailable
	}
	expected, err := transferAttemptItem(initial, output.OutputAttempt)
	if err != nil {
		return output, item, err
	}
	// Locations are compared separately to exact persisted Ref/owner facts; the
	// remaining snapshot must equal the immutable source/logical admission.
	var comparison application.FrozenTransferItem
	if strictCopyMedia(output.Frozen, &comparison) != nil {
		return output, item, application.ErrUnavailable
	}
	if comparison.TargetAsset != nil {
		comparison.TargetAsset.ObjectLocation = nil
	}
	for i := range comparison.TargetRenditions {
		comparison.TargetRenditions[i].ObjectLocation = nil
	}
	for i := range comparison.Objects {
		comparison.Objects[i].TargetLocation = nil
	}
	if !equalCopyMedia(expected, comparison) {
		return output, item, application.ErrObjectMismatch
	}
	if len(item.Objects) == 0 {
		if output.WorkIntentID != nil {
			return output, item, application.ErrObjectMismatch
		}
		return output, item, nil
	}
	if output.WorkIntentID == nil {
		return output, item, application.ErrStorageNotReady
	}
	work, err := readStorageWorkByID(tx, *output.WorkIntentID, "SHARE")
	if err != nil {
		return output, item, err
	}
	if work.intent().Owner != transferWorkOwner(job.ID, job.TargetLibraryID, index, output.OutputAttempt) {
		return output, item, application.ErrObjectMismatch
	}
	for _, object := range item.Objects {
		if object.TargetLocation == nil || object.TargetLocation.Ref.ProfileID != work.ProfileID || object.TargetLocation.Ref.Bucket != work.Bucket || object.TargetLocation.Ref.Key != object.TargetObjectKey {
			return output, item, application.ErrObjectMismatch
		}
		_, location, err := copyTargetLocation(tx.Statement.Context, tx, work.intent().Owner, &object.TargetLocation.ID, object.TargetObjectKey)
		if err != nil {
			return output, item, err
		}
		if location.Ref != object.TargetLocation.Ref {
			return output, item, application.ErrObjectMismatch
		}
		bound := item.TargetAsset.ObjectLocation
		if object.RenditionKind != "" {
			bound = nil
			for _, rendition := range item.TargetRenditions {
				if string(rendition.Kind) == object.RenditionKind {
					bound = rendition.ObjectLocation
				}
			}
		}
		if bound == nil || bound.ID != location.ID || bound.Ref != location.Ref {
			return output, item, application.ErrObjectMismatch
		}
	}
	return output, item, nil
}

func retryTransferItems(ctx context.Context, tx *gorm.DB, row transferJobRow) error {
	var failed []transferItemRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND status = ?", row.ID, domain.TransferItemStatusFailed).Order("item_index").Find(&failed).Error; err != nil {
		return err
	}
	if len(failed) == 0 {
		return domain.ErrTransferConflict
	}
	for _, state := range failed {
		var initial application.FrozenTransferItem
		if strictCopyMedia(state.Frozen, &initial) != nil {
			return application.ErrUnavailable
		}
		output, old, err := readTransferAttempt(tx, row, state.ItemIndex, initial)
		if err != nil {
			return err
		}
		var prior *storageWorkRow
		if len(old.Objects) > 0 {
			work, err := readStorageWorkByID(tx, *output.WorkIntentID, "SHARE")
			if err != nil {
				return err
			}
			if work.State != application.StorageWorkCancelled || work.ClosedAt == nil {
				return domain.ErrTransferConflict
			}
			var objects []transferObjectRow
			if err := tx.Where("job_id = ? AND item_index = ? AND output_attempt = ?", row.ID, state.ItemIndex, output.OutputAttempt).Find(&objects).Error; err != nil {
				return err
			}
			if len(objects) != len(old.Objects) {
				return application.ErrObjectMismatch
			}
			for _, object := range objects {
				location, err := readRowLocation(tx, object.TargetLocationID, object.TargetObjectKey)
				if err != nil {
					return err
				}
				if object.Status != application.ProjectCopyObjectRemoved || location == nil || location.State != domain.ObjectLocationDeleted {
					return domain.ErrTransferConflict
				}
			}
			prior = &work
		}
		item, err := transferAttemptItem(initial, row.Attempt+1)
		if err != nil {
			return err
		}
		work, err := bindTransferAttempt(ctx, tx, row.ID, row.TargetLibraryID, state.ItemIndex, row.Attempt+1, prior, &item)
		if err != nil {
			return err
		}
		if err := insertTransferAttempt(tx, row.ID, state.ItemIndex, row.Attempt+1, work, item); err != nil {
			return err
		}
		if err := updateOne(tx, "media.transfer_item", map[string]any{"job_id": row.ID, "item_index": state.ItemIndex, "status": domain.TransferItemStatusFailed}, map[string]any{"status": domain.TransferItemStatusQueued, "failure_code": nil}, application.ErrLibraryConflict); err != nil {
			return err
		}
	}
	return nil
}
