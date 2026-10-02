package audit_test

import (
	"testing"

	"github.com/google/uuid"
)

func TestMediaPurgeRecordedPolicyOnlyReviewedSummaries(t *testing.T) {
	for _, action := range []string{"create", "cancel", "reconcile"} {
		after := map[string]any{"id": uuid.NewString(), "revision": 2, "status": "queued", "count": 3}
		checkBibleTransferAudit(t, "media.purge."+action, "media.purge", after)
	}
}
