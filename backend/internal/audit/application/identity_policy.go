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
	fields["project.updated"] = []string{"revision", "style_preset_id", "allow_overseas_models", "name_changed", "description_changed", "cover_changed"}
	for _, action := range []string{"project.archived", "project.unarchived", "project.deleted", "project.restored"} {
		fields[action] = []string{"revision", "status", "is_delete", "archived_at", "delete_time", "purge_after"}
	}
	fields["project.defaults_changed"] = []string{"revision", "default_models_changed", "capability_count"}
	for _, action := range []string{"project_folder.created", "project_folder.updated", "project_folder.moved", "project_folder.recycled"} {
		fields[action] = []string{"folder_id", "project_id", "revision", "placement_revision", "recycled_count", "name_updated", "cover_updated"}
	}
	fields["prompt.customization_saved"] = []string{"operation", "mode", "revision", "content_sha256", "base_template_id"}
	fields["budget.changed"] = []string{"limit_micros", "revision", "is_overrun"}
	fields["operation.confirmed"] = []string{"quote_micros", "model_key", "region", "origin", "reused_from"}
	fields["batch.confirmed"] = []string{"quote_total_micros", "total_count", "expired_count"}
	fields["operation.cancel_requested"] = []string{"status"}
	fields["batch.cancel_requested"] = []string{"status"}
	fields["batch.resume_requested"] = []string{"status"}
	fields["media.uploaded"] = []string{"kind", "byte_size", "sha256", "review_method", "reused"}
	for _, action := range []string{"media.library.create_folder", "media.library.update_folder", "media.library.delete_folder", "media.library.create_text", "media.library.update_item", "media.library.move_items", "media.library.recycle_items", "media.library.restore_items", "media.library.remove_items"} {
		fields[action] = []string{"library_id", "revision", "count"}
	}
	for _, action := range []string{"media.transfer.create", "media.transfer.cancel", "media.transfer.retry", "media.transfer.reconcile"} {
		fields[action] = []string{"id", "revision", "status", "count"}
	}
	for _, kind := range []string{"character", "location", "prop"} {
		for _, action := range []string{"create", "update", "confirm", "delete", "restore", "adopt_result", "create_result"} {
			fields["bible."+kind+"_"+action] = []string{"revision", "project_revision", "version_id", "content_sha256"}
		}
	}
	for _, action := range []string{"merge", "split", "look_create", "look_update", "look_delete", "look_default", "references", "voice_bind", "voice_unbind"} {
		fields["bible.character_"+action] = []string{"revision", "project_revision", "version_id", "content_sha256"}
	}
	for _, action := range []string{"script.source_create", "script.source_update", "script.source_delete", "script.source_import", "script.source_reorder"} {
		fields[action] = []string{"script_revision", "project_revision", "version_id", "split_set_id", "source_count", "content_hash", "document_sha256", "source_manifest_sha256"}
	}
	for _, action := range []string{"script.rules_split_saved", "script.split_confirmed", "script.structure_saved", "script.structure_confirmed", "script.source_write_cancel", "script.source_write_reconcile", "script.version_adopted"} {
		fields[action] = []string{"script_revision", "version_id", "episode_id"}
	}
	for _, action := range []string{"script.file_import_create", "script.file_import_cancel", "script.file_import_retry", "script.file_import_reconcile", "script.file_import_completed", "script.file_import_cancelled"} {
		fields[action] = []string{"id", "revision", "attempt", "status", "stage", "file_count", "script_revision"}
	}
	for _, action := range []string{"media.export_requested", "media.export_cancel", "media.export_retry", "media.export_reviewed"} {
		fields[action] = []string{"id", "project_id", "canvas_id", "node_id", "source_revision", "output_kind", "status", "stage", "progress", "attempt", "revision", "asset_id", "sha256", "failure_code", "created_at", "updated_at"}
	}
	for _, action := range []string{"media.transcription_requested", "media.transcription_cancel", "media.transcription_retry", "media.transcription_completed", "media.transcription_cancelled", "media.transcription_failed", "media.transcription_uncertain"} {
		fields[action] = []string{"id", "project_id", "canvas_id", "node_id", "source_revision", "language", "status", "stage", "progress", "attempt", "revision", "result_sha256", "failure_code", "created_at", "updated_at"}
	}
	for _, action := range []string{"project.copy_requested", "project.copy_retry", "project.copy_cancel", "project.copy_reconcile", "project.copy_completed", "project.copy_failed", "project.copy_cancelled"} {
		fields[action] = []string{"copy_job_id", "source_project_id", "target_project_id", "status", "stage", "revision", "documents", "assets", "renditions", "needs_reconciliation", "failure_code"}
	}
	for _, action := range []string{"media.depth_requested", "media.depth_cancel", "media.depth_retry", "media.depth_reconcile", "media.depth_reviewed", "media.depth_rendered", "media.depth_failed", "media.depth_cancelled"} {
		fields[action] = []string{"id", "project_id", "canvas_id", "node_id", "source_revision", "source_asset_id", "source_asset_revision", "profile_id", "status", "stage", "attempt", "revision", "asset_id", "sha256", "failure_code", "needs_reconciliation", "execution_unconfirmed"}
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
