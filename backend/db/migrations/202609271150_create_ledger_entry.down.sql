DROP TRIGGER trg_ledger_entry_append_only ON billing.ledger_entry;
DROP FUNCTION billing.keep_ledger_entry_append_only();
DROP TABLE billing.ledger_entry;
