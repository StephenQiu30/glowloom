-- Install the transaction-scoped ORM lock row used to serialize route changes
-- with new storage admissions, including the first activation.
BEGIN;
CREATE TABLE IF NOT EXISTS media.storage_route_lock (
  singleton integer NOT NULL PRIMARY KEY,
  CONSTRAINT storage_route_lock_singleton_check CHECK (singleton = 1)
);
INSERT INTO media.storage_route_lock(singleton) VALUES (1)
ON CONFLICT (singleton) DO NOTHING;
GRANT SELECT, UPDATE(singleton) ON TABLE media.storage_route_lock TO glowloom_app;
COMMIT;
