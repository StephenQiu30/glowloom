-- Run as schema owner. Existing reservations retain their original charging policy.
ALTER TABLE billing.reservation ADD COLUMN IF NOT EXISTS cap_at_quote boolean NOT NULL DEFAULT false;
ALTER TABLE operation.provider_call ADD COLUMN IF NOT EXISTS procurement_cost_micros bigint CHECK (procurement_cost_micros >= 0);
GRANT UPDATE (procurement_cost_micros) ON operation.provider_call TO glowloom_app;

CREATE TABLE IF NOT EXISTS billing.wallet_account (
    scope_id uuid PRIMARY KEY REFERENCES workspace.creation_scope(id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing.project_wallet (
    project_id uuid PRIMARY KEY REFERENCES workspace.project(id),
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id)
);

CREATE TABLE IF NOT EXISTS billing.wallet_lot (
    id uuid PRIMARY KEY,
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id),
    source_kind text NOT NULL CHECK (source_kind IN ('promotional', 'paid', 'subscription', 'expiry_return')),
    source_id uuid NOT NULL,
    issued_micros bigint NOT NULL CHECK (issued_micros > 0),
    available_micros bigint NOT NULL CHECK (available_micros BETWEEN 0 AND issued_micros),
    expires_at timestamptz,
    model_key text CHECK (length(model_key) BETWEEN 1 AND 256),
    root_lot_id uuid REFERENCES billing.wallet_lot(id),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_kind, source_id, root_lot_id),
    UNIQUE (scope_id, id),
    CHECK ((source_kind = 'expiry_return') = (root_lot_id IS NOT NULL)),
    CHECK ((source_kind = 'paid') = (expires_at IS NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS wallet_lot_source_root ON billing.wallet_lot(source_kind, source_id) WHERE root_lot_id IS NULL;
CREATE INDEX IF NOT EXISTS wallet_lot_scope_available ON billing.wallet_lot(scope_id, expires_at, id) WHERE available_micros > 0;

CREATE TABLE IF NOT EXISTS billing.wallet_hold (
    reservation_id uuid PRIMARY KEY REFERENCES billing.reservation(id),
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id),
    UNIQUE (reservation_id, scope_id)
);

CREATE TABLE IF NOT EXISTS billing.wallet_allocation (
    reservation_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    lot_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    amount_micros bigint NOT NULL CHECK (amount_micros > 0),
    PRIMARY KEY (reservation_id, lot_id),
    UNIQUE (reservation_id, ordinal),
    FOREIGN KEY (reservation_id, scope_id) REFERENCES billing.wallet_hold(reservation_id, scope_id),
    FOREIGN KEY (scope_id, lot_id) REFERENCES billing.wallet_lot(scope_id, id)
);

CREATE TABLE IF NOT EXISTS billing.wallet_entry (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_id uuid NOT NULL,
    lot_id uuid NOT NULL,
    reservation_id uuid REFERENCES billing.reservation(id),
    kind text NOT NULL CHECK (kind IN ('issue', 'hold', 'redeem', 'release', 'expire', 'restore', 'compensate')),
    amount_micros bigint NOT NULL CHECK (amount_micros > 0),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (scope_id, lot_id) REFERENCES billing.wallet_lot(scope_id, id)
);

CREATE INDEX IF NOT EXISTS wallet_entry_scope_time ON billing.wallet_entry(scope_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS wallet_entry_per_reservation ON billing.wallet_entry(reservation_id, lot_id, kind) WHERE reservation_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS wallet_entry_issue ON billing.wallet_entry(lot_id) WHERE kind IN ('issue', 'restore');

CREATE TABLE IF NOT EXISTS billing.wallet_expiry_grace (
    root_lot_id uuid PRIMARY KEY REFERENCES billing.wallet_lot(id),
    expires_at timestamptz NOT NULL
);

-- Only the schema owner/finance role may bind projects or issue initial lots.
GRANT SELECT ON billing.wallet_account, billing.project_wallet TO glowloom_app;
-- PostgreSQL requires an UPDATE privilege to take a row lock.
GRANT UPDATE (created_at) ON billing.wallet_account TO glowloom_app;
GRANT SELECT, INSERT ON billing.wallet_hold, billing.wallet_allocation, billing.wallet_entry, billing.wallet_expiry_grace TO glowloom_app;
GRANT SELECT ON billing.wallet_lot TO glowloom_app;
GRANT UPDATE (available_micros) ON billing.wallet_lot TO glowloom_app;
-- Only a return lot may be inserted by the runtime; root issuance is separately controlled.
CREATE OR REPLACE FUNCTION billing.insert_wallet_return_lot(
    lot uuid, scope uuid, reservation uuid, amount bigint, deadline timestamptz, root uuid, at_time timestamptz
) RETURNS uuid
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog
AS $$
    INSERT INTO billing.wallet_lot
        (id, scope_id, source_kind, source_id, issued_micros, available_micros, expires_at, model_key, root_lot_id, reason, created_at)
    SELECT lot, scope, 'expiry_return', reservation, amount, amount, deadline, l.model_key, root, 'expired hold return', at_time
    FROM billing.wallet_lot l
    JOIN billing.wallet_expiry_grace g ON g.root_lot_id=l.id AND g.expires_at=deadline
    JOIN billing.wallet_hold h ON h.reservation_id=reservation AND h.scope_id=scope
    WHERE l.id=root AND l.scope_id=scope AND l.root_lot_id IS NULL AND deadline>at_time
      AND amount <= COALESCE((SELECT sum(e.amount_micros) FROM billing.wallet_entry e
        JOIN billing.wallet_lot original ON original.id=e.lot_id
        WHERE e.reservation_id=reservation AND e.kind IN ('expire','compensate')
          AND coalesce(original.root_lot_id,original.id)=root), 0)
    RETURNING id
$$;
REVOKE ALL ON FUNCTION billing.insert_wallet_return_lot(uuid,uuid,uuid,bigint,timestamptz,uuid,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION billing.insert_wallet_return_lot(uuid,uuid,uuid,bigint,timestamptz,uuid,timestamptz) TO glowloom_app;

CREATE OR REPLACE FUNCTION billing.bind_project_wallet(project uuid) RETURNS uuid
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE payer uuid;
BEGIN
    SELECT owner_scope_id INTO STRICT payer FROM workspace.project
        WHERE id=$1 AND status='active' AND NOT is_delete FOR SHARE;
    PERFORM id FROM billing.budget WHERE project_id=$1 AND NOT is_delete FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'active project budget required'; END IF;
    INSERT INTO billing.wallet_account(scope_id) VALUES (payer) ON CONFLICT DO NOTHING;
    INSERT INTO billing.project_wallet(project_id,scope_id) VALUES ($1,payer)
        ON CONFLICT (project_id) DO UPDATE SET scope_id=excluded.scope_id;
    RETURN payer;
END
$$;
REVOKE ALL ON FUNCTION billing.bind_project_wallet(uuid) FROM PUBLIC;

CREATE OR REPLACE FUNCTION billing.issue_promotional_credit(
    scope uuid, source uuid, amount bigint, deadline timestamptz, model text, justification text
) RETURNS uuid
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE lot billing.wallet_lot%ROWTYPE;
DECLARE current_timepoint timestamptz := clock_timestamp();
BEGIN
    PERFORM scope_id FROM billing.wallet_account WHERE scope_id=scope FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'wallet account required'; END IF;
    SELECT * INTO lot FROM billing.wallet_lot
        WHERE source_kind='promotional' AND source_id=source AND root_lot_id IS NULL;
    IF FOUND THEN
        IF lot.scope_id IS DISTINCT FROM scope OR lot.issued_micros IS DISTINCT FROM amount OR lot.expires_at IS DISTINCT FROM deadline
          OR lot.model_key IS DISTINCT FROM model OR lot.reason IS DISTINCT FROM justification THEN
            RAISE EXCEPTION 'credit issuance identity conflict';
        END IF;
        RETURN lot.id;
    END IF;
    IF amount IS NULL OR amount<=0 OR deadline IS NULL OR deadline<=current_timepoint THEN
        RAISE EXCEPTION 'positive credit and future expiry required';
    END IF;
    INSERT INTO billing.wallet_lot
        (id,scope_id,source_kind,source_id,issued_micros,available_micros,expires_at,model_key,reason,created_at)
    VALUES (gen_random_uuid(),scope,'promotional',source,amount,amount,deadline,model,justification,current_timepoint)
    RETURNING * INTO lot;
    INSERT INTO billing.wallet_entry(scope_id,lot_id,kind,amount_micros,created_at)
        VALUES (scope,lot.id,'issue',amount,current_timepoint);
    RETURN lot.id;
END
$$;
REVOKE ALL ON FUNCTION billing.issue_promotional_credit(uuid,uuid,bigint,timestamptz,text,text) FROM PUBLIC;

CREATE TABLE IF NOT EXISTS billing.wallet_compensation (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id uuid NOT NULL UNIQUE REFERENCES billing.wallet_hold(reservation_id),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    reviewed_by text NOT NULL,
    created_at timestamptz NOT NULL
);

-- Finance approves compensation using the original reservation. It leaves
-- project consumption and supplier liabilities available for reconciliation.
CREATE OR REPLACE FUNCTION billing.compensate_wallet_consumption(reservation uuid, justification text) RETURNS uuid
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE payer uuid;
DECLARE correction billing.wallet_compensation%ROWTYPE;
DECLARE allocation record;
DECLARE root uuid;
DECLARE returned_lot uuid;
DECLARE deadline timestamptz;
DECLARE current_timepoint timestamptz := clock_timestamp();
BEGIN
    SELECT h.scope_id INTO STRICT payer
        FROM billing.reservation r JOIN billing.wallet_hold h ON h.reservation_id=r.id
        WHERE r.id=$1 AND r.status='settled' AND NOT r.is_delete FOR UPDATE OF r;
    PERFORM scope_id FROM billing.wallet_account WHERE scope_id=payer FOR UPDATE;
    SELECT * INTO correction FROM billing.wallet_compensation WHERE reservation_id=$1;
    IF FOUND THEN
        IF correction.reason IS DISTINCT FROM justification THEN
            RAISE EXCEPTION 'credit compensation identity conflict';
        END IF;
        RETURN correction.id;
    END IF;
    INSERT INTO billing.wallet_compensation(reservation_id,reason,reviewed_by,created_at)
        VALUES ($1,justification,current_user,current_timepoint) RETURNING * INTO correction;
    FOR allocation IN
        SELECT l.id,l.expires_at,l.root_lot_id,l.model_key,e.amount_micros
        FROM billing.wallet_entry e JOIN billing.wallet_lot l ON l.id=e.lot_id
        WHERE e.reservation_id=$1 AND e.scope_id=payer AND e.kind='redeem'
        ORDER BY l.id
    LOOP
        INSERT INTO billing.wallet_entry(scope_id,lot_id,reservation_id,kind,amount_micros,created_at)
            VALUES (payer,allocation.id,$1,'compensate',allocation.amount_micros,current_timepoint);
        IF allocation.expires_at IS NULL OR allocation.expires_at>current_timepoint THEN
            UPDATE billing.wallet_lot SET available_micros=available_micros+allocation.amount_micros
                WHERE id=allocation.id;
        ELSE
            root := coalesce(allocation.root_lot_id,allocation.id);
            INSERT INTO billing.wallet_expiry_grace(root_lot_id,expires_at)
                VALUES (root,current_timepoint+interval '168 hours') ON CONFLICT DO NOTHING;
            SELECT expires_at INTO STRICT deadline FROM billing.wallet_expiry_grace WHERE root_lot_id=root;
            IF deadline<=current_timepoint THEN
                RAISE EXCEPTION 'credit compensation requires expiry review';
            END IF;
            INSERT INTO billing.wallet_lot
                (id,scope_id,source_kind,source_id,issued_micros,available_micros,expires_at,model_key,root_lot_id,reason,created_at)
            VALUES (gen_random_uuid(),payer,'expiry_return',gen_random_uuid(),allocation.amount_micros,
                allocation.amount_micros,deadline,allocation.model_key,root,'reviewed compensation',current_timepoint)
            RETURNING id INTO returned_lot;
            INSERT INTO billing.wallet_entry(scope_id,lot_id,reservation_id,kind,amount_micros,created_at)
                VALUES (payer,returned_lot,$1,'restore',allocation.amount_micros,current_timepoint);
        END IF;
    END LOOP;
    IF NOT FOUND THEN RAISE EXCEPTION 'wallet redemption evidence required'; END IF;
    RETURN correction.id;
END
$$;
REVOKE ALL ON FUNCTION billing.compensate_wallet_consumption(uuid,text) FROM PUBLIC;

-- Require funding for every new capped reservation and any wallet-bound project.
CREATE OR REPLACE FUNCTION billing.check_wallet_reservation() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF TG_OP='INSERT' AND NEW.amount_micros>0 AND
       (NEW.cap_at_quote OR EXISTS(SELECT 1 FROM billing.project_wallet WHERE project_id=NEW.project_id)) THEN
        IF NOT NEW.cap_at_quote OR
           NOT EXISTS(SELECT 1 FROM billing.wallet_hold WHERE reservation_id=NEW.id) OR
           coalesce((SELECT sum(amount_micros) FROM billing.wallet_allocation WHERE reservation_id=NEW.id),0)<>NEW.amount_micros THEN
            RAISE EXCEPTION 'wallet-backed reservation requires complete funding' USING ERRCODE='23514';
        END IF;
    ELSIF TG_OP='UPDATE' AND OLD.status='held' AND NEW.status IN ('settled','released') AND
          EXISTS(SELECT 1 FROM billing.wallet_hold WHERE reservation_id=NEW.id) THEN
        IF coalesce((SELECT sum(amount_micros) FROM billing.wallet_entry
          WHERE reservation_id=NEW.id AND kind IN ('redeem','release','expire')),0)<>NEW.amount_micros THEN
            RAISE EXCEPTION 'wallet-backed reservation requires complete settlement' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END
$$;
DROP TRIGGER IF EXISTS wallet_reservation_consistency ON billing.reservation;
CREATE CONSTRAINT TRIGGER wallet_reservation_consistency
AFTER INSERT OR UPDATE ON billing.reservation DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION billing.check_wallet_reservation();
