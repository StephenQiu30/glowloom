import { Pool } from "pg";
import { backfillConversationMessages } from "../src/sessions/backfill-messages.js";
const dsn = process.env.GLOWLOOM_AGENT_DB_DSN;
if (!dsn)
  throw new Error(
    "GLOWLOOM_AGENT_DB_DSN is required; this offline command never loads business configuration",
  );
const pool = new Pool({ connectionString: dsn, max: 1 });
try {
  process.stdout.write(
    JSON.stringify(await backfillConversationMessages(pool)) + "\n",
  );
} finally {
  await pool.end();
}
