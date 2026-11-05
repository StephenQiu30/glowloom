import { Pool } from "pg";
import { isRestrictedTestConnection } from "./restricted-connection.js";

/** A separate provisioned database prevents the global recovery pump claiming another test's run. */
export function recoveryPumpPool() {
  const dsn = process.env.GLOWLOOM_AGENT_RECOVERY_TEST_DB_DSN;
  const shared = process.env.GLOWLOOM_AGENT_TEST_DB_DSN;
  if (!dsn)
    throw new Error(
      "GLOWLOOM_AGENT_RECOVERY_TEST_DB_DSN must select a separately provisioned isolated *_test database",
    );
  const url = new URL(dsn);
  if (!isRestrictedTestConnection(url))
    throw new Error(
      "Recovery pump tests require the restricted glowloom_agent role in an isolated *_test database",
    );
  if (shared) {
    const other = new URL(shared);
    if (
      other.hostname === url.hostname &&
      other.port === url.port &&
      other.pathname === url.pathname
    )
      throw new Error(
        "Recovery pump test database must differ from the shared Agent test database",
      );
  }
  return new Pool({ connectionString: dsn, max: 4 });
}
