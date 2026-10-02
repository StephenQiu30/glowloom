-- Copy cleanup already owns confirmed target-object absence. Grant only its
-- three missing soft-deletion columns; identity and object ownership stay closed.
GRANT UPDATE (is_delete, delete_time, purge_after) ON media.media_asset TO lanverse_app;
