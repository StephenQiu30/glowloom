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
