ALTER TABLE script.command DROP CONSTRAINT IF EXISTS command_plan_check;
ALTER TABLE script.command
    ADD CONSTRAINT command_plan_check
    CHECK (jsonb_typeof(plan) = 'object' AND octet_length(plan::text) <= 113246208);
