-- Apply as one transaction with writers stopped. Preserve historical profile
-- identities and refuse the cutover while any live work still needs them.
LOCK TABLE media.storage_profile IN ACCESS EXCLUSIVE MODE;
LOCK TABLE media.object_location, media.storage_work_intent, media.upload_admission IN SHARE MODE;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM media.object_location l
    JOIN media.storage_profile p ON p.id = l.profile_id
    WHERE p.provider <> 'cos' AND l.state <> 'deleted'
  ) OR EXISTS (
    SELECT 1 FROM media.storage_work_intent w
    JOIN media.storage_profile p ON p.id = w.profile_id
    WHERE p.provider <> 'cos' AND w.state = 'open'
  ) OR EXISTS (
    SELECT 1 FROM media.upload_admission a
    JOIN media.storage_profile p ON p.id = a.profile_id
    WHERE p.provider <> 'cos' AND a.accepted_asset_id IS NULL AND a.expires_at > now()
  ) THEN
    RAISE EXCEPTION 'COS cutover blocked by active objects or work on a non-COS storage profile';
  END IF;
END
$$;

ALTER TABLE media.storage_profile DROP CONSTRAINT IF EXISTS storage_profile_provider_check;
ALTER TABLE media.storage_profile ADD CONSTRAINT storage_profile_provider_check
  CHECK (provider = 'cos') NOT VALID;
