CREATE EXTENSION IF NOT EXISTS citext;

CREATE SCHEMA identity;

CREATE TABLE identity."user" (
  id                   uuid PRIMARY KEY,
  org_id               uuid NOT NULL,
  login_name           citext NOT NULL,
  display_name         text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 50),
  role                 text NOT NULL CHECK (role IN ('admin', 'producer')),
  status               text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  password_hash        text NOT NULL,
  must_change_password boolean NOT NULL DEFAULT true,
  password_changed_at  timestamptz,
  failed_login_count   integer NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
  locked_until         timestamptz,
  session_epoch        integer NOT NULL DEFAULT 1 CHECK (session_epoch > 0),
  last_login_at        timestamptz,
  revision             integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  create_time          timestamptz NOT NULL DEFAULT now(),
  update_time          timestamptz NOT NULL DEFAULT now(),
  is_delete            boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX uq_user_login_name ON identity."user" (org_id, login_name) WHERE NOT is_delete;
