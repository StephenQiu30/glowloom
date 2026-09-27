CREATE SCHEMA catalog;

CREATE TABLE catalog.provider (
  id                    uuid PRIMARY KEY,
  key                   text NOT NULL UNIQUE CHECK (btrim(key) <> ''),
  name                  text NOT NULL CHECK (btrim(name) <> ''),
  adapter_key           text NOT NULL CHECK (btrim(adapter_key) <> ''),
  region                text NOT NULL CHECK (region IN ('domestic', 'overseas')),
  status                text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  concurrency_limit     integer NOT NULL DEFAULT 10 CHECK (concurrency_limit > 0),
  rate_limit_per_min    integer NOT NULL DEFAULT 60 CHECK (rate_limit_per_min > 0),
  revision              integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  create_time           timestamptz NOT NULL DEFAULT now(),
  update_time           timestamptz NOT NULL DEFAULT now(),
  is_delete             boolean NOT NULL DEFAULT false
);

CREATE TABLE catalog.provider_credential (
  id               uuid PRIMARY KEY,
  provider_id      uuid NOT NULL REFERENCES catalog.provider(id),
  label            text NOT NULL CHECK (btrim(label) <> ''),
  ciphertext       bytea NOT NULL CHECK (octet_length(ciphertext) > 0),
  key_id           text NOT NULL CHECK (btrim(key_id) <> ''),
  last4            text NOT NULL CHECK (char_length(last4) = 4),
  status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  last_tested_at   timestamptz,
  last_test_result text CHECK (last_test_result IN ('ok', 'auth_failed', 'unreachable', 'timeout', 'unsupported')),
  create_time      timestamptz NOT NULL DEFAULT now(),
  update_time      timestamptz NOT NULL DEFAULT now(),
  is_delete        boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX uq_provider_credential_active
  ON catalog.provider_credential (provider_id)
  WHERE status = 'active' AND NOT is_delete;
