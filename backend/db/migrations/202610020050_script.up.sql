-- Formal script owns immutable input/content history and small mutable heads.
CREATE SCHEMA IF NOT EXISTS script;
GRANT USAGE ON SCHEMA script TO lanverse_app;

CREATE TABLE script.script_source (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL,
 source_lineage_id uuid NOT NULL, previous_source_id uuid,
 source_revision bigint NOT NULL CHECK(source_revision>0),
 origin text NOT NULL CHECK(origin IN ('manual','beeftv','file')),
 source_kind text NOT NULL CHECK(source_kind IN ('chapter','episode','document')),
 title text NOT NULL CHECK(char_length(title) BETWEEN 1 AND 512),
 status text NOT NULL CHECK(status IN ('draft','ready','completed')),
 rights_actor_id uuid NOT NULL, rights_confirmed_at timestamptz NOT NULL,
 media_asset_id uuid, media_revision bigint, media_sha256 text,
 original_key text NOT NULL, original_sha256 text NOT NULL CHECK(original_sha256~'^[a-f0-9]{64}$'),
 original_bytes bigint NOT NULL CHECK(original_bytes BETWEEN 0 AND 20971520), original_mime text NOT NULL,
 rich_key text NOT NULL, rich_sha256 text NOT NULL CHECK(rich_sha256~'^[a-f0-9]{64}$'),
 rich_bytes bigint NOT NULL CHECK(rich_bytes BETWEEN 0 AND 8388608),
 content_hash text NOT NULL CHECK(content_hash~'^[a-f0-9]{64}$'), char_count integer NOT NULL CHECK(char_count BETWEEN 0 AND 1500000),
 provenance jsonb NOT NULL CHECK(jsonb_typeof(provenance)='object'), created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id), UNIQUE(project_id,source_lineage_id,source_revision),
 FOREIGN KEY(org_id,project_id,previous_source_id) REFERENCES script.script_source(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,project_id,source_lineage_id) REFERENCES script.script_source(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((media_asset_id IS NULL AND media_revision IS NULL AND media_sha256 IS NULL) OR (origin='file' AND media_asset_id IS NOT NULL AND media_revision>0 AND media_sha256~'^[a-f0-9]{64}$')),
 CHECK((source_revision=1 AND previous_source_id IS NULL AND source_lineage_id=id) OR (source_revision>1 AND previous_source_id IS NOT NULL))
);
CREATE INDEX script_source_project ON script.script_source(org_id,project_id,created_at,id);

CREATE TABLE script.script_version (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL,
 version_no bigint NOT NULL CHECK(version_no>0), source_ids uuid[] NOT NULL,
 source_spans jsonb NOT NULL CHECK(jsonb_typeof(source_spans)='array'),
 text_key text NOT NULL, text_sha256 text NOT NULL CHECK(text_sha256~'^[a-f0-9]{64}$'), text_bytes bigint NOT NULL CHECK(text_bytes BETWEEN 0 AND 6000000),
 rich_key text NOT NULL, rich_sha256 text NOT NULL CHECK(rich_sha256~'^[a-f0-9]{64}$'), rich_bytes bigint NOT NULL CHECK(rich_bytes BETWEEN 0 AND 33554432),
 content_hash text NOT NULL CHECK(content_hash=text_sha256), document_sha256 text NOT NULL CHECK(document_sha256=rich_sha256),
 source_manifest_sha256 text NOT NULL CHECK(source_manifest_sha256~'^[a-f0-9]{64}$'),
 char_count integer NOT NULL CHECK(char_count BETWEEN 0 AND 1500000), created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id), UNIQUE(project_id,version_no), UNIQUE(project_id,source_manifest_sha256)
);
CREATE INDEX script_version_content ON script.script_version(project_id,content_hash);
CREATE INDEX script_version_document ON script.script_version(project_id,document_sha256);
CREATE TABLE script.version_source (
 org_id uuid NOT NULL, project_id uuid NOT NULL, version_id uuid NOT NULL, source_id uuid NOT NULL, position integer NOT NULL CHECK(position>=0),
 PRIMARY KEY(version_id,position), UNIQUE(version_id,source_id),
 FOREIGN KEY(org_id,project_id,version_id) REFERENCES script.script_version(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,source_id) REFERENCES script.script_source(org_id,project_id,id)
);
CREATE TABLE script.project_state (
 project_id uuid PRIMARY KEY, org_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 draft_version_id uuid NOT NULL, adopted_version_id uuid, updated_at timestamptz NOT NULL,
 FOREIGN KEY(org_id,project_id,draft_version_id) REFERENCES script.script_version(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,adopted_version_id) REFERENCES script.script_version(org_id,project_id,id)
);
CREATE TABLE script.split_set (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, version_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('candidate','formal')), origin text NOT NULL CHECK(origin IN ('sources','rules','manual')),
 preface jsonb, boundaries jsonb NOT NULL CHECK(jsonb_typeof(boundaries)='array'), warnings jsonb NOT NULL CHECK(jsonb_typeof(warnings)='array'), created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id), FOREIGN KEY(org_id,project_id,version_id) REFERENCES script.script_version(org_id,project_id,id)
);
CREATE TABLE script.version_head (
 org_id uuid NOT NULL, project_id uuid NOT NULL, version_id uuid PRIMARY KEY, split_revision bigint NOT NULL DEFAULT 0 CHECK(split_revision>=0),
 candidate_split_set_id uuid NOT NULL, confirmed_split_set_id uuid,
 FOREIGN KEY(org_id,project_id,version_id) REFERENCES script.script_version(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,candidate_split_set_id) REFERENCES script.split_set(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,confirmed_split_set_id) REFERENCES script.split_set(org_id,project_id,id)
);
CREATE TABLE script.episode (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, script_version_id uuid NOT NULL, split_set_id uuid NOT NULL,
 seq_no integer NOT NULL CHECK(seq_no>0), title text NOT NULL CHECK(char_length(title) BETWEEN 1 AND 512), span_start integer NOT NULL CHECK(span_start>=0), span_end integer NOT NULL CHECK(span_end>span_start),
 revision bigint NOT NULL CHECK(revision>0), current_structure_id uuid, confirmed_structure_id uuid,
 previous_episode_id uuid, inherit_status text NOT NULL DEFAULT 'not_inherited' CHECK(inherit_status IN ('not_inherited','pending','inherited')),
 is_delete boolean NOT NULL DEFAULT false,
 UNIQUE(org_id,project_id,id), FOREIGN KEY(org_id,project_id,script_version_id) REFERENCES script.script_version(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,split_set_id) REFERENCES script.split_set(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,previous_episode_id) REFERENCES script.episode(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED
);
CREATE UNIQUE INDEX script_episode_active_seq ON script.episode(split_set_id,seq_no) WHERE NOT is_delete;
CREATE TABLE script.episode_structure (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, episode_id uuid NOT NULL, version_no bigint NOT NULL CHECK(version_no>0),
 source_hash text NOT NULL CHECK(source_hash~'^[a-f0-9]{64}$'), document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'), actor_id uuid NOT NULL, created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id), UNIQUE(episode_id,version_no), FOREIGN KEY(org_id,project_id,episode_id) REFERENCES script.episode(org_id,project_id,id)
);
ALTER TABLE script.episode ADD FOREIGN KEY(org_id,project_id,current_structure_id) REFERENCES script.episode_structure(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE script.episode ADD FOREIGN KEY(org_id,project_id,confirmed_structure_id) REFERENCES script.episode_structure(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE script.scene (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, episode_structure_id uuid NOT NULL, scene_key uuid NOT NULL, seq_no integer NOT NULL CHECK(seq_no>0),
 heading text NOT NULL, location_text text NOT NULL, time_of_day text NOT NULL, span_start integer NOT NULL, span_end integer NOT NULL CHECK(span_end>span_start),
 UNIQUE(org_id,project_id,id), UNIQUE(episode_structure_id,scene_key), UNIQUE(episode_structure_id,seq_no),
 FOREIGN KEY(org_id,project_id,episode_structure_id) REFERENCES script.episode_structure(org_id,project_id,id)
);
CREATE TABLE script.dialogue_line (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, scene_id uuid NOT NULL, line_key uuid NOT NULL, seq_no integer NOT NULL CHECK(seq_no>0),
 kind text NOT NULL CHECK(kind IN ('dialogue','voiceover','inner')), content text NOT NULL, speaker_text text NOT NULL, character_id uuid, emotion text NOT NULL, content_hash text NOT NULL CHECK(content_hash~'^[a-f0-9]{64}$'), span_start integer NOT NULL, span_end integer NOT NULL CHECK(span_end>span_start),
 UNIQUE(org_id,project_id,id), UNIQUE(scene_id,line_key), FOREIGN KEY(org_id,project_id,scene_id) REFERENCES script.scene(org_id,project_id,id)
);
CREATE TABLE script.action_line (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, scene_id uuid NOT NULL, line_key uuid NOT NULL, seq_no integer NOT NULL CHECK(seq_no>0),
 content text NOT NULL, span_start integer NOT NULL, span_end integer NOT NULL CHECK(span_end>span_start),
 UNIQUE(org_id,project_id,id), UNIQUE(scene_id,line_key), FOREIGN KEY(org_id,project_id,scene_id) REFERENCES script.scene(org_id,project_id,id)
);
CREATE TABLE script.split_confirmation (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, version_id uuid NOT NULL, candidate_set_id uuid NOT NULL, formal_set_id uuid NOT NULL,
 actor_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0), preface jsonb, episodes jsonb NOT NULL CHECK(jsonb_typeof(episodes)='array'), created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id), UNIQUE(version_id,revision),
 FOREIGN KEY(org_id,project_id,version_id) REFERENCES script.script_version(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,candidate_set_id) REFERENCES script.split_set(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,formal_set_id) REFERENCES script.split_set(org_id,project_id,id)
);
CREATE TABLE script.command (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, org_id uuid NOT NULL, project_id uuid NOT NULL,
 action text NOT NULL, request_hash text NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'), plan jsonb NOT NULL CHECK(jsonb_typeof(plan)='object'), created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id)
);
CREATE TABLE script.command_state (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, id uuid NOT NULL UNIQUE, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 status text NOT NULL CHECK(status IN ('pending','completed','cancelled')), failure_code text NOT NULL DEFAULT '',
 cancellation_requested boolean NOT NULL DEFAULT false, needs_reconciliation boolean NOT NULL DEFAULT false,
 io_owner_id uuid, io_state text NOT NULL DEFAULT 'idle' CHECK(io_state IN ('idle','running','ended','unknown')), updated_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id), FOREIGN KEY(actor_id,request_id) REFERENCES script.command(actor_id,request_id)
);
CREATE TABLE script.command_result (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, response jsonb NOT NULL CHECK(jsonb_typeof(response)='object'), created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id), FOREIGN KEY(actor_id,request_id) REFERENCES script.command(actor_id,request_id)
);
CREATE TABLE script.object_intent (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, org_id uuid NOT NULL, project_id uuid NOT NULL,
 object_key text PRIMARY KEY, sha256 text NOT NULL CHECK(sha256~'^[a-f0-9]{64}$'), byte_size bigint NOT NULL CHECK(byte_size BETWEEN 0 AND 37748736), mime text NOT NULL,
 FOREIGN KEY(actor_id,request_id) REFERENCES script.command(actor_id,request_id)
);
GRANT SELECT,INSERT ON ALL TABLES IN SCHEMA script TO lanverse_app;
GRANT UPDATE(revision,draft_version_id,adopted_version_id,updated_at) ON script.project_state TO lanverse_app;
GRANT UPDATE(split_revision,candidate_split_set_id,confirmed_split_set_id) ON script.version_head TO lanverse_app;
GRANT UPDATE(title,revision,current_structure_id,confirmed_structure_id,previous_episode_id,inherit_status,is_delete) ON script.episode TO lanverse_app;
GRANT UPDATE(revision,status,failure_code,cancellation_requested,needs_reconciliation,io_owner_id,io_state,updated_at) ON script.command_state TO lanverse_app;

CREATE TABLE script.review_command (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, org_id uuid NOT NULL, project_id uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('resplit','confirm_split','save_structure','confirm_structure','adopt')),
 request_hash text NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'), response jsonb NOT NULL CHECK(jsonb_typeof(response)='object'), created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id)
);
GRANT SELECT,INSERT ON script.review_command TO lanverse_app;

CREATE TABLE script.request (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, org_id uuid NOT NULL, project_id uuid NOT NULL,
 action text NOT NULL, request_hash text NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'), created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id)
);
GRANT SELECT,INSERT ON script.request TO lanverse_app;
ALTER TABLE script.command ADD FOREIGN KEY(actor_id,request_id) REFERENCES script.request(actor_id,request_id);
ALTER TABLE script.review_command ADD FOREIGN KEY(actor_id,request_id) REFERENCES script.request(actor_id,request_id);

CREATE TABLE script.object_state (
 object_key text PRIMARY KEY REFERENCES script.object_intent(object_key),
 put_started boolean NOT NULL DEFAULT false, confirmed boolean NOT NULL DEFAULT false, removed boolean NOT NULL DEFAULT false, updated_at timestamptz NOT NULL
);
GRANT SELECT,INSERT ON script.object_state TO lanverse_app;
GRANT UPDATE(put_started,confirmed,removed,updated_at) ON script.object_state TO lanverse_app;
CREATE TABLE script.source_control (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, org_id uuid NOT NULL, project_id uuid NOT NULL, intent_id uuid NOT NULL REFERENCES script.command_state(id),
 action text NOT NULL CHECK(action IN ('cancel','reconcile')), expected_revision bigint NOT NULL CHECK(expected_revision>0),
 request_hash text NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'), response jsonb NOT NULL CHECK(jsonb_typeof(response)='object'), created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id), FOREIGN KEY(actor_id,request_id) REFERENCES script.request(actor_id,request_id)
);
GRANT SELECT,INSERT ON script.source_control TO lanverse_app;

-- These immutable receipts cover all history; no execution or financial fact is copied.
CREATE TABLE script.copy_snapshot (
 id uuid PRIMARY KEY, job_id uuid NOT NULL UNIQUE, org_id uuid NOT NULL,
 source_project_id uuid NOT NULL, target_project_id uuid NOT NULL UNIQUE,
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object'),
 manifest_sha256 text NOT NULL CHECK(manifest_sha256~'^[a-f0-9]{64}$'),
 content_sha256 text NOT NULL CHECK(content_sha256~'^[a-f0-9]{64}$'), created_at timestamptz NOT NULL,
 CHECK(source_project_id<>target_project_id)
);
CREATE TABLE script.copy_object_intent (
 target_key text PRIMARY KEY, snapshot_id uuid NOT NULL REFERENCES script.copy_snapshot(id), source_key text NOT NULL,
 sha256 text NOT NULL CHECK(sha256~'^[a-f0-9]{64}$'), byte_size bigint NOT NULL CHECK(byte_size BETWEEN 0 AND 37748736), mime text NOT NULL,
 UNIQUE(snapshot_id,source_key), CHECK(source_key<>target_key)
);
CREATE TABLE script.copy_object_state (
 target_key text PRIMARY KEY REFERENCES script.copy_object_intent(target_key),
 put_started boolean NOT NULL, confirmed boolean NOT NULL, removed boolean NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE script.copy_receipt (
 snapshot_id uuid PRIMARY KEY REFERENCES script.copy_snapshot(id), receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object'), created_at timestamptz NOT NULL
);
GRANT SELECT,INSERT ON script.copy_snapshot,script.copy_object_intent,script.copy_object_state,script.copy_receipt TO lanverse_app;
GRANT UPDATE(put_started,confirmed,removed,updated_at) ON script.copy_object_state TO lanverse_app;

-- Document imports freeze originals and preserve each actual per-file attempt.
CREATE TABLE script.import_job (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, actor_id uuid NOT NULL,
 actor_role text NOT NULL CHECK(actor_role IN ('admin','producer')),
 request_id uuid NOT NULL, request_hash text NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 expected_script_revision bigint NOT NULL CHECK(expected_script_revision>=0), base_version_id uuid,
 rights_confirmed_at timestamptz NOT NULL, created_at timestamptz NOT NULL,
 UNIQUE(actor_id,request_id), UNIQUE(org_id,project_id,id),
 FOREIGN KEY(actor_id,request_id) REFERENCES script.request(actor_id,request_id),
 FOREIGN KEY(org_id,project_id,base_version_id) REFERENCES script.script_version(org_id,project_id,id)
);
CREATE TABLE script.import_state (
 job_id uuid PRIMARY KEY REFERENCES script.import_job(id), revision bigint NOT NULL CHECK(revision>0), attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 status text NOT NULL CHECK(status IN ('queued','running','partial','succeeded','failed','cancel_requested','cancelled')),
 stage text NOT NULL CHECK(stage IN ('queued','extracting','normalizing','storing','committing','completed','failed','cancelling','awaiting_reconciliation')),
 latest_script_revision bigint NOT NULL CHECK(latest_script_revision>=0), latest_version_id uuid,
 cancellation_requested boolean NOT NULL DEFAULT false, reconciliation_requested boolean NOT NULL DEFAULT false, needs_reconciliation boolean NOT NULL DEFAULT false,
 failure_code text NOT NULL DEFAULT '', io_owner_id uuid, io_state text NOT NULL CHECK(io_state IN ('idle','running','ended','unknown')), updated_at timestamptz NOT NULL
);
CREATE TABLE script.import_file (
 job_id uuid NOT NULL REFERENCES script.import_job(id), position integer NOT NULL CHECK(position BETWEEN 0 AND 199),
 source_id uuid NOT NULL UNIQUE, asset_id uuid NOT NULL, project_id uuid NOT NULL,
 media_revision bigint NOT NULL CHECK(media_revision>0), sha256 text NOT NULL CHECK(sha256~'^[a-f0-9]{64}$'),
 byte_size bigint NOT NULL CHECK(byte_size BETWEEN 0 AND 20971520), mime text NOT NULL CHECK(mime IN ('text/plain','application/vnd.openxmlformats-officedocument.wordprocessingml.document')), file_name text NOT NULL,
 PRIMARY KEY(job_id,position), UNIQUE(job_id,asset_id)
);
CREATE TABLE script.import_attempt (
 job_id uuid NOT NULL REFERENCES script.import_job(id), attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 expected_script_revision bigint NOT NULL CHECK(expected_script_revision>=0), base_version_id uuid,
 publication_key uuid NOT NULL UNIQUE, positions integer[] NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,attempt)
);
CREATE TABLE script.import_file_result (
 job_id uuid NOT NULL, attempt integer NOT NULL, position integer NOT NULL,
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object'), created_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,attempt,position), FOREIGN KEY(job_id,attempt) REFERENCES script.import_attempt(job_id,attempt),
 FOREIGN KEY(job_id,position) REFERENCES script.import_file(job_id,position)
);
CREATE TABLE script.import_publication (
 job_id uuid NOT NULL, attempt integer NOT NULL, response jsonb NOT NULL CHECK(jsonb_typeof(response)='object'), created_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,attempt), FOREIGN KEY(job_id,attempt) REFERENCES script.import_attempt(job_id,attempt)
);
CREATE TABLE script.import_command (
 actor_id uuid NOT NULL, request_id uuid NOT NULL, org_id uuid NOT NULL, project_id uuid NOT NULL, job_id uuid NOT NULL REFERENCES script.import_job(id),
 action text NOT NULL CHECK(action IN ('create','cancel','retry','reconcile')), request_hash text NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 expected_revision bigint NOT NULL CHECK(expected_revision>=0), event_id uuid NOT NULL UNIQUE, event_action text NOT NULL CHECK(event_action IN ('start','cancel','reconcile')), event_attempt integer NOT NULL,
 response jsonb NOT NULL CHECK(jsonb_typeof(response)='object'), created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id), FOREIGN KEY(actor_id,request_id) REFERENCES script.request(actor_id,request_id)
);
GRANT SELECT,INSERT ON script.import_job,script.import_state,script.import_file,script.import_attempt,script.import_file_result,script.import_publication,script.import_command TO lanverse_app;
GRANT UPDATE(revision,attempt,status,stage,latest_script_revision,latest_version_id,cancellation_requested,reconciliation_requested,needs_reconciliation,failure_code,io_owner_id,io_state,updated_at) ON script.import_state TO lanverse_app;
