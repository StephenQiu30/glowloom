ALTER TABLE operation.operation DROP CONSTRAINT fk_operation_reservation_identity;
DROP TABLE billing.reservation;
DROP TABLE operation.operation_input;
DROP TABLE operation.operation;
DROP TABLE operation.batch;
DROP SCHEMA operation;
