package catalogfixture

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// SeedValidationEvidence writes synthetic completed validation receipts to an
// isolated test database. It exercises the real availability joins; it does not
// contact a supplier or establish live-provider acceptance.
func SeedValidationEvidence(t *testing.T, db *gorm.DB, project, release, credential uuid.UUID, modes ...string) {
	t.Helper()
	for _, mode := range modes {
		operation, digest := uuid.New(), uuid.NewString()
		r := db.Exec(`INSERT INTO operation.operation(id,project_id,capability,mode,input_hash,origin,status,region,model_profile_version_id,price_rule_version_id)
 SELECT ?,?,m.capability,?,?,'system','completed',p.region,r.model_version_id,r.price_version_id
 FROM catalog.model_release r JOIN catalog.model_profile m ON m.id=r.model_id JOIN catalog.provider p ON p.id=m.provider_id WHERE r.id=?`, operation, project, mode, digest, release)
		require.NoError(t, r.Error)
		require.EqualValues(t, 1, r.RowsAffected)
		require.NoError(t, db.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, operation, release).Error)
		require.NoError(t, db.Exec(`INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?)`, operation, credential).Error)
		require.NoError(t, db.Exec(`INSERT INTO catalog.model_validation(id,release_id,credential_id,adapter_version,mode,input_digest,operation_id,status,evidence,completed_at)
 VALUES(?,?,?,'ai-provider-v1',?,?,?,'succeeded','{"kind":"real_provider","fixture":true}',now())`, uuid.New(), release, credential, mode, digest, operation).Error)
	}
}
