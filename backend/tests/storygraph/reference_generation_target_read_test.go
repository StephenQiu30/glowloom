package storygraph_test

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func TestReferenceGenerationTargetReadRevalidatesCurrentFactsWithoutWriting(t *testing.T) {
	for _, name := range []string{"current", "different_operator", "wrong_scope", "wrong_hash", "wrong_revision", "caller_revoked", "author_revoked", "head_drift", "brief_drift", "source_drift", "source_payload_drift", "capability_missing", "missing_receipt", "duplicate_receipt", "receipt_input_drift", "receipt_result_drift", "audit_drift", "authorization_drift"} {
		t.Run(name, func(t *testing.T) {
			repo, service, actor, command := referenceTargetFixture(t)
			published, err := service.BuildInitial(context.Background(), actor, command)
			if err != nil {
				t.Fatal(err)
			}
			query := generationapp.ReadReferenceGenerationTargetQuery{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, TargetRef: generationapp.ReferenceGenerationTargetRef{ID: published.ID, Revision: published.Revision, ContentHash: published.ContentHash}}
			caller := actor
			receiptKey := generationapp.BuildReferenceGenerationTargetOperation + ":" + command.IdempotencyKey
			receipt := repo.receipts[receiptKey]
			switch name {
			case "different_operator":
				caller.UserID = uuid.NewString()
			case "wrong_scope":
				query.ProjectID = uuid.NewString()
			case "wrong_hash":
				query.TargetRef.ContentHash = strings.Repeat("f", 64)
			case "wrong_revision":
				query.TargetRef.Revision++
			case "caller_revoked":
				caller.TokenVersion++
			case "author_revoked":
				caller.UserID = uuid.NewString()
				repo.deniedUser = actor.UserID
			case "head_drift":
				repo.head = uuid.NewString()
			case "brief_drift":
				repo.brief.RevisionHash = strings.Repeat("f", 64)
			case "source_drift":
				repo.source.ProductionWorldOwnerSetHash = strings.Repeat("f", 64)
			case "source_payload_drift":
				repo.source.Payload = []byte(`{}`)
			case "capability_missing":
				repo.capabilityError = errors.New("preset_capability_missing")
			case "missing_receipt":
				delete(repo.receipts, receiptKey)
			case "duplicate_receipt":
				duplicate := receipt
				duplicate.ID, duplicate.IdempotencyKey = uuid.NewString(), "duplicate"
				repo.receipts[duplicate.Operation+":"+duplicate.IdempotencyKey] = duplicate
			case "receipt_input_drift":
				receipt.InputHash = strings.Repeat("f", 64)
				repo.receipts[receiptKey] = receipt
			case "receipt_result_drift":
				receipt.Result = []byte(`{"id":"wrong"}`)
				repo.receipts[receiptKey] = receipt
			case "audit_drift":
				changed := published
				changed.CreatedAt = changed.CreatedAt.Add(time.Minute)
				repo.targets[published.ID] = changed
			case "authorization_drift":
				authKey := generationapp.AuthorizeInitialReferenceGenerationOperation + ":authorize"
				auth := repo.receipts[authKey]
				auth.InputHash = strings.Repeat("f", 64)
				repo.receipts[authKey] = auth
			}
			beforeReceipts, beforeTargets, beforeHead := maps.Clone(repo.receipts), maps.Clone(repo.targets), repo.head
			actual, err := service.ReadCurrent(context.Background(), caller, query)
			if name == "current" || name == "different_operator" {
				if err != nil || !reflect.DeepEqual(actual, published) {
					t.Fatalf("read current Target: %v", err)
				}
			} else if err == nil || !reflect.DeepEqual(actual, generationapp.ReferenceGenerationTarget{}) {
				t.Fatalf("read accepted drift or leaked Target: %v", err)
			}
			if !reflect.DeepEqual(beforeReceipts, repo.receipts) || !reflect.DeepEqual(beforeTargets, repo.targets) || beforeHead != repo.head {
				t.Fatal("Target read changed persisted facts")
			}
		})
	}
}

func (repo *referenceTargetMemory) FindReferenceGenerationTargetReceipt(_ context.Context, workspace, target string) (platformcommand.Receipt, error) {
	var found []platformcommand.Receipt
	for _, receipt := range repo.receipts {
		if receipt.WorkspaceID == workspace && receipt.Operation == generationapp.BuildReferenceGenerationTargetOperation && receipt.ResourceID == target {
			found = append(found, receipt)
		}
	}
	if len(found) != 1 {
		return platformcommand.Receipt{}, errors.New("publication receipt unavailable or ambiguous")
	}
	return found[0], nil
}
