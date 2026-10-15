package operation_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type managedRecords struct {
	result *application.ManagedResult
	fail   bool
}

func (r *managedRecords) SaveManagedResult(_ context.Context, v application.ManagedResult) error {
	if r.fail {
		return application.ErrProviderCallConflict
	}
	r.result = &v
	return nil
}
func (r *managedRecords) ReadManagedResult(_ context.Context, _ application.ProviderDispatchIdentity) (*application.ManagedResult, error) {
	return r.result, nil
}
func TestManagedManifestRecoversAfterDatabaseFailure(t *testing.T) {
	objects := &stageTestObjects{entries: make(map[string]stageTestObject)}
	records := &managedRecords{fail: true}
	service := application.NewManagedResults(objects, records)
	identity := application.ProviderDispatchIdentity{ProjectID: uuid.New(), OperationID: uuid.New(), ModelProfileVersionID: uuid.New(), PriceRuleVersionID: uuid.New(), Action: "submit", Attempt: 1, RequestKey: "fixture"}
	result := application.ManagedResult{Identity: identity, State: "succeeded", Text: "fixture output", Usage: json.RawMessage(`{"billable_requests":1}`)}
	require.Error(t, service.Save(t.Context(), result))
	records.fail = false
	recovered, err := service.Recover(t.Context(), identity)
	require.NoError(t, err)
	require.Equal(t, result, *recovered)
	other := identity
	other.OperationID = uuid.New()
	missing, err := service.Recover(t.Context(), other)
	require.NoError(t, err)
	require.Nil(t, missing)
}
