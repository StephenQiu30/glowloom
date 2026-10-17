-- Future accounts default to dark; existing explicit preferences remain intact.
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '10s';

ALTER TABLE identity."user" ALTER COLUMN theme SET DEFAULT 'dark';
