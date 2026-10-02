package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// TransferTopic identifies media-owned permanent commands, never paid operations.
const TransferTopic = "lanverse.media.transfer_command.v1"

func recordTransferCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string, job domain.TransferJob, emit string) error {
	response, err := json.Marshal(job)
	if err != nil || len(response) > 1<<20 {
		return application.ErrUnavailable
	}
	var eventID *uuid.UUID
	var eventAction *string
	var eventAttempt *int
	if emit != "" {
		id := uuid.NewSHA1(key, []byte("media-transfer/"+actor.ID.String()+"/"+emit))
		eventID, eventAction, eventAttempt = &id, &emit, &job.Attempt
		delivery := application.TransferDelivery{TransferWorkID: application.TransferWorkID{JobID: job.ID, Attempt: job.Attempt, Reconcile: emit == "reconcile"}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, Action: emit}
		body, err := json.Marshal(struct {
			EventID    uuid.UUID                    `json:"event_id"`
			EventType  string                       `json:"event_type"`
			OccurredAt time.Time                    `json:"occurred_at"`
			OrgID      uuid.UUID                    `json:"org_id"`
			Data       application.TransferDelivery `json:"data"`
		}{id, TransferTopic, time.Now().UTC(), actor.OrgID, delivery})
		if err != nil {
			return err
		}
		if err := libraryChanged(tx, `INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, TransferTopic, job.ID.String(), string(body)); err != nil {
			return err
		}
	}
	if err := libraryChanged(tx, `INSERT INTO media.transfer_command(actor_id,org_id,idem_key,job_id,action,request_sha256,response_body,event_id,event_action,event_attempt) VALUES(?,?,?,?,?,?,?,?,?,?)`, actor.ID, actor.OrgID, key, job.ID, action, hash, response, eventID, eventAction, eventAttempt); err != nil {
		return err
	}
	safe, _ := json.Marshal(struct {
		ID       uuid.UUID `json:"id"`
		Revision int64     `json:"revision"`
		Status   string    `json:"status"`
		Count    int       `json:"count"`
	}{job.ID, job.Revision, job.Status, len(job.Items)})
	id := uuid.NewSHA1(key, []byte("media-transfer-audit/"+actor.ID.String()+"/"+action))
	return libraryChanged(tx, `INSERT INTO audit.audit_log(id,org_id,project_id,actor_id,actor_kind,action,object_type,object_id,after) VALUES(?,?,?,?,'user',?,'media.transfer',?,?::jsonb)`, id, actor.OrgID, transferProject(job.Source, job.Target), actor.ID, "media.transfer."+action, job.ID.String(), string(safe))
}

// CreateTransfer accepts bytes-free SQL facts and permanent target identities.
// Authorization precedes replay, but replay never reruns later catalog CAS.
func (s *TransferStore) CreateTransfer(ctx context.Context, actor identityapp.Principal, in application.TransferInput) (domain.TransferJob, error) {
	var result domain.TransferJob
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if err := in.Validate(); err != nil {
		return result, err
	}
	hash, err := transferHash(in)
	if err != nil {
		return result, err
	}
	var cached *domain.TransferJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := transferLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, in.Source, in.Target, false); err != nil {
			return err
		}
		var err error
		cached, err = transferReplay(tx, actor, in.Key, hash, "create")
		return err
	})
	if err != nil {
		return result, err
	}
	if cached != nil {
		return *cached, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := transferLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		facts, err := s.access(ctx, tx, actor, in.Source, in.Target, true)
		if err != nil {
			return err
		}
		replay, err := transferReplay(tx, actor, in.Key, hash, "create")
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return nil
		}
		if facts.Revision != in.ExpectedProjectRevision {
			return domain.ErrTransferConflict
		}
		if s.guards == nil || s.guards(tx) == nil {
			return application.ErrUnavailable
		}
		busy, err := s.guards(tx).HasInflightProjectWork(ctx, actor, facts.ProjectID)
		if err != nil {
			return err
		}
		if busy {
			return domain.ErrTransferConflict
		}
		source, target, err := ensureTransferLibraries(tx, actor, in.Source, in.Target)
		if err != nil {
			return err
		}
		if source.Revision != in.ExpectedSourceRevision || target.Revision != in.ExpectedTargetRevision {
			return domain.ErrTransferConflict
		}
		var active bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.transfer_job WHERE (source_library_id IN ? OR target_library_id IN ?) AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended))`, []uuid.UUID{source.ID, target.ID}, []uuid.UUID{source.ID, target.ID}).Scan(&active).Error; err != nil {
			return err
		}
		if active {
			return domain.ErrTransferConflict
		}
		folders, err := libraryFolders(tx, target.ID, true)
		if err != nil {
			return err
		}
		if in.TargetFolderID != nil {
			found := false
			for _, f := range folders {
				if f.ID == *in.TargetFolderID && f.Revision == in.ExpectedFolderRevision {
					found = true
				}
			}
			if !found {
				return domain.ErrTransferConflict
			}
		}
		ids := make([]uuid.UUID, 0, len(in.Items))
		for _, item := range in.Items {
			ids = append(ids, item.ID)
		}
		items, err := lockLibraryItems(tx, actor, in.Source, source.ID, ids)
		if err != nil {
			return err
		}
		jobID := uuid.NewSHA1(in.Key, []byte("media-transfer-job/"+actor.ID.String()))
		now := s.clock().UTC().Truncate(time.Microsecond)
		if now.IsZero() {
			return application.ErrUnavailable
		}
		frozen := make([]application.FrozenTransferItem, 0, len(in.Items))
		for _, wanted := range in.Items {
			item := items[wanted.ID]
			if item.Revision != wanted.Revision || item.State != "active" {
				return domain.ErrTransferConflict
			}
			var file *application.LibraryMediaFile
			if item.AssetID != nil {
				sourceFile, err := transferSourceFile(tx, actor, in.Source, *item.AssetID)
				if err != nil {
					return err
				}
				file = &sourceFile
			}
			prepared, err := application.PrepareTransferItem(jobID, actor.OrgID, actor.ID, in.Target, in.TargetFolderID, item, file, now)
			if err != nil {
				return err
			}
			prepared.TargetItem.Position = int64(len(frozen))
			frozen = append(frozen, prepared)
		}
		manifest, bodies, err := transferFrozenBytes(frozen)
		if err != nil {
			return err
		}
		if err := libraryChanged(tx, `INSERT INTO media.transfer_job(id,org_id,actor_id,source_library_id,target_library_id,source_kind,target_kind,source_project_id,target_project_id,target_folder_id,target_folder_revision,source_revision,target_revision,project_revision,manifest_sha256,item_count,status,stage,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'queued','frozen',?,?)`, jobID, actor.OrgID, actor.ID, source.ID, target.ID, string(in.Source.Kind), string(in.Target.Kind), in.Source.ProjectID, in.Target.ProjectID, in.TargetFolderID, in.ExpectedFolderRevision, source.Revision, target.Revision, facts.Revision, manifest, len(frozen), now, now); err != nil {
			return err
		}
		for index, item := range frozen {
			var sourceID, targetID *uuid.UUID
			if item.SourceAsset != nil {
				sourceID = &item.SourceAsset.ID
				targetID = &item.TargetAsset.ID
			}
			if err := libraryChanged(tx, `INSERT INTO media.transfer_item(job_id,item_index,source_item_id,target_item_id,source_asset_id,target_asset_id,frozen,status) VALUES(?,?,?,?,?,?,?,'queued')`, jobID, index, item.SourceItem.ID, item.TargetItem.ID, sourceID, targetID, bodies[index]); err != nil {
				return err
			}
			for _, object := range item.Objects {
				if err := libraryChanged(tx, `INSERT INTO media.transfer_object(job_id,item_index,rendition_kind,source_object_key,target_object_key,byte_size,content_type,sha256) VALUES(?,?,?,?,?,?,?,?)`, jobID, index, object.RenditionKind, object.SourceObjectKey, object.TargetObjectKey, object.ByteSize, object.ContentType, object.SHA256); err != nil {
					return err
				}
			}
		}
		row, err := s.readJob(tx, actor, jobID, false)
		if err != nil {
			return err
		}
		result, err = transferView(tx, row)
		if err != nil {
			return err
		}
		return recordTransferCommand(tx, actor, in.Key, hash, "create", result, "start")
	})
	if err != nil {
		return domain.TransferJob{}, fmt.Errorf("admit media transfer: %w", err)
	}
	return result, nil
}

// HasInflightWork is consumed by workspace lifecycle and other admission guards.
// It reads actual transfer state for every creator in the already locked project.
func (s *TransferStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if project == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	var busy bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM media.transfer_job WHERE org_id=? AND (source_project_id=? OR target_project_id=?) AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended))`, actor.OrgID, project, project).Scan(&busy).Error
	return busy, err
}
