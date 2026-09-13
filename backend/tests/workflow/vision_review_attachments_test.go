package workflow_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

func assertPersistedVisionReviewAttachments(t *testing.T, ctx context.Context, store genapp.ReferenceStagedMediaTransactions, execution gen.ReferenceExecution, collection gen.ReferenceBundleInputCollection) {
	t.Helper()
	compiled := 0
	err := store.WithinReferenceStagedMedia(ctx, func(repo genapp.ReferenceStagedMediaRepository) error {
		target, err := repo.FindReferenceGenerationTarget(ctx, execution.WorkspaceID, execution.ProjectID, execution.ReadSet.TargetRef.ID)
		if err != nil {
			return err
		}
		for _, bundle := range collection.Bundles {
			if !bundle.Admission.InternalReviewReady {
				continue
			}
			subject := contract.VisionReviewSubject{
				WorkspaceID: execution.WorkspaceID, ProjectID: execution.ProjectID, TargetKind: target.TargetKind,
				GenerationRound: target.GenerationRound, TargetRef: bundle.Input.TargetRef, ExecutionRef: collection.ExecutionRef,
				BundleInputRef:       gen.GenerationActionRef{ID: bundle.Input.ID, ContentHash: bundle.Input.ContentHash},
				CandidateBundleIndex: bundle.Input.CandidateBundleIndex,
				BriefRevisionID:      target.ReferenceBriefRevisionRef.ID, BriefRevisionHash: target.ReferenceBriefRevisionRef.RevisionHash,
				// Contract fixture identities only: no Vision invocation or dispatch is created.
				StageReleaseHash: strings.Repeat("a", 64), InputHash: strings.Repeat("b", 64),
			}
			var media []gen.ReferenceStagedMedia
			for _, slot := range bundle.Input.Slots {
				item, err := repo.FindReferenceStagedMedia(ctx, execution.WorkspaceID, execution.ProjectID, slot.CallRef.CallKey)
				if err != nil {
					return err
				}
				if slot.MediaRef == nil {
					t.Fatal("ready bundle has no exact media reference")
				}
				subject.Slots = append(subject.Slots, contract.VisionReviewSlot{SlotKey: slot.SlotKey, ViewRole: slot.ViewRole, MediaRef: *slot.MediaRef, SHA256: item.SHA256})
				media = append(media, item)
			}
			attachments, err := contract.BuildVisionReviewAttachments(subject, media)
			if err != nil {
				return err
			}
			for i, attachment := range attachments {
				if attachment.RightsObservation != "not_assessed" || attachment.ProviderReceiptRef != media[i].ReceiptRef || attachment.ByteLength != media[i].ByteSize {
					t.Fatal("persisted media lost provenance or gained rights")
				}
			}
			repeated, err := contract.BuildVisionReviewAttachments(subject, media)
			if err != nil || !reflect.DeepEqual(repeated, attachments) {
				t.Fatalf("manifest replay: %v", err)
			}
			compiled++
		}
		return nil
	})
	if err != nil || compiled == 0 {
		t.Fatalf("persisted Vision attachment compilation: count=%d err=%v", compiled, err)
	}
}
