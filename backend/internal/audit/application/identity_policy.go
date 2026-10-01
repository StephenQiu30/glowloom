package application

// NewIdentityActionParser allows only the account-action summary fields
// declared by DES-12. Other action families must register a reviewed policy
// before their producers are enabled.
func NewIdentityActionParser() *Parser {
	return NewParser(identityActionFields())
}

// NewRecordedActionParser includes every action currently produced by the app.
func NewRecordedActionParser() *Parser {
	fields := identityActionFields()
	fields["credential.set"] = []string{"last4"}
	fields["credential.tested"] = []string{"last4"}
	fields["credential.disabled"] = []string{"last4"}
	fields["provider.created"] = []string{"key", "adapter_key", "region", "status", "concurrency_limit", "rate_limit_per_min"}
	fields["provider.updated"] = []string{"status", "concurrency_limit", "rate_limit_per_min"}
	fields["model.created"] = []string{"key", "provider_id", "capability", "display_name", "status"}
	fields["model.version_published"] = []string{"version_id", "version_no", "provider_model_id", "revision"}
	fields["model.enabled"] = []string{"status", "revision"}
	fields["model.disabled"] = []string{"status", "revision"}
	fields["price.published"] = []string{"version_id", "version_no", "unit", "currency", "effective_from", "revision"}
	fields["project.created"] = []string{"aspect_ratio", "style_type", "style_subtype", "style_preset_id", "status", "revision"}
	fields["project.updated"] = []string{"revision", "style_preset_id", "allow_overseas_models", "name_changed", "description_changed"}
	for _, action := range []string{"project.archived", "project.unarchived", "project.deleted", "project.restored"} {
		fields[action] = []string{"revision", "status", "is_delete", "archived_at", "delete_time", "purge_after"}
	}
	fields["project.defaults_changed"] = []string{"revision", "default_models_changed", "capability_count"}
	fields["prompt.customization_saved"] = []string{"operation", "mode", "revision", "content_sha256", "base_template_id"}
	fields["budget.changed"] = []string{"limit_micros", "revision", "is_overrun"}
	fields["operation.confirmed"] = []string{"quote_micros", "model_key", "region", "origin", "reused_from"}
	fields["batch.confirmed"] = []string{"quote_total_micros", "total_count", "expired_count"}
	fields["operation.cancel_requested"] = []string{"status"}
	fields["batch.cancel_requested"] = []string{"status"}
	fields["batch.resume_requested"] = []string{"status"}
	for _, action := range []string{"media.export_requested", "media.export_cancel", "media.export_retry", "media.export_reviewed"} {
		fields[action] = []string{"id", "project_id", "canvas_id", "node_id", "source_revision", "output_kind", "status", "stage", "progress", "attempt", "revision", "asset_id", "sha256", "failure_code", "created_at", "updated_at"}
	}
	for _, action := range []string{"media.transcription_requested", "media.transcription_cancel", "media.transcription_retry", "media.transcription_completed", "media.transcription_cancelled", "media.transcription_failed", "media.transcription_uncertain"} {
		fields[action] = []string{"id", "project_id", "canvas_id", "node_id", "source_revision", "language", "status", "stage", "progress", "attempt", "revision", "result_sha256", "failure_code", "created_at", "updated_at"}
	}
	return NewParser(fields)
}

func identityActionFields() map[string][]string {
	return map[string][]string{
		"auth.login_succeeded":  nil,
		"auth.login_failed":     {"reason"},
		"auth.locked":           {"retry_after_s"},
		"auth.logout":           nil,
		"user.created":          {"role", "status", "must_change_password"},
		"user.updated":          {"display_name", "role"},
		"user.disabled":         {"status"},
		"user.enabled":          {"status"},
		"user.password_reset":   {"must_change_password"},
		"user.password_changed": {"must_change_password"},
	}
}
