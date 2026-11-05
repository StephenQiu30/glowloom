import { randomUUID } from "node:crypto";
import { registerStorageConformance } from "@earendil-works/pi-durable/testing";
import { BACKGROUND_CONTEXT } from "@earendil-works/chord/context";
import { afterAll, describe, expect, it } from "vitest";
import { claimSessionOwner } from "../../src/storage/ownership.js";
import { openPostgresStorage } from "../../src/storage/postgres.js";
import { testPool } from "./database.js";

const pool = testPool();
afterAll(() => pool.end());

registerStorageConformance(
  { describe, expect, it },
  "PostgreSQL Storage",
  async (use) => {
    const session = randomUUID();
    const owner = await claimSessionOwner(pool, session, randomUUID(), 60_000);
    const storage = await openPostgresStorage(pool, session, owner);
    try {
      await use(storage);
    } finally {
      await storage.close(BACKGROUND_CONTEXT);
    }
  },
);
