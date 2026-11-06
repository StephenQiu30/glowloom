package catalog_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
)

// seedOnboardingModeEvidence simulates committed formal-validation receipts in
// the isolated database. It never sends a generation request.
func seedOnboardingModeEvidence(t *testing.T, db *gorm.DB, project, credential uuid.UUID, review catalogapp.OnboardingReleaseReview, mode string) uuid.UUID {
	t.Helper()
	operationID := uuid.New()
	input := uuid.NewString()
	require.NoError(t, db.Exec(`INSERT INTO operation.operation(id,project_id,capability,mode,input_hash,origin,status,region,model_profile_version_id,price_rule_version_id)
 SELECT ?,?,m.capability,?,?,'system','completed',p.region,r.model_version_id,r.price_version_id
 FROM catalog.model_release r JOIN catalog.model_profile m ON m.id=r.model_id JOIN catalog.provider p ON p.id=m.provider_id WHERE r.id=?`,
		operationID, project, mode, input, review.ID).Error)
	require.NoError(t, db.Exec("INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)", operationID, review.ID).Error)
	require.NoError(t, db.Exec("INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?)", operationID, credential).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_validation(id,release_id,credential_id,adapter_version,mode,input_digest,operation_id,status,evidence,completed_at)
 VALUES(?,?,?,'ai-provider-v1',?,?,?,'succeeded','{"kind":"real_provider"}',now())`,
		uuid.New(), review.ID, credential, mode, input, operationID).Error)
	return operationID
}
