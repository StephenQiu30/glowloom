CREATE SCHEMA billing;

CREATE TABLE billing.budget (
  id              uuid PRIMARY KEY,
  project_id      uuid NOT NULL,
  limit_micros    bigint NOT NULL CHECK (limit_micros >= 0),
  reserved_micros bigint NOT NULL DEFAULT 0 CHECK (reserved_micros >= 0),
  settled_micros  bigint NOT NULL DEFAULT 0 CHECK (settled_micros >= 0),
  is_overrun      boolean NOT NULL DEFAULT false,
  revision        integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  update_time     timestamptz NOT NULL DEFAULT now(),
  update_by       uuid,
  create_time     timestamptz NOT NULL DEFAULT now(),
  is_delete       boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_budget_project UNIQUE (project_id),
  CONSTRAINT ck_budget_balance CHECK (is_overrun OR reserved_micros + settled_micros <= limit_micros)
);

CREATE FUNCTION billing.set_budget_update_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.update_time := now();
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_budget_update_time
BEFORE UPDATE ON billing.budget
FOR EACH ROW EXECUTE FUNCTION billing.set_budget_update_time();
