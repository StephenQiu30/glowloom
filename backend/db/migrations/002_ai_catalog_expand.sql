-- Additive AI catalog expansion. Apply only after reviewing 001 and backing up
-- the target database. No existing connection is activated or verified here.
BEGIN;
CREATE TABLE catalog.provider_preset (
    id uuid PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z][a-z0-9_-]{0,63}$'),
    origin text NOT NULL CHECK (origin IN ('builtin','custom')),
    parent_id uuid REFERENCES catalog.provider_preset(id),
    archived boolean NOT NULL DEFAULT false,
    emergency_disabled boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    update_time timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE catalog.provider_preset_version (
    id uuid PRIMARY KEY,
    preset_id uuid NOT NULL REFERENCES catalog.provider_preset(id),
    version_no integer NOT NULL CHECK (version_no > 0),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object'),
    content_hash text NOT NULL CHECK (length(content_hash)=64),
    create_time timestamptz NOT NULL DEFAULT now(),
    UNIQUE (preset_id,version_no)
);
CREATE TABLE catalog.provider_config_version (
    id uuid PRIMARY KEY,
    provider_id uuid NOT NULL REFERENCES catalog.provider(id),
    preset_version_id uuid REFERENCES catalog.provider_preset_version(id),
    version_no integer NOT NULL CHECK (version_no > 0),
    config jsonb NOT NULL CHECK (jsonb_typeof(config)='object'),
    content_hash text NOT NULL CHECK (length(content_hash)=64),
    create_time timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id,version_no),
    UNIQUE (provider_id,id)
);
ALTER TABLE catalog.provider ADD COLUMN current_config_version_id uuid;
ALTER TABLE catalog.provider ADD CONSTRAINT provider_current_config_fk
    FOREIGN KEY (id,current_config_version_id) REFERENCES catalog.provider_config_version(provider_id,id);
ALTER TABLE catalog.provider_credential DROP CONSTRAINT provider_credential_status_check;
ALTER TABLE catalog.provider_credential ADD CONSTRAINT provider_credential_status_check CHECK (status IN ('active','draining','disabled'));
CREATE TABLE catalog.config_request (
    actor_id uuid NOT NULL,
    action text NOT NULL,
    target_id uuid NOT NULL,
    request_id uuid NOT NULL,
    fingerprint text NOT NULL,
    result jsonb NOT NULL,
    create_time timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(actor_id,action,target_id,request_id)
);
CREATE TABLE catalog.provider_check (
    id uuid PRIMARY KEY,
    actor_id uuid NOT NULL REFERENCES identity."user"(id),
    provider_id uuid NOT NULL REFERENCES catalog.provider(id),
    config_version_id uuid NOT NULL REFERENCES catalog.provider_config_version(id),
    credential_id uuid NOT NULL REFERENCES catalog.provider_credential(id),
    kind text NOT NULL CHECK(kind IN ('connectivity','discovery')),
    status text NOT NULL CHECK(status IN ('pending','running','completed','failed')),
    auth_result text NOT NULL DEFAULT 'unknown',
    adapter_result text NOT NULL DEFAULT 'unknown',
    discovery_result text NOT NULL DEFAULT 'unknown',
    error_code text,
    create_time timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
CREATE TABLE catalog.model_candidate (
    id uuid PRIMARY KEY,
    check_id uuid NOT NULL REFERENCES catalog.provider_check(id),
    provider_model_id text NOT NULL,
    display_name text NOT NULL,
    capability text NOT NULL,
    metadata jsonb NOT NULL,
    UNIQUE(check_id,provider_model_id,capability)
);
CREATE TABLE catalog.model_release (
    id uuid PRIMARY KEY,
    model_id uuid NOT NULL REFERENCES catalog.model_profile(id),
    model_version_id uuid NOT NULL REFERENCES catalog.model_profile_version(id),
    price_version_id uuid NOT NULL REFERENCES catalog.price_rule_version(id),
    config_version_id uuid NOT NULL REFERENCES catalog.provider_config_version(id),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','retired')),
    upstream_cost jsonb NOT NULL CHECK (jsonb_typeof(upstream_cost)='object'),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
    create_time timestamptz NOT NULL DEFAULT now(),
    activated_at timestamptz,
    UNIQUE(model_id,id)
);
CREATE TABLE catalog.model_validation (
    id uuid PRIMARY KEY,
    release_id uuid NOT NULL REFERENCES catalog.model_release(id),
    credential_id uuid NOT NULL REFERENCES catalog.provider_credential(id),
    adapter_version text NOT NULL,
    mode text NOT NULL,
    input_digest text NOT NULL,
    operation_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('pending','running','succeeded','failed','unknown')),
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    create_time timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE(operation_id)
);
ALTER TABLE catalog.model_profile ADD COLUMN active_release_id uuid;
ALTER TABLE catalog.model_profile ADD CONSTRAINT model_active_release_fk
    FOREIGN KEY (id,active_release_id) REFERENCES catalog.model_release(model_id,id);
CREATE TABLE catalog.capability_default (
    capability text PRIMARY KEY REFERENCES catalog.capability(key),
    release_id uuid NOT NULL REFERENCES catalog.model_release(id),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
    update_time timestamptz NOT NULL DEFAULT now()
);

-- Version 1 builtins: portable configuration only; no models or keys are guessed.
INSERT INTO catalog.provider_preset(id,slug,origin) VALUES('6da07a10-0000-4000-8000-000000000001','volcengine-ark','builtin');
INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash) VALUES('6da07a11-0000-4000-8000-000000000001','6da07a10-0000-4000-8000-000000000001',1,'{"schema_version":1,"name":"火山方舟","config":{"adapter_key":"volcengine_ark","base_url":"https://ark.cn-beijing.volces.com/api/v3","region":"domestic","capabilities":["video"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}}'::jsonb,'a357018c3c198f6d58123c99cca6978fe6068a44e48b3794a0bcef2dbf9ed078');
INSERT INTO catalog.provider_preset(id,slug,origin) VALUES('6da07a10-0000-4000-8000-000000000002','openrouter','builtin');
INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash) VALUES('6da07a11-0000-4000-8000-000000000002','6da07a10-0000-4000-8000-000000000002',1,'{"schema_version":1,"name":"OpenRouter","config":{"adapter_key":"openrouter","base_url":"https://openrouter.ai/api/v1","region":"overseas","capabilities":["text","image","video"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}}'::jsonb,'621b784416c935788a8fe5a39a9775a3eed7651307db4c2f14c646cab165b765');


-- Capability vocabulary is platform-owned; models opt into a verified subset.
INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES
 ('6da07a12-0000-4000-8000-000000000001','text.generate','json',ARRAY['text_to_text'],ARRAY['prompt']),
 ('6da07a12-0000-4000-8000-000000000002','image.generate','image',ARRAY['text2image','image2image'],ARRAY['prompt','reference_image','subject']),
 ('6da07a12-0000-4000-8000-000000000003','video.generate','video',ARRAY['text2video','image2video','omni_reference'],ARRAY['prompt','first_frame','last_frame','reference_image','subject'])
ON CONFLICT(key) DO UPDATE SET
 modes=ARRAY(SELECT DISTINCT value FROM unnest(catalog.capability.modes || EXCLUDED.modes) AS value ORDER BY value),
 input_roles=ARRAY(SELECT DISTINCT value FROM unnest(catalog.capability.input_roles || EXCLUDED.input_roles) AS value ORDER BY value);
GRANT SELECT,INSERT,UPDATE ON catalog.provider_preset,catalog.provider_check,catalog.model_release,catalog.model_validation,catalog.capability_default TO lanverse_app;
GRANT SELECT,INSERT ON catalog.provider_preset_version,catalog.provider_config_version,catalog.config_request,catalog.model_candidate TO lanverse_app;
COMMIT;
