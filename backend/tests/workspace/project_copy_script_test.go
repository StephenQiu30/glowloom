package workspace_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCopyScriptRequiresCompleteHistoricalReceiptBeforeCanvases(t *testing.T) {
	job := projectCopyJob()
	digest := job.Manifest.MediaSHA256
	job.Manifest.Script = &domain.ProjectCopyScriptSnapshot{ID: uuid.New(), ManifestSHA256: digest, ContentSHA256: digest, Counts: domain.ProjectCopyScriptCounts{Sources: 3, Versions: 2, VersionSources: 4, ProjectStates: 1, VersionHeads: 2, SplitSets: 3, SplitConfirmations: 2, Episodes: 4, Structures: 4, Scenes: 5, DialogueLines: 6, ActionLines: 7, Objects: 8}}
	worker := uuid.New()
	if err := job.Start(worker, time.Now()); err != nil {
		t.Fatal(err)
	}
	media := domain.ProjectCopyReceipt{ManifestSHA256: digest, ContentSHA256: digest, PrimaryCount: 3, SecondaryCount: 4}
	if err := job.AcceptMediaReceipt(worker, media); err != nil || job.Stage != "script" {
		t.Fatal("media checkpoint skipped full script history", job.Stage, err)
	}
	canvas := domain.ProjectCopyReceipt{ManifestSHA256: digest, ContentSHA256: digest, PrimaryCount: 2}
	before := job
	if err := job.AcceptCanvasReceipt(worker, canvas); !errors.Is(err, domain.ErrProjectCopyStateConflict) || !reflect.DeepEqual(job, before) {
		t.Fatal("missing script receipt changed job", err)
	}
	correct := domain.ProjectCopyScriptReceipt{ManifestSHA256: digest, ContentSHA256: digest, Counts: job.Manifest.Script.Counts}
	for _, changed := range []string{"worker", "objects", "historical_sources", "semantic_hash"} {
		t.Run(changed, func(t *testing.T) {
			receipt, fence := correct, worker
			switch changed {
			case "worker":
				fence = uuid.New()
			case "objects":
				receipt.Counts.Objects--
			case "historical_sources":
				receipt.Counts.VersionSources--
			case "semantic_hash":
				receipt.ContentSHA256 = "fedcba98fedcba98fedcba98fedcba98fedcba98fedcba98fedcba98fedcba98"
			}
			if err := job.AcceptScriptReceipt(fence, receipt); err == nil || !reflect.DeepEqual(job, before) {
				t.Fatal("incomplete history changed checkpoint", err)
			}
		})
	}
	if err := job.AcceptScriptReceipt(worker, correct); err != nil || job.Stage != "canvases" {
		t.Fatal("complete script history did not advance", err)
	}
	if err := job.AcceptCanvasReceipt(worker, canvas); err != nil {
		t.Fatal(err)
	}
	if err := job.Publish(worker); err != nil || job.Validate() != nil {
		t.Fatal("fully verified historical copy rejected", err)
	}
}

func TestProjectCopyScriptLegacyNilEncodingAndCorruptStates(t *testing.T) {
	legacy := projectCopyJob()
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	if _, present := object["ScriptReceipt"]; present {
		t.Fatal("nil field changed permanent admission response")
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(object["Manifest"], &manifest); err != nil {
		t.Fatal(err)
	}
	if _, present := manifest["script"]; present {
		t.Fatal("nil script changed legacy manifest")
	}
	for _, corruption := range []string{"orphan_receipt", "missing_snapshot", "negative_counts", "missing_semantic_hash"} {
		t.Run(corruption, func(t *testing.T) {
			job := legacy
			s := domain.ProjectCopyScriptSnapshot{ID: uuid.New(), ManifestSHA256: job.Manifest.MediaSHA256, ContentSHA256: job.Manifest.MediaSHA256}
			switch corruption {
			case "orphan_receipt":
				job.ScriptReceipt = &domain.ProjectCopyScriptReceipt{}
			case "missing_snapshot":
				job.Stage = "script"
			case "negative_counts":
				s.Counts.DialogueLines = -1
				job.Manifest.Script = &s
			case "missing_semantic_hash":
				s.ContentSHA256 = ""
				job.Manifest.Script = &s
			}
			if !errors.Is(job.Validate(), domain.ErrInvalidProjectCopy) {
				t.Fatal("corrupt script state accepted", corruption)
			}
		})
	}
}
