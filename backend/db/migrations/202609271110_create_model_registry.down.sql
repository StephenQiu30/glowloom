DROP TABLE catalog.price_rule_version;
ALTER TABLE catalog.model_profile DROP CONSTRAINT fk_model_profile_current_version;
DROP TABLE catalog.model_profile_version;
DROP TABLE catalog.model_profile;
DROP TABLE catalog.capability;
