-- Reviewed, operator-only project naming migration. No business rows are rewritten.
-- Connect to postgres with the schema owner, stop project services, then supply:
-- psql -X -v ON_ERROR_STOP=1 -v source_database=OLD_DATABASE -v source_role=OLD_APP_ROLE -f backend/db/project_resource_rename.sql
-- Existing resource OIDs, permissions, projects, canvases and ledger facts are retained.
SELECT current_database()='postgres'
 AND EXISTS(SELECT 1 FROM pg_database WHERE datname=:'source_database')
 AND NOT EXISTS(SELECT 1 FROM pg_database WHERE datname='glowloom')
 AND EXISTS(SELECT 1 FROM pg_roles WHERE rolname=:'source_role' AND NOT rolcanlogin AND NOT rolsuper)
 AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='glowloom_app')
 AND NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=:'source_database') AS rename_ready
\gset
\if :rename_ready
ALTER ROLE :"source_role" RENAME TO glowloom_app;
ALTER DATABASE :"source_database" RENAME TO glowloom;
\else
\echo 'Resource rename refused: target collision, role mismatch, wrong connection or active project sessions.'
\quit 1
\endif
