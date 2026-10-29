-- Apply transactionally; acquire parent table locks before altering dependent foreign keys.
SET LOCAL lock_timeout = '2s';
LOCK TABLE identity."user", workspace.project, media.media_asset IN ACCESS EXCLUSIVE MODE NOWAIT;
-- Private reference voices; supplier training is a separate, explicit operation.
CREATE TABLE IF NOT EXISTS media.voice_profile (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 request_id uuid NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 60),
 request_name text NOT NULL CHECK(length(request_name) BETWEEN 1 AND 60),
 asset_id uuid NOT NULL REFERENCES media.media_asset(id),
 asset_revision bigint NOT NULL CHECK(asset_revision>0),
 consent_at timestamptz NOT NULL,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 favorite boolean NOT NULL DEFAULT false,
 is_delete boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id,actor_id,request_id)
);
CREATE INDEX IF NOT EXISTS voice_profile_owner ON media.voice_profile(project_id,actor_id,id) WHERE NOT is_delete;
GRANT SELECT,INSERT,UPDATE ON media.voice_profile TO glowloom_app;

CREATE TABLE IF NOT EXISTS media.voice_favorite (
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 speaker text NOT NULL CHECK(length(speaker) BETWEEN 1 AND 128),
 favorite boolean NOT NULL,
 PRIMARY KEY(project_id,actor_id,speaker)
);
GRANT SELECT,INSERT,UPDATE ON media.voice_favorite TO glowloom_app;

ALTER TABLE media.voice_profile ADD COLUMN IF NOT EXISTS gender text NOT NULL DEFAULT '未标注' CHECK(gender IN ('未标注','男声','女声'));
ALTER TABLE media.voice_profile ADD COLUMN IF NOT EXISTS age text NOT NULL DEFAULT '未标注' CHECK(age IN ('未标注','儿童','青年','中年','老年'));
ALTER TABLE media.voice_profile ADD COLUMN IF NOT EXISTS language text NOT NULL DEFAULT '未标注' CHECK(language IN ('未标注','中文','英文','日文','其他'));
ALTER TABLE media.voice_profile ADD COLUMN IF NOT EXISTS style text NOT NULL DEFAULT '未标注' CHECK(style IN ('未标注','自然','温柔','活泼','沉稳','旁白'));

-- Voice library metadata expires with its owning project, account or source asset.
ALTER TABLE media.voice_profile DROP CONSTRAINT IF EXISTS voice_profile_project_id_fkey;
ALTER TABLE media.voice_profile ADD CONSTRAINT voice_profile_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id) ON DELETE CASCADE;
ALTER TABLE media.voice_profile DROP CONSTRAINT IF EXISTS voice_profile_actor_id_fkey;
ALTER TABLE media.voice_profile ADD CONSTRAINT voice_profile_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id) ON DELETE CASCADE;
ALTER TABLE media.voice_profile DROP CONSTRAINT IF EXISTS voice_profile_asset_id_fkey;
ALTER TABLE media.voice_profile ADD CONSTRAINT voice_profile_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES media.media_asset(id) ON DELETE CASCADE;
ALTER TABLE media.voice_favorite DROP CONSTRAINT IF EXISTS voice_favorite_project_id_fkey;
ALTER TABLE media.voice_favorite ADD CONSTRAINT voice_favorite_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id) ON DELETE CASCADE;
ALTER TABLE media.voice_favorite DROP CONSTRAINT IF EXISTS voice_favorite_actor_id_fkey;
ALTER TABLE media.voice_favorite ADD CONSTRAINT voice_favorite_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id) ON DELETE CASCADE;
