-- Lanverse MVP partial unique indexes.
CREATE UNIQUE INDEX uq_source_revisions_current ON public.source_revisions (episode_id) WHERE status = 'confirmed';
CREATE UNIQUE INDEX uq_script_versions_current ON public.script_versions (episode_id) WHERE status = 'confirmed';
CREATE UNIQUE INDEX uq_creative_asset_versions_current ON public.creative_asset_versions (asset_id) WHERE status = 'confirmed';
CREATE UNIQUE INDEX uq_shot_spec_versions_current ON public.shot_spec_versions (episode_id) WHERE status = 'confirmed';
CREATE UNIQUE INDEX uq_subtitle_versions_current ON public.subtitle_versions (episode_id) WHERE status = 'confirmed';
CREATE UNIQUE INDEX uq_script_versions_origin_task ON public.script_versions (origin_task_id) WHERE origin_task_id IS NOT NULL;
CREATE UNIQUE INDEX uq_creative_asset_versions_origin_task_asset ON public.creative_asset_versions (origin_task_id, asset_id) WHERE origin_task_id IS NOT NULL;
CREATE UNIQUE INDEX uq_shot_spec_versions_origin_task ON public.shot_spec_versions (origin_task_id) WHERE origin_task_id IS NOT NULL;
CREATE UNIQUE INDEX uq_production_attempts_provider_key ON public.production_attempts (provider_id, provider_request_key) WHERE provider_request_key IS NOT NULL;
CREATE UNIQUE INDEX uq_production_attempts_provider_request_id ON public.production_attempts (provider_id, provider_request_id) WHERE provider_request_id IS NOT NULL;
CREATE UNIQUE INDEX uq_render_snapshots_initial_task ON public.render_snapshots (initial_task_id) WHERE initial_task_id IS NOT NULL;
CREATE UNIQUE INDEX uq_generation_candidates_ready_primary ON public.generation_candidates (task_id, usage_type, usage_id, input_version_id, input_hash) WHERE status = 'ready' AND output_slot = 'primary';
CREATE UNIQUE INDEX uq_adoptions_active_slot ON public.adoptions (usage_type, usage_id, input_version_id, input_hash) WHERE status = 'active';

-- Lanverse MVP partial lookup indexes.
CREATE INDEX ix_production_tasks_nonterminal ON public.production_tasks (status, updated_at) WHERE status IN ('queued', 'running', 'cancelling', 'unknown');
CREATE INDEX ix_task_jobs_pending ON public.task_jobs (next_attempt_at, created_at) WHERE state = 'pending';
CREATE INDEX ix_task_jobs_leased ON public.task_jobs (lease_until) WHERE state = 'leased';
CREATE INDEX ix_media_versions_sha256 ON public.media_versions (sha256) WHERE sha256 IS NOT NULL;

-- Lanverse MVP query indexes.
CREATE INDEX ix_projects_created_at ON public.projects (created_at DESC);
CREATE INDEX ix_production_tasks_episode_created_at ON public.production_tasks (episode_id, created_at DESC);
CREATE INDEX ix_task_events_task_occurred_at ON public.task_events (task_id, occurred_at);
CREATE INDEX ix_media_objects_episode_created_at ON public.media_objects (episode_id, created_at);
CREATE INDEX ix_generation_candidates_slot_created_at ON public.generation_candidates (episode_id, usage_type, usage_id, input_version_id, input_hash, created_at DESC);
CREATE INDEX ix_adoptions_episode_slot_created_at ON public.adoptions (episode_id, usage_type, usage_id, input_version_id, input_hash, created_at DESC);

-- Foreign-key support indexes not already covered by a PK/UQ/query index.
CREATE INDEX ix_episodes_current_source_fk ON public.episodes (id, current_source_revision_id);
CREATE INDEX ix_source_revisions_parent_fk ON public.source_revisions (episode_id, parent_id);
CREATE INDEX ix_script_versions_parent_fk ON public.script_versions (episode_id, parent_id);
CREATE INDEX ix_script_versions_source_fk ON public.script_versions (episode_id, source_revision_id);
CREATE INDEX ix_script_versions_origin_task_fk ON public.script_versions (episode_id, origin_task_id);
CREATE INDEX ix_creative_assets_episode_fk ON public.creative_asset_versions (episode_id);
CREATE INDEX ix_creative_assets_parent_fk ON public.creative_asset_versions (asset_id, episode_id, parent_id);
CREATE INDEX ix_creative_assets_source_script_fk ON public.creative_asset_versions (episode_id, source_script_version_id);
CREATE INDEX ix_creative_assets_origin_task_fk ON public.creative_asset_versions (episode_id, origin_task_id);
CREATE INDEX ix_shot_specs_parent_fk ON public.shot_spec_versions (episode_id, parent_id);
CREATE INDEX ix_shot_specs_script_fk ON public.shot_spec_versions (episode_id, script_version_id);
CREATE INDEX ix_shot_specs_origin_task_fk ON public.shot_spec_versions (episode_id, origin_task_id);
CREATE INDEX ix_production_tasks_snapshot_fk ON public.production_tasks (episode_id, snapshot_id);
CREATE INDEX ix_production_tasks_retry_fk ON public.production_tasks (episode_id, retry_of_task_id);
CREATE INDEX ix_production_tasks_current_attempt_fk ON public.production_tasks (id, current_attempt_id);
CREATE INDEX ix_production_attempts_snapshot_fk ON public.production_attempts (task_id, snapshot_id);
CREATE INDEX ix_production_attempts_parent_fk ON public.production_attempts (task_id, parent_attempt_id);
CREATE INDEX ix_media_versions_parent_fk ON public.media_versions (media_object_id, parent_id);
CREATE INDEX ix_generation_candidates_task_fk ON public.generation_candidates (episode_id, task_id);
CREATE INDEX ix_generation_candidates_attempt_fk ON public.generation_candidates (task_id, attempt_id);
CREATE INDEX ix_generation_candidates_media_fk ON public.generation_candidates (media_version_id, attempt_id, output_slot);
CREATE INDEX ix_adoptions_candidate_fk ON public.adoptions (episode_id, usage_type, usage_id, input_version_id, input_hash, candidate_id);
CREATE INDEX ix_adoptions_supersedes_fk ON public.adoptions (episode_id, usage_type, usage_id, input_version_id, input_hash, supersedes_id);
CREATE INDEX ix_subtitle_versions_parent_fk ON public.subtitle_versions (episode_id, parent_id);
CREATE INDEX ix_subtitle_versions_script_fk ON public.subtitle_versions (episode_id, script_version_id);
CREATE INDEX ix_subtitle_versions_shot_spec_fk ON public.subtitle_versions (episode_id, shot_spec_version_id);
CREATE INDEX ix_render_snapshots_initial_task_fk ON public.render_snapshots (episode_id, initial_task_id);
CREATE INDEX ix_render_snapshots_shot_spec_fk ON public.render_snapshots (episode_id, shot_spec_version_id);
CREATE INDEX ix_render_snapshots_subtitle_fk ON public.render_snapshots (episode_id, subtitle_version_id);
CREATE INDEX ix_delivery_versions_render_task_fk ON public.delivery_versions (episode_id, render_task_id);
CREATE INDEX ix_delivery_versions_retry_fk ON public.delivery_versions (episode_id, retry_of_delivery_id);
CREATE INDEX ix_delivery_versions_snapshot_fk ON public.delivery_versions (episode_id, render_snapshot_id);
CREATE INDEX ix_delivery_versions_final_attempt_fk ON public.delivery_versions (render_task_id, final_attempt_id);
CREATE INDEX ix_delivery_versions_mp4_fk ON public.delivery_versions (mp4_media_version_id);
CREATE INDEX ix_delivery_versions_srt_fk ON public.delivery_versions (srt_media_version_id);
CREATE INDEX ix_delivery_versions_manifest_fk ON public.delivery_versions (manifest_media_version_id);
