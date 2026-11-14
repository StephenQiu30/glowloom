import { Pool } from "pg";
import { PgStore } from "./store";
import { AccessError } from "./auth/session";
let store: PgStore | undefined;
export function getConfig() {
  return {
    secret: process.env.GLOWLOOM_WORKSPACE_SESSION_SECRET ?? "",
    origin: process.env.GLOWLOOM_WORKSPACE_ORIGIN ?? "",
    socket: process.env.GLOWLOOM_WORKSPACE_BRIDGE_SOCKET ?? "",
    bridgeToken: process.env.GLOWLOOM_WORKSPACE_BRIDGE_TOKEN ?? "",
    database: !!process.env.GLOWLOOM_WORKSPACE_DB_DSN,
  };
}
export function getStore() {
  const dsn = process.env.GLOWLOOM_WORKSPACE_DB_DSN;
  if (!dsn) throw new AccessError("database_unconfigured", 503);
  return (store ??= new PgStore(
    new Pool({
      connectionString: dsn,
      max: 5,
      connectionTimeoutMillis: 3000,
      statement_timeout: 15000,
    }),
  ));
}
