import { Pool } from "pg";
import { BusinessClient } from "../../src/business/client.js";
import { SessionRuntime } from "../../src/sessions/runtime.js";
import type { RunRow } from "../../src/sessions/store.js";

const pool = new Pool({
  connectionString: process.env.GLOWLOOM_AGENT_TEST_DB_DSN,
  max: 4,
});
const run = (
  await pool.query<RunRow>("SELECT * FROM agent.service_runs WHERE id=$1", [
    process.env.RECOVERY_RUN,
  ])
).rows[0]!;
const audit = new Pool({
  connectionString: process.env.GLOWLOOM_AGENT_TEST_DB_DSN,
  max: 1,
});
const faultWindow = process.env.RECOVERY_WINDOW;
if (faultWindow) {
  const connect = pool.connect.bind(pool);
  let reached = false;
  pool.connect = ((...args: unknown[]) => {
    if (args.length) return Reflect.apply(connect, pool, args);
    return (async () => {
      const client = await connect();
      const query = client.query.bind(client);
      return new Proxy(client, {
        get(target, key) {
          if (key === "query")
            return async (...queryArgs: unknown[]) => {
              const result = await Reflect.apply(query, target, queryArgs);
              if (!reached && queryArgs[0] === "COMMIT") {
                const children = await audit.query<{
                  id: string;
                  owner_task_id: string;
                }>(
                  "SELECT id,owner_task_id FROM agent.runtime_records WHERE session_id=$1 AND record_type='conversation' AND owner_task_id IS NOT NULL",
                  [run.session_id],
                );
                const child = children.rows[0];
                if (child) {
                  const submissions = await audit.query<{ id: string }>(
                    "SELECT id FROM agent.runtime_records WHERE session_id=$1 AND record_type='submission' AND conversation_id=$2",
                    [run.session_id, child.id],
                  );
                  const state = await audit.query<{
                    payload: { delegations?: { status: string }[] };
                  }>(
                    "SELECT payload FROM agent.runtime_events WHERE session_id=$1 AND event_type='agent.state' ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 1",
                    [run.session_id],
                  );
                  const hit =
                    faultWindow === "child_create_config" ||
                    (faultWindow === "child_submit" &&
                      submissions.rowCount! > 0) ||
                    (faultWindow === "child_collect" &&
                      state.rows[0]?.payload.delegations?.some(
                        (x) => x.status === "done",
                      ));
                  if (hit) {
                    reached = true;
                    process.send?.({
                      kind: "window",
                      window: faultWindow,
                      child_id: child.id,
                      owner_task_id: child.owner_task_id,
                      submission_ids: submissions.rows.map((x) => x.id),
                    });
                    await new Promise(() => {});
                  }
                }
              }
              return result;
            };
          const value = Reflect.get(target, key);
          return typeof value === "function" ? value.bind(target) : value;
        },
      });
    })();
  }) as typeof pool.connect;
}
const runtime = await SessionRuntime.open(
  pool,
  new BusinessClient(process.env.RECOVERY_ORIGIN!, "fixture"),
  run,
  1200,
);
process.send?.({ kind: "opened" });
try {
  await runtime.start();
  process.send?.({ kind: "settled", state: await runtime.state() });
} catch (error) {
  process.send?.({ kind: "error", error: String(error) });
} finally {
  await runtime.close();
  await pool.end();
  await audit.end();
}
