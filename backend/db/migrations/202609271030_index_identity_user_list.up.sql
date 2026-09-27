CREATE INDEX idx_user_directory ON identity."user" (org_id, create_time DESC, id DESC) WHERE NOT is_delete;
