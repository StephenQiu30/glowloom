-- Account profile and appearance preferences. Safe to rerun on existing databases.
BEGIN;
ALTER TABLE identity."user" ADD COLUMN IF NOT EXISTS bio text NOT NULL DEFAULT '';
ALTER TABLE identity."user" ADD COLUMN IF NOT EXISTS canvas_background text NOT NULL DEFAULT 'dots';
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='identity."user"'::regclass AND conname='user_bio_check') THEN
  ALTER TABLE identity."user" ADD CONSTRAINT user_bio_check CHECK (char_length(bio)<=100);
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='identity."user"'::regclass AND conname='user_canvas_background_check') THEN
  ALTER TABLE identity."user" ADD CONSTRAINT user_canvas_background_check CHECK (canvas_background IN ('dots','lines','blank'));
 END IF;
END $$;
COMMIT;
