package providerprobe

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	flow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// TestReachability admits and records a no-key check at the worker. Neither a
// credential nor an encrypted envelope is loaded by this path.
func (a *Activities) TestReachability(ctx context.Context, input flow.Input) error {
	request := catalog.CredentialTestRequest{Kind: "reachability", TestID: input.TestID, ProviderID: input.ProviderID, ActorID: input.ActorID, OrgID: input.OrgID, RequestID: input.RequestID}
	if input.CredentialID != uuid.Nil || a.access == nil {
		return catalog.ErrInvalidCredentialTest
	}
	authorize := func() error {
		return a.access.VerifyCredentialTestRequest(ctx, request, uuid.NewSHA1(input.TestID, []byte("credential_test_requested")))
	}
	if err := authorize(); err != nil {
		return err
	}
	var row struct{ Config []byte }
	r := a.db.WithContext(ctx).Raw(`UPDATE catalog.provider_check check_run SET status='running',dispatch_started_at=now()
 FROM catalog.provider p,catalog.provider_config_version config,catalog.provider_preset_version pv,catalog.provider_preset preset
 WHERE check_run.id=? AND check_run.actor_id=? AND check_run.provider_id=? AND check_run.kind='reachability' AND check_run.credential_id IS NULL
 AND check_run.status='pending' AND check_run.dispatch_started_at IS NULL AND p.id=check_run.provider_id AND NOT p.is_delete
 AND p.current_config_version_id=check_run.config_version_id AND config.id=check_run.config_version_id
 AND pv.id=config.preset_version_id AND preset.id=pv.preset_id AND NOT preset.emergency_disabled RETURNING config.config`, input.TestID, input.ActorID, input.ProviderID).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		// A completed/running replay is harmless, but stale pending admission must
		// become terminal so it cannot occupy a shared dispatch slot forever.
		return a.db.WithContext(ctx).Exec(`UPDATE catalog.provider_check SET status='failed',error_code='configuration_changed',completed_at=now() WHERE id=? AND actor_id=? AND provider_id=? AND kind='reachability' AND status='pending'`, input.TestID, input.ActorID, input.ProviderID).Error
	}
	var config domain.ProviderConfig
	if json.Unmarshal(row.Config, &config) != nil || config.Validate() != nil {
		return catalog.ErrInvalidCredentialTest
	}
	if err := authorize(); err != nil {
		return err
	}
	result := a.client.ProbeReachability(ctx, config)
	if err := authorize(); err != nil {
		return err
	}
	status := "failed"
	if result.Result == "reachable" {
		status = "completed"
	}
	return a.db.WithContext(ctx).Exec(`UPDATE catalog.provider_check SET status=?,network_result=?,http_status=?,elapsed_ms=?,attempts=?,error_code=NULLIF(?,''),completed_at=now() WHERE id=? AND status='running'`, status, result.Result, result.HTTPStatus, result.ElapsedMS, result.Attempts, result.ErrorCode, input.TestID).Error
}
