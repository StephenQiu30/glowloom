import { Pool } from "pg";
import { openOwnedSession } from "../../dist/session-owner.js";
import { GatewayClient } from "../../dist/model-gateway.js";
const pool = new Pool({
  connectionString: process.env.GLOWLOOM_TEST_AGENT_DB_DSN,
});
process.on("message", async ({ identity, model, origin, command }) => {
  try {
    const session = await openOwnedSession({
      pool,
      identity,
      model,
      gateway: new GatewayClient(origin, "internal-test-key".repeat(3)),
    });
    const receipt = await session.acceptCommand(command);
    process.send({ receipt });
  } catch (error) {
    process.send({ error: error.message });
    process.exitCode = 1;
  }
});
