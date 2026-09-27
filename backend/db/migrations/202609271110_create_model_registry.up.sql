CREATE TABLE catalog.capability (
  id          uuid PRIMARY KEY,
  key         text NOT NULL UNIQUE CHECK (btrim(key) <> ''),
  output_type text NOT NULL CHECK (output_type IN ('json', 'image', 'video', 'audio', 'none')),
  modes       text[] NOT NULL CHECK (cardinality(modes) > 0),
  input_roles text[] NOT NULL,
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  is_delete   boolean NOT NULL DEFAULT false
);

CREATE TABLE catalog.model_profile (
  id                 uuid PRIMARY KEY,
  model_key          text NOT NULL UNIQUE CHECK (btrim(model_key) <> ''),
  provider_id        uuid NOT NULL REFERENCES catalog.provider(id),
  capability         text NOT NULL REFERENCES catalog.capability(key),
  display_name       text NOT NULL CHECK (btrim(display_name) <> ''),
  status             text NOT NULL DEFAULT 'disabled' CHECK (status IN ('active', 'disabled')),
  current_version_id uuid,
  revision           integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  create_time        timestamptz NOT NULL DEFAULT now(),
  update_time        timestamptz NOT NULL DEFAULT now(),
  is_delete          boolean NOT NULL DEFAULT false
);

CREATE INDEX ix_model_profile_provider_not_deleted
  ON catalog.model_profile (provider_id) WHERE NOT is_delete;

CREATE TABLE catalog.model_profile_version (
  id                uuid PRIMARY KEY,
  model_profile_id  uuid NOT NULL REFERENCES catalog.model_profile(id),
  version_no        integer NOT NULL CHECK (version_no > 0),
  provider_model_id text NOT NULL CHECK (btrim(provider_model_id) <> ''),
  modes             text[] NOT NULL CHECK (cardinality(modes) > 0),
  limits            jsonb NOT NULL CHECK (jsonb_typeof(limits) = 'object'),
  param_schema      jsonb NOT NULL CHECK (jsonb_typeof(param_schema) = 'array'),
  supports_query    boolean NOT NULL,
  supports_cancel   boolean NOT NULL,
  supports_callback boolean NOT NULL,
  expected_max_ms   integer NOT NULL CHECK (expected_max_ms > 0),
  moderation        text NOT NULL DEFAULT 'provider' CHECK (moderation IN ('provider', 'platform', 'both')),
  queue             text NOT NULL DEFAULT 'agent' CHECK (btrim(queue) <> ''),
  create_time       timestamptz NOT NULL DEFAULT now(),
  create_by         uuid,
  update_time       timestamptz NOT NULL DEFAULT now(),
  is_delete         boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_model_profile_version UNIQUE (model_profile_id, version_no),
  CONSTRAINT uq_model_profile_version_owner UNIQUE (model_profile_id, id)
);

ALTER TABLE catalog.model_profile
  ADD CONSTRAINT fk_model_profile_current_version
  FOREIGN KEY (id, current_version_id)
  REFERENCES catalog.model_profile_version (model_profile_id, id);

CREATE TABLE catalog.price_rule_version (
  id               uuid PRIMARY KEY,
  model_profile_id uuid NOT NULL REFERENCES catalog.model_profile(id),
  version_no       integer NOT NULL CHECK (version_no > 0),
  unit             text NOT NULL CHECK (unit IN ('per_image', 'per_second', 'per_request', 'per_1k_tokens', 'per_1k_chars')),
  rule             jsonb NOT NULL CHECK (jsonb_typeof(rule) = 'object'),
  currency         char(3) NOT NULL DEFAULT 'CNY' CHECK (currency ~ '^[A-Z]{3}$'),
  fx_rate_to_cny   numeric(12,6) CHECK (fx_rate_to_cny > 0),
  effective_from   timestamptz NOT NULL,
  create_time      timestamptz NOT NULL DEFAULT now(),
  create_by        uuid,
  update_time      timestamptz NOT NULL DEFAULT now(),
  is_delete        boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_price_rule_version UNIQUE (model_profile_id, version_no),
  CONSTRAINT ck_price_rule_foreign_exchange CHECK (currency = 'CNY' OR fx_rate_to_cny IS NOT NULL)
);

CREATE INDEX ix_price_rule_effective
  ON catalog.price_rule_version (model_profile_id, effective_from DESC);
