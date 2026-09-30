-- Nullable fields deliberately preserve legacy uncertainty. A NULL timestamp
-- on an old call is not evidence that the provider request was never sent.
ALTER TABLE operation.provider_call
  ADD COLUMN dispatch_started_at timestamptz,
  ADD COLUMN receipt jsonb,
  ADD CONSTRAINT ck_provider_call_receipt CHECK (
    receipt IS NULL OR COALESCE(
      jsonb_typeof(receipt) = 'object'
      AND octet_length(receipt::text) <= 4096
      AND receipt - ARRAY['version', 'identity', 'manifest_sha256', 'outputs'] = '{}'::jsonb
      AND jsonb_typeof(receipt -> 'version') = 'number'
      AND receipt ->> 'version' = '1'
      AND jsonb_typeof(receipt -> 'manifest_sha256') = 'string'
      AND receipt ->> 'manifest_sha256' ~ '^[0-9a-f]{64}$'
      AND jsonb_typeof(receipt -> 'identity') = 'object'
      AND (receipt -> 'identity') - ARRAY[
        'project_id', 'operation_id', 'action', 'attempt', 'request_key',
        'model_profile_version_id', 'price_rule_version_id'
      ] = '{}'::jsonb
      AND jsonb_typeof(receipt #> '{identity,project_id}') = 'string'
      AND receipt #>> '{identity,project_id}' = project_id::text
      AND jsonb_typeof(receipt #> '{identity,operation_id}') = 'string'
      AND receipt #>> '{identity,operation_id}' = operation_id::text
      AND jsonb_typeof(receipt #> '{identity,action}') = 'string'
      AND receipt #>> '{identity,action}' = 'submit' AND action = 'submit'
      AND jsonb_typeof(receipt #> '{identity,attempt}') = 'number'
      AND receipt #>> '{identity,attempt}' = attempt::text
      AND jsonb_typeof(receipt #> '{identity,request_key}') = 'string'
      AND receipt #>> '{identity,request_key}' = request_key
      AND octet_length(receipt #>> '{identity,request_key}') BETWEEN 1 AND 256
      AND btrim(receipt #>> '{identity,request_key}') = receipt #>> '{identity,request_key}'
      AND jsonb_typeof(receipt #> '{identity,model_profile_version_id}') = 'string'
      AND receipt #>> '{identity,model_profile_version_id}' = model_profile_version_id::text
      AND jsonb_typeof(receipt #> '{identity,price_rule_version_id}') = 'string'
      AND receipt #>> '{identity,price_rule_version_id}' = price_rule_version_id::text
      AND CASE WHEN jsonb_typeof(receipt -> 'outputs') = 'array'
        THEN jsonb_array_length(receipt -> 'outputs') = 1 ELSE false END
      AND jsonb_typeof(receipt #> '{outputs,0}') = 'object'
      AND (receipt #> '{outputs,0}') - ARRAY['sequence', 'size_bytes', 'mime_type', 'sha256'] = '{}'::jsonb
      AND jsonb_typeof(receipt #> '{outputs,0,sequence}') = 'number'
      AND receipt #>> '{outputs,0,sequence}' = '1'
      AND CASE WHEN jsonb_typeof(receipt #> '{outputs,0,size_bytes}') = 'number'
        AND receipt #>> '{outputs,0,size_bytes}' ~ '^[0-9]+$'
        THEN (receipt #>> '{outputs,0,size_bytes}')::numeric BETWEEN 1 AND 33554432
        ELSE false END
      AND jsonb_typeof(receipt #> '{outputs,0,mime_type}') = 'string'
      AND receipt #>> '{outputs,0,mime_type}' IN ('image/png', 'image/jpeg', 'image/webp')
      AND jsonb_typeof(receipt #> '{outputs,0,sha256}') = 'string'
      AND receipt #>> '{outputs,0,sha256}' ~ '^[0-9a-f]{64}$'
      AND dispatch_started_at IS NOT NULL
      AND request_summary ->> 'dispatch_contract' = 'v1'
      AND outcome = 'ok' AND response_summary ->> 'state' = 'completed'
      AND provider_task_id IS NULL,
      false
    )
  );
