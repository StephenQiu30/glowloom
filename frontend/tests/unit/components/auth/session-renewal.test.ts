import { afterEach, expect, it, vi } from "vitest";
import {
  configureSessionRenewal,
  disposeSessionRenewal,
  ensureSessionFresh,
} from "@/components/auth/session-renewal";
import {
  getCreationScope,
  getSessionGeneration,
  setCreationScope,
  setCSRFToken,
  RequestError,
} from "@/lib/request";
import { getSession, refreshSession, upgradeSession } from "@/api/identity";
vi.mock("@/api/identity", () => ({
  getSession: vi.fn(),
  refreshSession: vi.fn(),
  upgradeSession: vi.fn(),
}));
const receipt = (): API.EmailSessionView => ({
  actor: {
    bio: "",
    id: "11111111-1111-4111-8111-111111111111",
    email: "a@example.com",
    display_name: "A",
    platform_role: "creator",
    status: "active",
    revision: "1",
    capabilities: [],
    avatar_asset_id: "",
    email_verified_at: "",
  },
  session_id: "22222222-2222-4222-8222-222222222222",
  csrf_token: "stable",
  credential_epoch: "1",
  persistent: false,
  session_protocol: "v2",
  refresh_generation: "0",
  upgrade_available: false,
  last_active_at: "2026-10-18T00:00:00Z",
  server_time: "2026-10-18T00:00:00Z",
  idle_expires_at: "2026-10-18T12:00:00Z",
  absolute_expires_at: "2026-10-25T00:00:00Z",
  access_expires_at: "2026-10-18T00:15:00Z",
  refresh_after_at: "2026-10-18T00:14:00Z",
});
afterEach(() => {
  disposeSessionRenewal();
  setCSRFToken(null);
  document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
  vi.resetAllMocks();
});
it("restoreWithoutCookiesStops", async () => {
  configureSessionRenewal(null, () => {});
  await expect(ensureSessionFresh()).rejects.toMatchObject({
    code: "session_invalid",
  });
  expect(refreshSession).not.toHaveBeenCalled();
});
it("refreshKeepsScopeAndDraft", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  const current = receipt();
  setCSRFToken(current.csrf_token);
  setCreationScope("team-viewer");
  const generation = getSessionGeneration();
  const activate = vi.fn();
  configureSessionRenewal(current, activate);
  vi.mocked(refreshSession).mockResolvedValue(current);
  await ensureSessionFresh();
  expect(getCreationScope()).toBe("team-viewer");
  expect(getSessionGeneration()).toBe(generation);
  expect(activate).toHaveBeenCalledWith(current);
});
it("lateRefreshCannotReplaceNewAccount", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  const current = receipt();
  setCSRFToken(current.csrf_token);
  const activate = vi.fn();
  configureSessionRenewal(current, activate);
  let finish!: (s: API.EmailSessionView) => void;
  vi.mocked(refreshSession).mockReturnValue(new Promise((r) => (finish = r)));
  const pending = ensureSessionFresh().catch((e) => e);
  await Promise.resolve();
  setCSRFToken("next-account");
  configureSessionRenewal(
    {
      ...current,
      csrf_token: "next-account",
      session_id: "33333333-3333-4333-8333-333333333333",
    },
    activate,
  );
  finish(current);
  expect((await pending).code).toBe("ERR_CANCELED");
  expect(activate).not.toHaveBeenCalled();
});
it("startup reads only readable csrf proof and uses generated refresh with recovery never", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  configureSessionRenewal(null, () => {});
  vi.mocked(refreshSession).mockResolvedValue(receipt());
  await ensureSessionFresh();
  expect(refreshSession).toHaveBeenCalledWith(
    expect.objectContaining({ sessionRecovery: "never" }),
  );
});
it("one canceled waiter does not cancel twenty other waiters", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  setCSRFToken("stable");
  configureSessionRenewal(receipt(), () => {});
  let finish!: (s: API.EmailSessionView) => void;
  vi.mocked(refreshSession).mockReturnValue(new Promise((r) => (finish = r)));
  const controller = new AbortController();
  const canceled = ensureSessionFresh(controller.signal).catch((e) => e);
  const rest = Array.from({ length: 20 }, () => ensureSessionFresh());
  await Promise.resolve();
  controller.abort();
  finish(receipt());
  await Promise.all(rest);
  expect((await canceled).code).toBe("ERR_CANCELED");
  expect(refreshSession).toHaveBeenCalledTimes(1);
});
it("explicit v1 upgrade uses generated call and validates unknown receipt", async () => {
  const current = {
    ...receipt(),
    session_protocol: "v1",
    upgrade_available: true,
  };
  setCSRFToken("stable");
  configureSessionRenewal(current, () => {});
  vi.mocked(upgradeSession).mockResolvedValue({} as API.EmailSessionView);
  await expect(ensureSessionFresh()).rejects.toEqual(
    new RequestError("invalid_receipt"),
  );
  expect(refreshSession).not.toHaveBeenCalled();
});
it("deleted csrf cookie stops v2 recovery even with in-memory proof", async () => {
  setCSRFToken("stable");
  configureSessionRenewal(receipt(), () => {});
  await expect(ensureSessionFresh()).rejects.toMatchObject({
    code: "session_invalid",
  });
  expect(refreshSession).not.toHaveBeenCalled();
});
it("Web Locks checks latest readable proof before sending, never refreshes another account", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  setCSRFToken("stable");
  configureSessionRenewal(receipt(), () => {});
  let enter!: () => Promise<void>;
  const locks = {
    request: vi.fn(
      (
        _name: string,
        _options: unknown,
        run: () => Promise<API.EmailSessionView>,
      ) =>
        new Promise<API.EmailSessionView>((resolve, reject) => {
          enter = () => run().then(resolve, reject);
        }),
    ),
  };
  vi.stubGlobal("navigator", { locks });
  try {
    const pending = ensureSessionFresh().catch((e) => e);
    document.cookie = "glowloom_csrf=next-account; path=/";
    await enter();
    expect((await pending).code).toBe("ERR_CANCELED");
    expect(refreshSession).not.toHaveBeenCalled();
  } finally {
    vi.unstubAllGlobals();
  }
});

it.each(["success", "scope", "identity"])(
  "all renewal entry points gate protected writes: %s",
  async (boundary) => {
    const { request } = await import("@/lib/request");
    document.cookie = "glowloom_csrf=stable; path=/";
    setCSRFToken("stable");
    setCreationScope("team");
    configureSessionRenewal(receipt(), () => {});
    let finish!: (s: API.EmailSessionView) => void;
    vi.mocked(refreshSession).mockReturnValue(new Promise((r) => (finish = r)));
    const renewal = ensureSessionFresh().catch((e) => e);
    let sends = 0;
    const write = request("/api/protected-write", {
      method: "POST",
      adapter: async (config) => {
        sends++;
        return {
          status: 200,
          statusText: "OK",
          headers: {},
          config,
          data: "sent",
        };
      },
    }).catch((e) => e);
    await new Promise((r) => setTimeout(r, 0));
    expect(sends).toBe(0);
    if (boundary === "scope") setCreationScope("new-team");
    if (boundary === "identity") setCSRFToken("new-account");
    finish(receipt());
    await renewal;
    const result = await write;
    if (boundary === "success") {
      expect(result).toBe("sent");
      expect(sends).toBe(1);
    } else {
      expect(result).toMatchObject({ code: "ERR_CANCELED" });
      expect(sends).toBe(0);
    }
  },
);

it("fresh JSON v1 renewal result never reactivates upgrade", async () => {
  const old = { ...receipt(), session_protocol: "v1", upgrade_available: true };
  setCSRFToken("stable");
  const activate = vi.fn();
  configureSessionRenewal(old, activate);
  vi.mocked(upgradeSession).mockImplementation(async () => ({ ...old }));
  await expect(ensureSessionFresh()).rejects.toMatchObject({
    code: "invalid_receipt",
  });
  expect(activate).not.toHaveBeenCalled();
});
it("late same SID lower durable generation cannot overwrite newer metadata or scope", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  setCSRFToken("stable");
  setCreationScope("team");
  const activate = vi.fn();
  configureSessionRenewal(
    { ...receipt(), refresh_generation: "9007199254740992" },
    activate,
  );
  let finish!: (s: API.EmailSessionView) => void;
  vi.mocked(refreshSession).mockReturnValue(new Promise((r) => (finish = r)));
  const pending = ensureSessionFresh().catch((e) => e);
  await Promise.resolve();
  configureSessionRenewal(
    { ...receipt(), refresh_generation: "9007199254740993" },
    activate,
  );
  finish({ ...receipt(), refresh_generation: "9007199254740992" });
  expect(await pending).toMatchObject({ code: "ERR_CANCELED" });
  expect(activate).not.toHaveBeenCalled();
  expect(getCreationScope()).toBe("team");
});
it("lost upgrade body recovers same actor through one foreground GET", async () => {
  const old = { ...receipt(), session_protocol: "v1", upgrade_available: true };
  setCSRFToken("stable");
  const activate = vi.fn();
  configureSessionRenewal(old, activate);
  vi.mocked(upgradeSession).mockImplementation(async () => {
    document.cookie = "glowloom_csrf=stable; path=/";
    throw new RequestError("request_unconfirmed");
  });
  vi.mocked(getSession).mockResolvedValue({
    ...receipt(),
    refresh_generation: "1",
  });
  await expect(ensureSessionFresh(undefined, true)).resolves.toMatchObject({
    session_protocol: "v2",
    refresh_generation: "1",
  });
  expect(getSession).toHaveBeenCalledTimes(1);
  expect(upgradeSession).toHaveBeenCalledTimes(1);
  expect(activate).toHaveBeenCalledTimes(1);
});

it("without Web Locks startup busy retries one flight and coalesces the winner", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  configureSessionRenewal(null, () => {});
  vi.mocked(refreshSession)
    .mockRejectedValueOnce(new RequestError("refresh_busy", 409))
    .mockResolvedValue(receipt());
  await expect(ensureSessionFresh()).resolves.toMatchObject({
    refresh_generation: "0",
  });
  expect(refreshSession).toHaveBeenCalledTimes(2);
  expect(getSession).not.toHaveBeenCalled();
});
it("repeated coordination conflicts are bounded without GET activity", async () => {
  vi.useFakeTimers();
  document.cookie = "glowloom_csrf=stable; path=/";
  configureSessionRenewal(null, () => {});
  vi.mocked(refreshSession).mockRejectedValue(
    new RequestError("refresh_raced", 409),
  );
  const pending = ensureSessionFresh().catch((e) => e);
  await vi.advanceTimersByTimeAsync(5100);
  expect(await pending).toMatchObject({ code: "refresh_raced" });
  vi.useRealTimers();
  expect(refreshSession).toHaveBeenCalledTimes(3);
  expect(getSession).not.toHaveBeenCalled();
});
it("coordination retry rechecks removed proof", async () => {
  document.cookie = "glowloom_csrf=stable; path=/";
  configureSessionRenewal(null, () => {});
  vi.mocked(refreshSession).mockImplementation(async () => {
    document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
    throw new RequestError("refresh_busy", 409);
  });
  await expect(ensureSessionFresh()).rejects.toMatchObject({
    code: "session_invalid",
  });
  expect(refreshSession).toHaveBeenCalledTimes(1);
});

it("closed owner lease can expire before final bounded retry", async () => {
  vi.useFakeTimers();
  try {
    document.cookie = "glowloom_csrf=stable; path=/";
    configureSessionRenewal(null, () => {});
    const started = Date.now();
    vi.mocked(refreshSession).mockImplementation(async () => {
      if (Date.now() - started < 5000)
        throw new RequestError("refresh_busy", 409);
      return receipt();
    });
    const pending = ensureSessionFresh();
    await vi.advanceTimersByTimeAsync(5100);
    await expect(pending).resolves.toMatchObject({ refresh_generation: "0" });
    expect(refreshSession).toHaveBeenCalledTimes(3);
    expect(getSession).not.toHaveBeenCalled();
  } finally {
    vi.useRealTimers();
  }
});

it("timer lost upgrade body preserves identity without foreground GET", async () => {
  const old = { ...receipt(), session_protocol: "v1", upgrade_available: true };
  setCSRFToken("stable");
  setCreationScope("draft-team");
  const generation = getSessionGeneration();
  const activate = vi.fn();
  configureSessionRenewal(old, activate);
  vi.mocked(upgradeSession).mockImplementation(async () => {
    document.cookie = "glowloom_csrf=stable; path=/";
    throw new RequestError("request_unconfirmed");
  });
  await expect(ensureSessionFresh()).rejects.toMatchObject({
    code: "request_unconfirmed",
  });
  expect(getSession).not.toHaveBeenCalled();
  expect(activate).not.toHaveBeenCalled();
  expect(getSessionGeneration()).toBe(generation);
  expect(getCreationScope()).toBe("draft-team");
  // Repeated timer observes applied headers but never writes or slides idle.
  await expect(ensureSessionFresh()).rejects.toMatchObject({
    code: "request_unconfirmed",
  });
  expect(upgradeSession).toHaveBeenCalledTimes(1);
  vi.mocked(getSession).mockResolvedValue({
    ...receipt(),
    refresh_generation: "1",
  });
  await expect(ensureSessionFresh(undefined, true)).resolves.toMatchObject({
    session_protocol: "v2",
  });
  expect(getSession).toHaveBeenCalledTimes(1);
});
