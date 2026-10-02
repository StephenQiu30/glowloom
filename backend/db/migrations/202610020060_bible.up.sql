-- Bible owns stable project identities and immutable typed content histories.
CREATE SCHEMA bible;
GRANT USAGE ON SCHEMA bible TO lanverse_app;

CREATE TABLE bible.character (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
 current_version_id uuid NOT NULL,confirmed_version_id uuid,
 redirect_id uuid,is_delete boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id),CHECK(redirect_id IS NULL OR redirect_id<>id)
);
CREATE TABLE bible.character_version (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,entry_id uuid NOT NULL,
 version_no bigint NOT NULL CHECK(version_no BETWEEN 1 AND 2147483647),previous_id uuid,
 actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 origin text NOT NULL CHECK(origin IN ('manual','ai')),result_source jsonb,
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object' AND pg_column_size(content)<=4194304),
 content_sha256 text NOT NULL CHECK(content_sha256~'^[a-f0-9]{64}$'),
 UNIQUE(org_id,project_id,id),UNIQUE(entry_id,version_no),
 FOREIGN KEY(org_id,project_id,entry_id) REFERENCES bible.character(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,project_id,previous_id) REFERENCES bible.character_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((version_no=1 AND previous_id IS NULL) OR (version_no>1 AND previous_id IS NOT NULL)),
 CHECK((origin='manual' AND result_source IS NULL) OR (origin='ai' AND jsonb_typeof(result_source)='object'))
);
ALTER TABLE bible.character ADD CONSTRAINT character_current_version_fk FOREIGN KEY(org_id,project_id,current_version_id) REFERENCES bible.character_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE bible.character ADD CONSTRAINT character_confirmed_version_fk FOREIGN KEY(org_id,project_id,confirmed_version_id) REFERENCES bible.character_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX character_project_page ON bible.character(org_id,project_id,created_at,id);
CREATE TABLE bible.character_confirmation (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,entry_id uuid NOT NULL,
 version_id uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id),UNIQUE(entry_id,revision),
 FOREIGN KEY(org_id,project_id,entry_id) REFERENCES bible.character(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,version_id) REFERENCES bible.character_version(org_id,project_id,id)
);
GRANT SELECT,INSERT ON bible.character,bible.character_version,bible.character_confirmation TO lanverse_app;
GRANT UPDATE(revision,current_version_id,confirmed_version_id,redirect_id,is_delete,updated_at) ON bible.character TO lanverse_app;

CREATE TABLE bible.location (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
 current_version_id uuid NOT NULL,confirmed_version_id uuid,
 is_delete boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id)
);
CREATE TABLE bible.location_version (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,entry_id uuid NOT NULL,
 version_no bigint NOT NULL CHECK(version_no BETWEEN 1 AND 2147483647),previous_id uuid,
 actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 origin text NOT NULL CHECK(origin IN ('manual','ai')),result_source jsonb,
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object' AND pg_column_size(content)<=4194304),
 content_sha256 text NOT NULL CHECK(content_sha256~'^[a-f0-9]{64}$'),
 UNIQUE(org_id,project_id,id),UNIQUE(entry_id,version_no),
 FOREIGN KEY(org_id,project_id,entry_id) REFERENCES bible.location(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,project_id,previous_id) REFERENCES bible.location_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((version_no=1 AND previous_id IS NULL) OR (version_no>1 AND previous_id IS NOT NULL)),
 CHECK((origin='manual' AND result_source IS NULL) OR (origin='ai' AND jsonb_typeof(result_source)='object'))
);
ALTER TABLE bible.location ADD CONSTRAINT location_current_version_fk FOREIGN KEY(org_id,project_id,current_version_id) REFERENCES bible.location_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE bible.location ADD CONSTRAINT location_confirmed_version_fk FOREIGN KEY(org_id,project_id,confirmed_version_id) REFERENCES bible.location_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX location_project_page ON bible.location(org_id,project_id,created_at,id);
CREATE TABLE bible.location_confirmation (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,entry_id uuid NOT NULL,
 version_id uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id),UNIQUE(entry_id,revision),
 FOREIGN KEY(org_id,project_id,entry_id) REFERENCES bible.location(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,version_id) REFERENCES bible.location_version(org_id,project_id,id)
);
GRANT SELECT,INSERT ON bible.location,bible.location_version,bible.location_confirmation TO lanverse_app;
GRANT UPDATE(revision,current_version_id,confirmed_version_id,is_delete,updated_at) ON bible.location TO lanverse_app;

CREATE TABLE bible.prop (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
 current_version_id uuid NOT NULL,confirmed_version_id uuid,
 is_delete boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id)
);
CREATE TABLE bible.prop_version (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,entry_id uuid NOT NULL,
 version_no bigint NOT NULL CHECK(version_no BETWEEN 1 AND 2147483647),previous_id uuid,
 actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 origin text NOT NULL CHECK(origin IN ('manual','ai')),result_source jsonb,
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object' AND pg_column_size(content)<=4194304),
 content_sha256 text NOT NULL CHECK(content_sha256~'^[a-f0-9]{64}$'),
 UNIQUE(org_id,project_id,id),UNIQUE(entry_id,version_no),
 FOREIGN KEY(org_id,project_id,entry_id) REFERENCES bible.prop(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,project_id,previous_id) REFERENCES bible.prop_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((version_no=1 AND previous_id IS NULL) OR (version_no>1 AND previous_id IS NOT NULL)),
 CHECK((origin='manual' AND result_source IS NULL) OR (origin='ai' AND jsonb_typeof(result_source)='object'))
);
ALTER TABLE bible.prop ADD CONSTRAINT prop_current_version_fk FOREIGN KEY(org_id,project_id,current_version_id) REFERENCES bible.prop_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE bible.prop ADD CONSTRAINT prop_confirmed_version_fk FOREIGN KEY(org_id,project_id,confirmed_version_id) REFERENCES bible.prop_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX prop_project_page ON bible.prop(org_id,project_id,created_at,id);
CREATE TABLE bible.prop_confirmation (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,entry_id uuid NOT NULL,
 version_id uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id),UNIQUE(entry_id,revision),
 FOREIGN KEY(org_id,project_id,entry_id) REFERENCES bible.prop(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,version_id) REFERENCES bible.prop_version(org_id,project_id,id)
);
GRANT SELECT,INSERT ON bible.prop,bible.prop_version,bible.prop_confirmation TO lanverse_app;
GRANT UPDATE(revision,current_version_id,confirmed_version_id,is_delete,updated_at) ON bible.prop TO lanverse_app;

ALTER TABLE bible.character ADD CONSTRAINT character_redirect_fk FOREIGN KEY(org_id,project_id,redirect_id) REFERENCES bible.character(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE bible.look (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,character_id uuid NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,character_id) REFERENCES bible.character(org_id,project_id,id)
);
CREATE TABLE bible.look_version (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,look_id uuid NOT NULL,character_version_id uuid NOT NULL,
 position integer NOT NULL CHECK(position BETWEEN 0 AND 199),name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 512),
 description text NOT NULL CHECK(char_length(description)<=8192),is_default boolean NOT NULL,applies_to jsonb,
 UNIQUE(org_id,project_id,id),UNIQUE(character_version_id,position),UNIQUE(character_version_id,look_id),
 FOREIGN KEY(org_id,project_id,look_id) REFERENCES bible.look(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,character_version_id) REFERENCES bible.character_version(org_id,project_id,id)
);
CREATE UNIQUE INDEX look_one_default ON bible.look_version(character_version_id) WHERE is_default;
CREATE TABLE bible.reference_version (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,look_version_id uuid NOT NULL,
 position integer NOT NULL CHECK(position BETWEEN 0 AND 7),
 role text NOT NULL CHECK(role IN ('primary','front','side','back','turnaround_sheet','expression_sheet')),
 media_asset_id uuid NOT NULL,media_revision bigint NOT NULL CHECK(media_revision>0),
 media_sha256 text NOT NULL CHECK(media_sha256~'^[a-f0-9]{64}$'),media_bytes bigint NOT NULL CHECK(media_bytes>0),
 rendition_id uuid NOT NULL,rendition_sha256 text NOT NULL CHECK(rendition_sha256~'^[a-f0-9]{64}$'),
 UNIQUE(org_id,project_id,id),UNIQUE(look_version_id,role),UNIQUE(look_version_id,position),
 FOREIGN KEY(org_id,project_id,look_version_id) REFERENCES bible.look_version(org_id,project_id,id)
);
CREATE TABLE bible.voice_version (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,character_version_id uuid NOT NULL,
 source_kind text NOT NULL CHECK(source_kind IN ('catalog','sample')),content jsonb NOT NULL CHECK(jsonb_typeof(content)='object'),
 UNIQUE(org_id,project_id,id),UNIQUE(character_version_id),
 FOREIGN KEY(org_id,project_id,character_version_id) REFERENCES bible.character_version(org_id,project_id,id)
);
CREATE TABLE bible.character_redirect (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,source_id uuid NOT NULL,target_id uuid NOT NULL,
 source_version_id uuid NOT NULL,target_version_id uuid NOT NULL,actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id),CHECK(source_id<>target_id),
 FOREIGN KEY(org_id,project_id,source_id) REFERENCES bible.character(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,target_id) REFERENCES bible.character(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,source_version_id) REFERENCES bible.character_version(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,target_version_id) REFERENCES bible.character_version(org_id,project_id,id)
);
CREATE TABLE bible.command (
 actor_id uuid NOT NULL,request_id uuid NOT NULL,org_id uuid NOT NULL,project_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('character','location','prop')),action text NOT NULL,
 request_hash text NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 response jsonb NOT NULL CHECK(jsonb_typeof(response)='object' AND pg_column_size(response)<=16384),created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id)
);
CREATE TABLE bible.character_split (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,source_id uuid NOT NULL,source_version_id uuid NOT NULL,
 target_id uuid NOT NULL,target_version_id uuid NOT NULL,actor_id uuid NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(org_id,project_id,id),CHECK(source_id<>target_id),
 FOREIGN KEY(org_id,project_id,source_id) REFERENCES bible.character(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,target_id) REFERENCES bible.character(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,source_version_id) REFERENCES bible.character_version(org_id,project_id,id),
 FOREIGN KEY(org_id,project_id,target_version_id) REFERENCES bible.character_version(org_id,project_id,id)
);
GRANT SELECT,INSERT ON bible.look,bible.look_version,bible.reference_version,bible.voice_version,bible.character_redirect,bible.character_split,bible.command TO lanverse_app;

-- Copy owns typed immutable plans and proofs; no runtime role can rewrite history or receipts.
CREATE TABLE bible.copy_snapshot (
 id uuid PRIMARY KEY,job_id uuid NOT NULL UNIQUE,org_id uuid NOT NULL,
 source_project_id uuid NOT NULL,target_project_id uuid NOT NULL UNIQUE,
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object'),
 manifest_sha256 text NOT NULL CHECK(manifest_sha256~'^[a-f0-9]{64}$'),
 content_sha256 text NOT NULL CHECK(content_sha256~'^[a-f0-9]{64}$'),created_at timestamptz NOT NULL,
 CHECK(source_project_id<>target_project_id)
);
CREATE TABLE bible.copy_transfer_receipt (
 snapshot_id uuid PRIMARY KEY REFERENCES bible.copy_snapshot(id),
 manifest_sha256 text NOT NULL CHECK(manifest_sha256~'^[a-f0-9]{64}$'),
 content_sha256 text NOT NULL CHECK(content_sha256~'^[a-f0-9]{64}$'),created_at timestamptz NOT NULL
);
CREATE TABLE bible.copy_receipt (LIKE bible.copy_transfer_receipt INCLUDING ALL);
ALTER TABLE bible.copy_receipt ADD FOREIGN KEY(snapshot_id) REFERENCES bible.copy_snapshot(id);
CREATE TABLE bible.copy_cleanup_receipt (LIKE bible.copy_transfer_receipt INCLUDING ALL);
ALTER TABLE bible.copy_cleanup_receipt ADD FOREIGN KEY(snapshot_id) REFERENCES bible.copy_snapshot(id);
GRANT SELECT,INSERT ON bible.copy_snapshot,bible.copy_transfer_receipt,bible.copy_receipt,bible.copy_cleanup_receipt TO lanverse_app;

-- A newly published script structure freezes its exact approved character version.
-- Existing immutable history remains NULL and its original typed JSON stays unchanged.
ALTER TABLE script.dialogue_line ADD COLUMN character_version_id uuid;
ALTER TABLE script.dialogue_line ADD CONSTRAINT dialogue_character_version_pair CHECK(character_version_id IS NULL OR character_id IS NOT NULL);
ALTER TABLE script.dialogue_line ADD CONSTRAINT dialogue_character_version_fk FOREIGN KEY(org_id,project_id,character_version_id) REFERENCES bible.character_version(org_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
