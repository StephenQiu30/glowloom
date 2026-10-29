// @vitest-environment node
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { afterAll, afterEach, beforeAll, expect, it, vi } from "vitest";
import {
  configureSessionRenewal,
  disposeSessionRenewal,
  ensureSessionFresh,
} from "@/components/auth/session-renewal";
import { setCSRFToken } from "@/lib/request";

const transport = vi.hoisted(() => ({ baseURL: "" }));
vi.mock("@/api/identity", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/identity")>();
  return {
    ...actual,
    refreshSession: (options: Parameters<typeof actual.refreshSession>[0]) =>
      actual.refreshSession({ ...options, baseURL: transport.baseURL }),
    upgradeSession: (options: Parameters<typeof actual.upgradeSession>[0]) =>
      actual.upgradeSession({ ...options, baseURL: transport.baseURL }),
  };
});

const receipt: API.EmailSessionView = {
  actor: {
    id: "11111111-1111-4111-8111-111111111111",
    email: "delivery@example.invalid",
    email_verified_at: "2026-10-29T00:00:00Z",
    display_name: "Delivery fixture",
    bio: "",
    avatar_asset_id: "",
    platform_role: "creator",
    status: "active",
    revision: "1",
    capabilities: [],
  },
  session_id: "22222222-2222-4222-8222-222222222222",
  csrf_token: "synthetic-delivery-proof",
  credential_epoch: "1",
  persistent: false,
  session_protocol: "v2",
  refresh_generation: "1",
  upgrade_available: false,
  last_active_at: "2026-10-29T00:00:00Z",
  server_time: "2026-10-29T00:00:00Z",
  idle_expires_at: "2026-10-29T12:00:00Z",
  absolute_expires_at: "2026-11-05T00:00:00Z",
  access_expires_at: "2026-10-29T00:15:00Z",
  refresh_after_at: "2026-10-29T00:14:00Z",
};

let server: Server;
let commits = 0;
let requests = 0;
let lockHeld = false;
let lockHeldAtCommit = false;
let lockHeldAtDelivery = false;
const timers = new Set<ReturnType<typeof setTimeout>>();
beforeAll(async () => {
  server = createServer((incoming, response) => {
    incoming.resume();
    ++requests;
    // Model a frontend queue before the bounded upstream operation begins.
    timers.add(
      setTimeout(() => {
        ++commits;
        lockHeldAtCommit = lockHeld;
      }, 14_000),
    );
    timers.add(
      setTimeout(() => {
        lockHeldAtDelivery = lockHeld;
        response.writeHead(200, {
          "Content-Type": "application/json",
          Connection: "close",
          "Set-Cookie": "synthetic_refresh=next; HttpOnly; Path=/",
        });
        response.end(JSON.stringify(receipt));
      }, 16_000),
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  transport.baseURL = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});
afterEach(() => {
  disposeSessionRenewal();
  setCSRFToken(null);
  vi.unstubAllGlobals();
  for (const timer of timers) clearTimeout(timer);
  timers.clear();
  server.closeAllConnections();
});
afterAll(async () => {
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
});

it.each(["refresh", "upgrade"] as const)(
  "%s holds its identity lock until a delayed committed receipt arrives",
  async (kind) => {
    commits = requests = 0;
    lockHeldAtCommit = lockHeldAtDelivery = false;
    vi.stubGlobal("document", {
      cookie:
        kind === "refresh" ? "glowloom_csrf=synthetic-delivery-proof" : "",
    });
    vi.stubGlobal("location", { protocol: "http:" });
    const lock = vi.fn(
      async (_name: string, _options: unknown, run: () => Promise<unknown>) => {
        lockHeld = true;
        try {
          return await run();
        } finally {
          lockHeld = false;
        }
      },
    );
    vi.stubGlobal("navigator", { locks: { request: lock } });
    setCSRFToken(receipt.csrf_token);
    configureSessionRenewal(
      {
        ...receipt,
        session_protocol: kind === "upgrade" ? "v1" : "v2",
        refresh_generation: "0",
        upgrade_available: kind === "upgrade",
      },
      () => {},
    );
    const result = await ensureSessionFresh().then(
      (value) => ({ value }),
      (error: unknown) => ({ error }),
    );
    expect(commits).toBe(1);
    expect(requests).toBe(1);
    expect(lock).toHaveBeenCalledTimes(1);
    expect(lockHeldAtCommit).toBe(true);
    expect(result).toEqual({ value: receipt });
    expect(lockHeldAtDelivery).toBe(true);
    expect(lockHeld).toBe(false);
  },
  25_000,
);

it("identity disposal still aborts delayed delivery and releases the lock", async () => {
  commits = requests = 0;
  vi.stubGlobal("document", {
    cookie: "glowloom_csrf=synthetic-delivery-proof",
  });
  vi.stubGlobal("location", { protocol: "http:" });
  vi.stubGlobal("navigator", {
    locks: {
      request: async (
        _name: string,
        _options: unknown,
        run: () => Promise<unknown>,
      ) => {
        lockHeld = true;
        try {
          return await run();
        } finally {
          lockHeld = false;
        }
      },
    },
  });
  setCSRFToken(receipt.csrf_token);
  configureSessionRenewal(receipt, () => {});
  const pending = ensureSessionFresh().catch((error: unknown) => error);
  await vi.waitFor(() => expect(requests).toBe(1));
  disposeSessionRenewal();
  expect(await pending).toMatchObject({ code: "ERR_CANCELED" });
  expect(lockHeld).toBe(false);
  expect(commits).toBe(0);
});
