import { Pool } from "pg";
import { isRestrictedTestConnection } from "../helpers/restricted-connection.js";

export function testPool() {
  const dsn = process.env.GLOWLOOM_AGENT_TEST_DB_DSN;
  if (!dsn || !isRestrictedTestConnection(new URL(dsn))) {
    throw new Error(
      "GLOWLOOM_AGENT_TEST_DB_DSN must select an isolated *_test database with role glowloom_agent",
    );
  }
  return new Pool({ connectionString: dsn, max: 4 });
}
