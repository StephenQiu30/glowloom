-- Durable local upload responses contain no object keys or preview URLs.
CREATE TABLE media.upload_request (
  project_id uuid NOT NULL REFERENCES workspace.project(id),
  principal_id uuid NOT NULL REFERENCES identity."user"(id),
  request_key uuid NOT NULL CHECK (request_key <> '00000000-0000-0000-0000-000000000000'),
  sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  file_name text NOT NULL CHECK (octet_length(file_name) BETWEEN 1 AND 255),
  byte_size bigint NOT NULL CHECK (byte_size BETWEEN 1 AND 524288000),
  asset_id uuid NOT NULL REFERENCES media.media_asset(id),
  response jsonb NOT NULL CHECK (jsonb_typeof(response)='object' AND octet_length(response::text)<=4096),
  create_time timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(project_id,principal_id,request_key)
);
GRANT SELECT, INSERT ON TABLE media.upload_request TO lanverse_app;
