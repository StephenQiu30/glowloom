CREATE TABLE billing.ledger_entry (
  id                  uuid PRIMARY KEY,
  project_id          uuid NOT NULL,
  entry_type          text NOT NULL CHECK (entry_type IN ('reserve', 'settle', 'release', 'adjust', 'budget_change', 'provider_overage')),
  amount_micros       bigint NOT NULL,
  operation_id        uuid,
  episode_id          uuid,
  shot_id             uuid,
  model_key           text,
  region              text,
  orig_currency       char(3),
  orig_amount_micros  bigint,
  fx_rate             numeric(12,6),
  note                text NOT NULL DEFAULT '',
  create_time         timestamptz NOT NULL DEFAULT now(),
  create_by           uuid,
  update_time         timestamptz NOT NULL DEFAULT now(),
  is_delete           boolean NOT NULL DEFAULT false
);

CREATE INDEX ix_ledger_project_time ON billing.ledger_entry (project_id, create_time);
CREATE INDEX ix_ledger_episode ON billing.ledger_entry (episode_id) WHERE episode_id IS NOT NULL;

CREATE FUNCTION billing.keep_ledger_entry_append_only()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'ledger entries are append only';
  END IF;
  IF OLD.is_delete OR NOT NEW.is_delete OR
     (to_jsonb(NEW) - 'is_delete' - 'update_time') IS DISTINCT FROM
     (to_jsonb(OLD) - 'is_delete' - 'update_time') THEN
    RAISE EXCEPTION 'ledger entries are append only';
  END IF;
  NEW.update_time := now();
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_ledger_entry_append_only
BEFORE UPDATE OR DELETE ON billing.ledger_entry
FOR EACH ROW EXECUTE FUNCTION billing.keep_ledger_entry_append_only();
