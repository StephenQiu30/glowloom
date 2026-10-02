LOCK TABLE media.upload_request IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media.upload_request WHERE project_id IS NULL OR personal_org_id IS NOT NULL) THEN
  RAISE EXCEPTION 'retained personal upload receipts prevent rollback' USING ERRCODE='55000';
 END IF;
END $$;
DROP TRIGGER upload_request_scope_guard ON media.upload_request;
DROP FUNCTION media.check_upload_receipt_scope();
ALTER TABLE media.upload_request DROP CONSTRAINT upload_request_scope_check;
ALTER TABLE media.upload_request DROP CONSTRAINT upload_request_personal_actor_fk;
ALTER TABLE media.upload_request DROP CONSTRAINT upload_request_personal_unique;
ALTER TABLE media.upload_request DROP CONSTRAINT upload_request_project_unique;
ALTER TABLE media.upload_request DROP COLUMN personal_org_id;
ALTER TABLE media.upload_request ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE media.upload_request ADD PRIMARY KEY(project_id,principal_id,request_key);
