package operation_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type pinnedStageFactory struct {
	targets  map[uuid.UUID]string
	profiles map[string]*stageTestObjects
}

func (f pinnedStageFactory) ForProviderDispatch(_ context.Context, id application.ProviderDispatchIdentity) (application.ProviderStagingObjects, error) {
	profile, ok := f.targets[id.OperationID]
	if !ok {
		return nil, errors.New("operation has no admitted storage target")
	}
	return f.profiles[profile], nil
}
func TestProviderStagingReusesAdmittedTargetAfterRestart(t *testing.T) {
	identity := codexTestInput().Identity
	data := []byte("same generated original")
	sum := sha256.Sum256(data)
	output := application.ProviderReceiptOutput{Sequence: 1, SizeBytes: int64(len(data)), MIMEType: "image/png", SHA256: hex.EncodeToString(sum[:])}
	original := &stageTestObjects{entries: map[string]stageTestObject{}}
	next := &stageTestObjects{entries: map[string]stageTestObject{}}
	factory := pinnedStageFactory{targets: map[uuid.UUID]string{identity.OperationID: "minio-main"}, profiles: map[string]*stageTestObjects{"minio-main": original, "cos-main": next}}
	records := &stageTestRecords{}
	service := application.NewRoutedProviderStageService(factory, records)
	receipt, err := service.StageImage(t.Context(), identity, application.ProviderImageEvidence{ThreadID: "thread", TurnID: "turn", ItemID: "item", Model: "fixture"}, output, data)
	if err != nil {
		t.Fatal(err)
	}
	// A newly admitted operation may use COS; this existing operation must still
	// recover its MinIO artifact and manifest through a newly constructed worker.
	factory.targets[uuid.New()] = "cos-main"
	restarted := application.NewRoutedProviderStageService(factory, records)
	recovered, err := restarted.RecoverImage(t.Context(), identity)
	if err != nil || recovered == nil || recovered.ManifestSHA256 != receipt.ManifestSHA256 {
		t.Fatal("restart used another profile", err)
	}
	_, got, err := restarted.ReadImage(t.Context(), receipt)
	if err != nil || !bytes.Equal(got, data) || len(original.entries) != 2 || len(next.entries) != 0 {
		t.Fatal("provider staging drifted from its admission", err)
	}
}
