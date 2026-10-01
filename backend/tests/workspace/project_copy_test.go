package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func projectCopyJob() domain.ProjectCopyJob {
	return domain.ProjectCopyJob{RequestID: uuid.NewString(), ID: uuid.New(), OrgID: uuid.New(), ActorID: uuid.New(), SourceProjectID: uuid.New(), SourceRevision: 1, TargetProjectID: uuid.New(), TargetName: "完整副本", Status: "queued", Stage: "media", Revision: 1, Manifest: domain.ProjectCopyManifest{WorkspaceSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CanvasSnapshotID: uuid.New(), CanvasSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Documents: 2, MediaSnapshotID: uuid.New(), MediaSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Assets: 3, Renditions: 4}}
}

func TestProjectCopyRequiresAllReceiptsAndWorkerFence(t *testing.T) {
	job := projectCopyJob()
	worker := uuid.New()
	if err := job.Start(worker, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := job.Publish(worker); !errors.Is(err, domain.ErrProjectCopyStateConflict) {
		t.Fatalf("unverified project published: %v", err)
	}
	mediaReceipt := domain.ProjectCopyReceipt{ManifestSHA256: job.Manifest.MediaSHA256, ContentSHA256: job.Manifest.MediaSHA256, PrimaryCount: 3, SecondaryCount: 4}
	if err := job.AcceptMediaReceipt(uuid.New(), mediaReceipt); !errors.Is(err, domain.ErrProjectCopyWorkerConflict) {
		t.Fatalf("other worker changed job: %v", err)
	}
	bad := mediaReceipt
	bad.SecondaryCount = 3
	if err := job.AcceptMediaReceipt(worker, bad); !errors.Is(err, domain.ErrInvalidProjectCopy) {
		t.Fatalf("incomplete renditions accepted: %v", err)
	}
	if err := job.AcceptMediaReceipt(worker, mediaReceipt); err != nil {
		t.Fatal(err)
	}
	if err := job.Publish(worker); !errors.Is(err, domain.ErrProjectCopyStateConflict) {
		t.Fatal("missing document receipt published", err)
	}
	canvasReceipt := domain.ProjectCopyReceipt{ManifestSHA256: job.Manifest.CanvasSHA256, ContentSHA256: job.Manifest.CanvasSHA256, PrimaryCount: 2}
	if err := job.AcceptCanvasReceipt(worker, canvasReceipt); err != nil {
		t.Fatal(err)
	}
	if err := job.Publish(worker); err != nil || job.Status != "succeeded" || job.Stage != "complete" || job.WorkerID != uuid.Nil {
		t.Fatalf("verified project not publishable: %+v %v", job, err)
	}
	if err := job.RequestCancel(); !errors.Is(err, domain.ErrProjectCopyStateConflict) {
		t.Fatal("published project cancelled", err)
	}
}

func TestProjectCopyCancellationWaitsForCleanupAndUnknownRecovery(t *testing.T) {
	job := projectCopyJob()
	worker := uuid.New()
	if err := job.Start(worker, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := job.RequestCancel(); err != nil || job.Status != "cancel_requested" {
		t.Fatal("cancel request not recorded", err)
	}
	if err := job.Publish(worker); !errors.Is(err, domain.ErrProjectCopyStateConflict) {
		t.Fatal("cancelled work published", err)
	}
	if err := job.Fail(worker, "object_write_unknown", false, true); err != nil || !job.NeedsReconciliation || !job.CancellationRequested {
		t.Fatal("unknown write discarded cancellation intent", err)
	}
	if err := job.Retry(); !errors.Is(err, domain.ErrProjectCopyStateConflict) {
		t.Fatal("unknown object write blindly retried", err)
	}
	if err := job.RequestReconciliation(); err != nil {
		t.Fatal(err)
	}
	if err := job.ResumeReconciliation(uuid.New()); err != nil || job.Status != "cancel_requested" {
		t.Fatal("recovery lost pending cancellation", err)
	}
	if err := job.ConfirmCancelled(job.WorkerID, false); !errors.Is(err, domain.ErrProjectCopyStateConflict) {
		t.Fatal("missing cleanup receipt reported cancelled", err)
	}
	if err := job.ConfirmCancelled(job.WorkerID, true); err != nil || job.Status != "cancelled" || job.WorkerID != uuid.Nil {
		t.Fatal("completed cleanup not cancelled", err)
	}
}

func TestProjectCopyRejectsCorruptPersistedState(t *testing.T) {
	for _, name := range []string{"running without worker", "canvases without media receipt", "finalizing without canvas receipt", "succeeded without receipts", "failed without failure", "unknown queued", "cancelled without intent", "invalid stored receipt"} {
		t.Run(name, func(t *testing.T) {
			job := projectCopyJob()
			switch name {
			case "running without worker":
				job.Status = "running"
			case "canvases without media receipt":
				job.Stage = "canvases"
			case "finalizing without canvas receipt":
				job.Stage = "finalizing"
			case "succeeded without receipts":
				job.Status, job.Stage = "succeeded", "complete"
			case "failed without failure":
				job.Status = "failed"
			case "unknown queued":
				job.NeedsReconciliation = true
			case "cancelled without intent":
				job.Status, job.Stage = "cancelled", "complete"
			case "invalid stored receipt":
				job.MediaReceipt = &domain.ProjectCopyReceipt{ManifestSHA256: job.Manifest.MediaSHA256, ContentSHA256: job.Manifest.MediaSHA256, PrimaryCount: 1, SecondaryCount: 4}
			}
			if !errors.Is(job.Validate(), domain.ErrInvalidProjectCopy) {
				t.Fatal("corrupt stored copy accepted")
			}
		})
	}
}
