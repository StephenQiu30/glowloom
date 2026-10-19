/* eslint-disable @typescript-eslint/no-explicit-any -- Minimal asynchronous native IDB adapter; application boundaries remain real. */
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  fireEvent,
} from "@testing-library/react";
import { AxiosError, type InternalAxiosRequestConfig } from "axios";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  SessionProvider,
  SessionGate,
  useOptionalSession,
  revokeSession,
} from "@/components/auth/session";
import {
  ensureSessionFresh,
  isCurrentSession,
  sessionIdentitySource,
} from "@/components/auth/session-renewal";
import {
  request,
  getCSRFToken,
  getSessionGeneration,
  setCreationScope,
} from "@/lib/request";
import {
  branchJournal,
  type LocalBranch,
} from "@/components/generation/draft-save";
import { SpaceProvider } from "@/components/spaces/space-context";
import { HomePage } from "@/app/_components/home-page";
import { RichGeneration } from "@/components/generation/rich-generation";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Settings } from "@/app/account/_components/account-page";
import { emptyPromptDraft } from "@/components/generation/prompt-document";

const transport = vi.hoisted(() => ({
  send: undefined as
    undefined | ((config: InternalAxiosRequestConfig) => Promise<any>),
}));
vi.mock("axios", async (original) => {
  const actual = await original<typeof import("axios")>();
  return {
    ...actual,
    default: {
      ...actual.default,
      create: (options: object) =>
        actual.default.create({
          ...options,
          adapter: (config) => transport.send!(config),
        }),
    },
  };
});
vi.mock("next/navigation", () => ({
  usePathname: () => "/",
  useRouter: () => ({ replace() {} }),
}));
vi.mock("next-themes", () => ({ useTheme: () => ({ setTheme() {} }) }));

// jsdom lacks IndexedDB. Only the native transaction boundary is substituted;
// Provider, generated clients, Axios, coordinator and branchJournal run unchanged.
let storageFault: "open" | "abort" | undefined;
let storageFailures = 0;
function identityMessages() {
  const channels = new Set<{ onmessage?: (event: unknown) => void }>();
  class Channel {
    onmessage?: (event: unknown) => void;
    constructor(name: string) {
      if (name === "glowloom-session-identity") channels.add(this);
    }
    postMessage() {}
    close() {
      channels.delete(this);
    }
  }
  vi.stubGlobal("BroadcastChannel", Channel);
  return (source = "other-tab") => {
    for (const channel of channels)
      channel.onmessage?.({ data: { type: "identity-changed", source } });
  };
}
function installIndexedDB() {
  const records = new Map<string, unknown>();
  let queue = Promise.resolve();
  const db = {
    close() {},
    transaction(_name: string, mode: string) {
      const tx: Record<string, any> = {};
      let pending = 0,
        ready = false,
        aborted = false;
      const abortOnCommit = storageFault === "abort" && mode === "readwrite";
      if (abortOnCommit) storageFailures++;
      let done!: () => void;
      const finished = new Promise<void>((r) => {
        done = r;
      });
      const start = queue;
      queue = finished;
      let staged: Map<string, unknown>;
      const finish = () => {
        if (!ready || pending) return;
        if (abortOnCommit) aborted = true;
        if (!aborted) {
          records.clear();
          for (const [k, v] of staged) records.set(k, v);
          tx.oncomplete?.();
        } else tx.onabort?.();
        done();
      };
      const op = (run: () => unknown) => {
        const req: Record<string, any> = {};
        pending++;
        void start.then(() =>
          queueMicrotask(() => {
            if (!aborted) {
              req.result = structuredClone(run());
              req.onsuccess?.();
            }
            pending--;
            finish();
          }),
        );
        return req;
      };
      void start.then(() => {
        staged = new Map(records);
        ready = true;
        queueMicrotask(finish);
      });
      const store = {
        transaction: tx,
        get: (key: string[]) => op(() => staged.get(JSON.stringify(key))),
        getAll: () => op(() => [...staged.values()]),
        put: (value: any) =>
          op(() => {
            staged.set(
              JSON.stringify([value.contextKey, value.replicaId]),
              structuredClone(value),
            );
          }),
        delete: (key: string[]) =>
          op(() => {
            staged.delete(JSON.stringify(key));
          }),
        clear: () =>
          op(() => {
            staged.clear();
          }),
      };
      tx.objectStore = () => store;
      tx.abort = () => {
        aborted = true;
      };
      return tx;
    },
  };
  vi.stubGlobal("indexedDB", {
    open: () => {
      const req: Record<string, any> = { result: db };
      queueMicrotask(() => {
        if (storageFault === "open") {
          storageFailures++;
          req.error = new Error("storage temporarily unavailable");
          req.onerror?.();
        } else req.onsuccess?.();
      });
      return req;
    },
  });
  return records;
}
const uid = "11111111-1111-4111-8111-111111111111";
const other = "22222222-2222-4222-8222-222222222222";
function identity(
  actor = uid,
  sid = "33333333-3333-4333-8333-333333333333",
): API.EmailSessionView {
  return {
    actor: {
      id: actor,
      email: "fixture@example.com",
      display_name: actor,
      email_verified_at: "2026-01-01T00:00:00Z",
      avatar_asset_id: "",
      platform_role: "creator",
      status: "active",
      revision: "1",
      capabilities: [],
    },
    session_id: sid,
    csrf_token: "synthetic-proof",
    credential_epoch: "1",
    persistent: false,
    last_active_at: new Date().toISOString(),
    absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
    idle_expires_at: new Date(Date.now() + 43200000).toISOString(),
    server_time: new Date().toISOString(),
    session_protocol: "v2",
    upgrade_available: false,
    refresh_generation: "1",
    access_expires_at: new Date(Date.now() + 900000).toISOString(),
    refresh_after_at: new Date(Date.now() + 840000).toISOString(),
  };
}
const context = `${uid}/project/draft`;
function branch(): LocalBranch {
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [
    { type: "text", text: "recover my unfinished input" },
  ];
  draft.params = { private_parameter: "must disappear" };
  draft.modelVersion = "private-model";
  return {
    contextKey: context,
    replicaId: "replica",
    localRecordRevision: 1,
    snapshot: { contextKey: context, replicaId: "replica", localSeq: 3, draft },
    serverRevision: 1,
    draftId: "draft",
    ackedSeq: 0,
    ownerState: { private: "cache" },
    pendingSave: {
      key: "old-save-command",
      expectedRevision: 1,
      snapshot: {
        contextKey: context,
        replicaId: "replica",
        localSeq: 2,
        draft,
      },
    },
    pendingSubmission: {
      key: "old-command",
      body: {} as API.QuoteItemRequest,
      confirmRequested: true,
    },
  };
}
let current: API.EmailSessionView | null;
let allowed: boolean;
let spaces: API.SpaceView[];
let records: Map<string, unknown>;
let calls: { url: string; method: string; csrf: unknown }[];
let refresh: ((config: InternalAxiosRequestConfig) => Promise<any>) | undefined;
function response(
  config: InternalAxiosRequestConfig,
  data: unknown,
  status = 200,
) {
  return { config, data, status, statusText: "fixture", headers: {} };
}
function reject(
  config: InternalAxiosRequestConfig,
  code: string,
  status: number,
): never {
  throw new AxiosError(
    code,
    undefined,
    config,
    undefined,
    response(config, { code }, status),
  );
}
function Private() {
  const value = useOptionalSession();
  return <p data-session={value?.session_id}>private {value?.actor.id}</p>;
}
function mount() {
  return render(
    <SessionProvider>
      <SessionGate>
        <Private />
      </SessionGate>
    </SessionProvider>,
  );
}
async function readyIdentity() {
  await screen.findByText(`private ${uid}`);
  await waitFor(() => expect(isCurrentSession(current!)).toBe(true));
}
async function login(actor = uid) {
  current = identity(actor, "44444444-4444-4444-8444-444444444444");
  document.cookie = "glowloom_csrf=synthetic-proof";
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
  });
  await waitFor(() =>
    expect(
      screen.getByText(`private ${actor}`).getAttribute("data-session"),
    ).toBe(current!.session_id),
  );
}
async function expire() {
  current = null;
  await act(async () => {
    await request("/api/private").catch(() => {});
  });
  await waitFor(() => expect(screen.queryByText(`private ${uid}`)).toBeNull());
  await waitFor(() => expect(getCSRFToken()).toBeNull());
  // Wait for the real storage transaction, not a mocked clear promise.
  await act(async () => {
    await branchJournal.list("no-owner/no-project/no-draft").catch(() => {});
  });
}
beforeEach(() => {
  storageFault = undefined;
  storageFailures = 0;
  records = installIndexedDB();
  current = identity();
  allowed = true;
  spaces = [
    {
      id: "personal-space",
      kind: "personal",
      name: "个人空间",
      role: "owner",
      team_id: "",
      writable: true,
    },
  ];
  calls = [];
  refresh = undefined;
  document.cookie = "glowloom_csrf=synthetic-proof";
  transport.send = async (config) => {
    const url = config.url!;
    const method = config.method!;
    calls.push({ url, method, csrf: config.headers.get("X-CSRF-Token") });
    if (url === "/api/session/refresh" && refresh) return refresh(config);
    if (method === "delete") return response(config, undefined, 204);
    if (url === "/api/me/preferences")
      return response(config, {
        theme: "system",
        prompt_suffix: "",
        revision: "1",
      });
    if (!current) return reject(config, "session_expired", 401);
    if (url === "/api/session") return response(config, current);
    if (url === "/api/me") return response(config, current.actor);
    if (url === "/api/me/spaces") return response(config, { items: spaces });
    if (url === "/api/me/sessions")
      return response(config, {
        items: [
          {
            id: current.session_id,
            current: true,
            device_summary: "this device",
            last_active_at: current.last_active_at,
          },
          {
            id: "55555555-5555-4555-8555-555555555555",
            current: false,
            device_summary: "other device",
            last_active_at: current.last_active_at,
          },
        ],
      });
    if (url === "/api/me/security-events")
      return response(config, { items: [] });
    if (url.includes("/prompt-drafts/")) {
      if (!allowed) return reject(config, "forbidden", 403);
      return response(config, {
        draft_id: "draft",
        revision: 1,
        draft_hash: "hash",
        input_hash: "input",
        draft: emptyPromptDraft(),
      });
    }
    return response(config, {});
  };
});
afterEach(async () => {
  storageFault = undefined;
  cleanup();
  setCreationScope(null);
  await branchJournal.clear().catch(() => {});
  vi.unstubAllGlobals();
});

it("logout sends one DELETE before pending refresh fails 503 and late work cannot revive identity", async () => {
  let fail!: () => void;
  refresh = (config) =>
    new Promise((_, rejectPromise) => {
      fail = () => {
        try {
          reject(config, "dependency_unavailable", 503);
        } catch (e) {
          rejectPromise(e);
        }
      };
    });
  mount();
  await readyIdentity();
  const pending = ensureSessionFresh().catch((error) => error);
  await waitFor(() =>
    expect(calls.some((c) => c.url === "/api/session/refresh")).toBe(true),
  );
  let logout: Promise<void>;
  await act(async () => {
    logout = revokeSession(current!).catch(() => {});
  });
  try {
    await waitFor(() =>
      expect(calls.filter((c) => c.method === "delete")).toHaveLength(1),
    );
    await act(async () => {
      await logout!;
    });
    expect(getCSRFToken()).toBeNull();
    expect(screen.queryByText(`private ${uid}`)).toBeNull();
    expect(calls.find((c) => c.method === "delete")?.csrf).toBe(
      "synthetic-proof",
    );
  } finally {
    await act(async () => {
      fail();
      await pending;
      await logout!;
    });
  }
  expect(screen.queryByText(`private ${uid}`)).toBeNull();
});

it("terminal Provider cleanup keeps only input, hidden until same account new sid and current draft permission", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  const stored = JSON.stringify([...records.values()]);
  expect(stored).toContain("recover my unfinished input");
  expect(stored).not.toMatch(
    /old-command|old-save-command|private_parameter|private-model|ownerState|pendingSubmission/,
  );
  expect(await branchJournal.list(context)).toEqual([]);
  await login();
  const restored = await branchJournal.list(context);
  expect(restored).toHaveLength(1);
  expect(restored[0].snapshot.draft.document.content[0].content).toEqual([
    { type: "text", text: "recover my unfinished input" },
  ]);
  expect(calls.filter((c) => !["get", "head"].includes(c.method))).toEqual([]);
});

it.each(["different account", "lost membership", "explicit logout"])(
  "does not restore expired input after %s",
  async (reason) => {
    mount();
    await readyIdentity();
    await branchJournal.write(branch());
    if (reason === "explicit logout")
      await act(async () => {
        await revokeSession(current!);
      });
    else await expire();
    if (reason === "lost membership") allowed = false;
    await login(reason === "different account" ? other : uid);
    expect(await branchJournal.list(context).catch(() => [])).toEqual([]);
    if (reason === "different account")
      expect(JSON.stringify([...records.values()])).not.toContain(
        "recover my unfinished input",
      );
    expect(
      calls.filter((c) => ["post", "put", "patch"].includes(c.method)),
    ).toEqual([]);
  },
);

it("startup terminal 401 preserves locked text until a fresh login", async () => {
  const view = mount();
  await readyIdentity();
  await branchJournal.write(branch());
  view.unmount();
  current = null;
  mount();
  await waitFor(() => expect(getCSRFToken()).toBeNull());
  await branchJournal.list(context);
  expect(await branchJournal.list(context)).toEqual([]);
  await login();
  expect(await branchJournal.list(context)).toHaveLength(1);
});

it("IndexedDB denied does not turn a successful server identity into a failed login", async () => {
  vi.stubGlobal("indexedDB", {
    open() {
      throw new DOMException("denied", "SecurityError");
    },
  });
  mount();
  await readyIdentity();
  expect(getCSRFToken()).toBe("synthetic-proof");
  expect(screen.queryByText("暂时无法确认登录状态")).toBeNull();
});

it("a cleanup in another connection rejects old writer leases and delayed passive cleanup cannot undo logout", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  vi.resetModules();
  const { branchJournal: remote } =
    await import("@/components/generation/draft-save");
  await remote.clear();
  const stale = branch();
  stale.localRecordRevision = 99;
  await expect(branchJournal.write(stale)).rejects.toThrow();
  await branchJournal.clear({ preserveInput: true });
  expect(JSON.stringify([...records.values()])).not.toContain(
    "recover my unfinished input",
  );
  expect(JSON.stringify([...records.values()])).not.toContain(
    "private_parameter",
  );
});

it("a delayed authorization read cannot reveal input after another connection clears it", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  await login();
  const send = transport.send!;
  let release!: () => void;
  transport.send = async (config) => {
    if (config.url?.includes("/prompt-drafts/"))
      await new Promise<void>((r) => {
        release = r;
      });
    return send(config);
  };
  const reading = branchJournal.list(context);
  await waitFor(() => expect(release).toBeTypeOf("function"));
  vi.resetModules();
  const { branchJournal: remote } =
    await import("@/components/generation/draft-save");
  await remote.clear();
  release();
  expect(await reading).toEqual([]);
});

it("a server receipt captured before another connection clears cannot reactivate its writer lease", async () => {
  mount();
  await readyIdentity();
  const checkpoint = await branchJournal.checkpoint();
  vi.resetModules();
  const { branchJournal: remote } =
    await import("@/components/generation/draft-save");
  await remote.clear();
  await expect(
    branchJournal.activateAccount(
      uid,
      current!.session_id,
      getSessionGeneration(),
      checkpoint,
    ),
  ).rejects.toThrow();
  await expect(branchJournal.write(branch())).rejects.toThrow();
});

it("an authorization response for a previous scope cannot reveal recovery input", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  await login();
  const send = transport.send!;
  let release!: () => void;
  transport.send = async (config) => {
    if (config.url?.includes("/prompt-drafts/"))
      await new Promise<void>((r) => {
        release = r;
      });
    return send(config);
  };
  const reading = branchJournal.list(context).catch(() => []);
  await waitFor(() => expect(release).toBeTypeOf("function"));
  setCreationScope("another-scope");
  release();
  expect(await reading).toEqual([]);
});

it("permission service failure keeps text locked and retry needs a fresh permission read", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  await login();
  const send = transport.send!;
  transport.send = async (config) => {
    if (config.url?.includes("/prompt-drafts/"))
      return reject(config, "dependency_unavailable", 503);
    return send(config);
  };
  await expect(branchJournal.list(context)).rejects.toMatchObject({
    status: 503,
  });
  expect(JSON.stringify([...records.values()])).toContain(
    "recover my unfinished input",
  );
  transport.send = send;
  expect(await branchJournal.list(context)).toHaveLength(1);
  expect(calls.filter((c) => !["get", "head"].includes(c.method))).toEqual([]);
});

it("explicit logout that finds the session already expired still destroys recoverable input", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  const session = current!;
  const send = transport.send!;
  transport.send = async (config) =>
    config.method === "delete"
      ? reject(config, "session_expired", 401)
      : send(config);
  await act(async () => {
    await revokeSession(session).catch(() => {});
  });
  await login();
  expect(await branchJournal.list(context)).toEqual([]);
});

it("foreground proof for the old sid cannot reopen a writer while logout DELETE is pending", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  const send = transport.send!;
  let finish!: () => void;
  transport.send = async (config) => {
    if (config.method === "delete")
      await new Promise<void>((r) => {
        finish = r;
      });
    return send(config);
  };
  const logout = revokeSession(current!);
  await waitFor(() => expect(finish).toBeTypeOf("function"));
  vi.resetModules();
  const { branchJournal: remote } =
    await import("@/components/generation/draft-save");
  const checkpoint = await remote.checkpoint();
  try {
    // Another tab obtains its server GET before the pending DELETE commits.
    await remote
      .activateAccount(uid, current!.session_id, 0, checkpoint)
      .catch(() => {});
    await expect(remote.write(branch())).rejects.toThrow();
  } finally {
    await act(async () => {
      finish();
      await logout;
    });
  }
  expect(JSON.stringify([...records.values()])).not.toContain(
    "recover my unfinished input",
  );
  await login();
  await branchJournal.write(branch());
  expect(await branchJournal.list(context)).toHaveLength(1);
});

it.each([true, false])(
  "device directory current=%s keeps the renewal bypass narrow",
  async (isCurrent) => {
    let fail!: () => void;
    refresh = (config) =>
      new Promise((_, rejectPromise) => {
        fail = () => {
          try {
            reject(config, "dependency_unavailable", 503);
          } catch (e) {
            rejectPromise(e);
          }
        };
      });
    render(
      <SessionProvider>
        <SessionGate>
          <Private />
          <Settings />
        </SessionGate>
      </SessionProvider>,
    );
    await readyIdentity();
    await branchJournal.write(branch());
    fireEvent.click(screen.getByRole("button", { name: "密码与会话" }));
    const buttons = await screen.findAllByRole("button", {
      name: "退出此设备",
    });
    const pending = ensureSessionFresh().catch((e) => e);
    await waitFor(() => expect(fail).toBeTypeOf("function"));
    fireEvent.click(buttons[isCurrent ? 0 : 1]);
    try {
      if (isCurrent) {
        await waitFor(() =>
          expect(calls.filter((c) => c.method === "delete")).toHaveLength(1),
        );
        await waitFor(() => expect(getCSRFToken()).toBeNull());
      } else {
        await act(async () => {});
        expect(calls.filter((c) => c.method === "delete")).toHaveLength(0);
        expect(screen.queryByText(`private ${uid}`)).toBeTruthy();
        expect(JSON.stringify([...records.values()])).toContain(
          "recover my unfinished input",
        );
      }
    } finally {
      await act(async () => {
        fail();
        await pending;
      });
    }
    if (!isCurrent)
      expect(calls.filter((c) => c.method === "delete")).toHaveLength(0);
  },
);

it("a stale logout callback cannot clear or revoke the newly confirmed session", async () => {
  mount();
  await readyIdentity();
  const stale = current!;
  await login();
  await branchJournal.write(branch());
  await act(async () => {
    await revokeSession(stale).catch(() => {});
  });
  expect(calls.filter((c) => c.method === "delete")).toEqual([]);
  expect(JSON.stringify([...records.values()])).toContain(
    "recover my unfinished input",
  );
  expect(screen.queryByText(`private ${uid}`)).toBeTruthy();
});

it("the actual home prompt restores text only after same-account new-sid and writable-space confirmation", async () => {
  render(
    <SessionProvider>
      <Private />
      <TooltipProvider>
        <SpaceProvider blockUntilReady={false}>
          <HomePage />
        </SpaceProvider>
      </TooltipProvider>
    </SessionProvider>,
  );
  const prompt = await screen.findByLabelText("创作提示词");
  fireEvent.change(prompt, { target: { value: "home unfinished idea" } });
  await act(async () => {});
  await expire();
  expect(screen.getByLabelText("创作提示词")).toHaveProperty("value", "");
  await login();
  const restore = await screen.findByRole("button", { name: "恢复上次文字" });
  expect(screen.getByLabelText("创作提示词")).toHaveProperty("value", "");
  fireEvent.click(restore);
  await waitFor(() =>
    expect(screen.getByLabelText("创作提示词")).toHaveProperty(
      "value",
      "home unfinished idea",
    ),
  );
  expect(calls.filter((c) => !["get", "head"].includes(c.method))).toEqual([]);
});

it("authorized scope cache clearing keeps the current identity able to save new local input", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await branchJournal.clear({ retainWriter: true });
  setCreationScope("next-space");
  const next = branch();
  next.contextKey = `${uid}/next-project/draft`;
  next.snapshot.contextKey = next.contextKey;
  await expect(branchJournal.write(next)).resolves.toBeUndefined();
  expect(await branchJournal.list(next.contextKey)).toHaveLength(1);
  expect(await branchJournal.list(context)).toEqual([]);
});

it("the actual home with retained input sends no activity GET when its Provider timer rotates credentials", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "Date"] });
  // Keep the server timestamp and refresh deadline on the same frozen clock.
  current = identity();
  current!.refresh_after_at = new Date(Date.now() + 60000).toISOString();
  refresh = async (config) => {
    current = {
      ...current!,
      refresh_generation: "2",
      server_time: new Date().toISOString(),
      access_expires_at: new Date(Date.now() + 900000).toISOString(),
      refresh_after_at: new Date(Date.now() + 840000).toISOString(),
    };
    return response(config, current);
  };
  try {
    render(
      <SessionProvider>
        <Private />
        <TooltipProvider>
          <SpaceProvider blockUntilReady={false}>
            <HomePage />
          </SpaceProvider>
        </TooltipProvider>
      </SessionProvider>,
    );
    await act(async () => {});
    fireEvent.change(screen.getByLabelText("创作提示词"), {
      target: { value: "keep while credentials rotate" },
    });
    await act(async () => {});
    expect(JSON.stringify([...records.values()])).toContain(
      "keep while credentials rotate",
    );
    const reads = calls.filter((call) => call.method === "get").length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60001);
    });
    expect(current!.refresh_generation).toBe("2");
    expect(
      calls.filter((call) => call.url === "/api/session/refresh"),
    ).toHaveLength(1);
    expect(calls.filter((call) => call.method === "get")).toHaveLength(reads);
    expect(screen.getByLabelText("创作提示词")).toHaveProperty(
      "value",
      "keep while credentials rotate",
    );
  } finally {
    cleanup();
    vi.useRealTimers();
  }
});

it.each(["different account", "different scope", "explicit logout"])(
  "actual home refuses prior text after %s",
  async (reason) => {
    render(
      <SessionProvider>
        <Private />
        <TooltipProvider>
          <SpaceProvider blockUntilReady={false}>
            <HomePage />
          </SpaceProvider>
        </TooltipProvider>
      </SessionProvider>,
    );
    fireEvent.change(await screen.findByLabelText("创作提示词"), {
      target: { value: "scoped home input" },
    });
    await waitFor(() =>
      expect(JSON.stringify([...records.values()])).toContain(
        "scoped home input",
      ),
    );
    if (reason === "explicit logout")
      await act(async () => {
        await revokeSession(current!);
      });
    else await expire();
    if (reason === "different scope")
      spaces = [{ ...spaces[0], id: "another-space" }];
    await login(reason === "different account" ? other : uid);
    await screen.findByLabelText("创作提示词");
    await act(async () => {});
    expect(screen.queryByRole("button", { name: "恢复上次文字" })).toBeNull();
    expect(screen.getByLabelText("创作提示词")).toHaveProperty("value", "");
    expect(
      calls.filter((c) => ["post", "put", "patch"].includes(c.method)),
    ).toEqual([]);
  },
);

it.each(["revoked", "read-only"])(
  "home restore rechecks scope permission when the button is used: %s",
  async (permission) => {
    render(
      <SessionProvider>
        <Private />
        <TooltipProvider>
          <SpaceProvider blockUntilReady={false}>
            <HomePage />
          </SpaceProvider>
        </TooltipProvider>
      </SessionProvider>,
    );
    fireEvent.change(await screen.findByLabelText("创作提示词"), {
      target: { value: "sensitive home input" },
    });
    await waitFor(() =>
      expect(JSON.stringify([...records.values()])).toContain(
        "sensitive home input",
      ),
    );
    await expire();
    await login();
    const restore = await screen.findByRole("button", { name: "恢复上次文字" });
    spaces =
      permission === "revoked" ? [] : [{ ...spaces[0], writable: false }];
    fireEvent.click(restore);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "恢复上次文字" })).toBeNull(),
    );
    expect(screen.getByLabelText("创作提示词")).toHaveProperty("value", "");
    expect(JSON.stringify([...records.values()])).not.toContain(
      "sensitive home input",
    );
    expect(calls.filter((c) => !["get", "head"].includes(c.method))).toEqual(
      [],
    );
  },
);

it.each(
  ["open", "abort"].flatMap((fault) =>
    [false, true].flatMap((logout) =>
      [false, true].map((reload) => ({ fault, logout, reload })),
    ),
  ),
)(
  "failed cleanup stays closed with existing private rows: $fault, logout=$logout, reload=$reload",
  async ({ fault, logout, reload }) => {
    const view = mount();
    await readyIdentity();
    await waitFor(() => expect(isCurrentSession(current!)).toBe(true));
    await branchJournal.write(branch());
    storageFault = fault as "open" | "abort";
    if (logout)
      await act(async () => {
        await revokeSession(current!);
      });
    else await expire();
    await waitFor(() => expect(storageFailures).toBeGreaterThan(0));
    expect(JSON.stringify([...records.values()])).toContain("old-command");
    storageFault = undefined;
    let journal = branchJournal;
    if (reload) {
      view.unmount();
      current = identity(uid, "44444444-4444-4444-8444-444444444444");
      vi.resetModules();
      journal = (await import("@/components/generation/draft-save"))
        .branchJournal;
      const authority = await import("@/lib/request");
      authority.setCSRFToken(current.csrf_token);
      await journal.activateAccount(
        uid,
        current.session_id,
        authority.getSessionGeneration(),
      );
    } else await login();
    expect(await journal.list(context)).toEqual([]);
    expect(JSON.stringify([...records.values()])).not.toMatch(
      /recover my unfinished input|old-command|old-save-command|private_parameter|private-model|ownerState/,
    );
    expect(
      calls.filter((c) => ["post", "put", "patch"].includes(c.method)),
    ).toEqual([]);
  },
);

async function openRichBranch(inputRecovery = true) {
  const view = mount();
  await readyIdentity();
  await branchJournal.write(branch());
  if (inputRecovery) {
    await expire();
    await login();
  }
  const send = transport.send!;
  transport.send = async (config) => {
    if (config.url?.endsWith("/prompt-submissions/current"))
      return response(config, { epoch: 0 });
    if (config.method === "post" && config.url?.endsWith("/prompt-drafts")) {
      calls.push({ url: config.url, method: "post", csrf: null });
      return response(config, {
        draft_id: "copy",
        revision: 1,
        draft_hash: "hash",
        input_hash: "input",
        draft: JSON.parse(config.data).draft,
      });
    }
    return send(config);
  };
  view.rerender(
    <SessionProvider>
      <Private />
      <TooltipProvider>
        <RichGeneration
          projectID="project"
          draftID="draft"
          seed={{ capability: "image", prompt: "" }}
        />
      </TooltipProvider>
    </SessionProvider>,
  );
  await screen.findByRole("button", { name: "恢复此分支" });
  return view;
}

it.each([403, 404])(
  "actual RichGeneration cannot copy expired input after the original draft becomes %s",
  async (status) => {
    await openRichBranch();
    const copy = await screen.findByRole("button", { name: "保留为副本" });
    const send = transport.send!;
    transport.send = async (config) =>
      config.url?.includes("/prompt-drafts/")
        ? reject(config, "forbidden", status)
        : send(config);
    fireEvent.click(copy);
    await act(async () => {});
    expect(calls.filter((c) => c.method === "post")).toEqual([]);
    expect(
      screen.getByRole("textbox", { name: "创作提示词" }).textContent,
    ).not.toContain("recover my unfinished input");
  },
);

it("actual RichGeneration keeps ordinary branch copy behavior", async () => {
  await openRichBranch(false);
  fireEvent.click(screen.getByRole("button", { name: "保留为副本" }));
  await waitFor(() =>
    expect(calls.filter((c) => c.method === "post")).toHaveLength(1),
  );
});

it("actual RichGeneration adopts the latest authorized plaintext branch only on restore", async () => {
  await openRichBranch();
  const [latest] = await branchJournal.list(context);
  latest.localRecordRevision++;
  latest.snapshot.localSeq++;
  latest.snapshot.draft.document.content[0].content = [
    { type: "text", text: "latest safely retained text" },
  ];
  await branchJournal.write(latest);
  expect(
    screen.getByRole("textbox", { name: "创作提示词" }).textContent,
  ).not.toContain("latest safely retained text");
  fireEvent.click(screen.getByRole("button", { name: "恢复此分支" }));
  await waitFor(() =>
    expect(
      screen.getByRole("textbox", { name: "创作提示词" }).textContent,
    ).toContain("latest safely retained text"),
  );
  expect(calls.filter((c) => c.method === "post")).toEqual([]);
});

it.each(["actor", "fence", "403", "404"])(
  "actual RichGeneration rechecks expired input at restore click: %s",
  async (changed) => {
    await openRichBranch();
    if (changed === "actor") current = identity(other);
    if (changed === "fence") {
      vi.resetModules();
      const remote = (await import("@/components/generation/draft-save"))
        .branchJournal;
      // A different tab need not deliver its advisory broadcast before this click.
      const channel = globalThis.BroadcastChannel;
      vi.stubGlobal("BroadcastChannel", undefined);
      const dispatch = vi.spyOn(window, "dispatchEvent").mockReturnValue(true);
      try {
        await remote.clear();
      } finally {
        dispatch.mockRestore();
        vi.stubGlobal("BroadcastChannel", channel);
      }
    }
    if (changed === "403" || changed === "404") {
      const send = transport.send!;
      transport.send = async (config) =>
        config.url?.includes("/prompt-drafts/")
          ? reject(config, "forbidden", Number(changed))
          : send(config);
    }
    fireEvent.click(screen.getByRole("button", { name: "恢复此分支" }));
    await act(async () => {});
    expect(
      screen.getByRole("textbox", { name: "创作提示词" }).textContent,
    ).not.toContain("recover my unfinished input");
    expect(
      calls.filter((c) => ["post", "put", "patch"].includes(c.method)),
    ).toEqual([]);
  },
);

it("a new journal module discards passive text without a local cleanup witness", async () => {
  const view = mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  view.unmount();
  current = identity(uid, "44444444-4444-4444-8444-444444444444");
  vi.resetModules();
  const remote = (await import("@/components/generation/draft-save"))
    .branchJournal;
  const authority = await import("@/lib/request");
  authority.setCSRFToken(current.csrf_token);
  await remote.activateAccount(
    uid,
    current.session_id,
    authority.getSessionGeneration(),
  );
  const found = await remote.list(context);
  expect(found).toEqual([]);
  expect(JSON.stringify([...records.values()])).not.toContain(
    "recover my unfinished input",
  );
});

it("unknown startup cannot turn an old active snapshot into witnessed passive recovery", async () => {
  const view = mount();
  await readyIdentity();
  await branchJournal.write(branch());
  view.unmount();
  vi.resetModules();
  const remote = (await import("@/components/generation/draft-save"))
    .branchJournal;
  await remote.clear({ preserveInput: true });
  current = identity(uid, "44444444-4444-4444-8444-444444444444");
  const authority = await import("@/lib/request");
  authority.setCSRFToken(current.csrf_token);
  await remote.activateAccount(
    uid,
    current.session_id,
    authority.getSessionGeneration(),
  );
  expect(await remote.list(context)).toEqual([]);
  expect(JSON.stringify([...records.values()])).not.toContain("old-command");
});

it("failed strong cleanup cannot become plaintext recovery on a later passive cleanup", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  storageFault = "abort";
  await expect(
    branchJournal.clear({ logoutSession: current!.session_id }),
  ).rejects.toThrow();
  storageFault = undefined;
  await branchJournal.clear({ preserveInput: true });
  await login();
  expect(await branchJournal.list(context)).toEqual([]);
  expect(JSON.stringify([...records.values()])).not.toContain(
    "recover my unfinished input",
  );
});

it("server relogin succeeds while old private storage remains unavailable and later activation discards it", async () => {
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  storageFault = "open";
  await expire();
  await login();
  expect(screen.getByText(`private ${uid}`)).toBeTruthy();
  expect(await branchJournal.list(context)).toEqual([]);
  expect(JSON.stringify([...records.values()])).toContain("old-command");
  storageFault = undefined;
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
  });
  await waitFor(() =>
    expect(JSON.stringify([...records.values()])).not.toContain("old-command"),
  );
  expect(await branchJournal.list(context)).toEqual([]);
});

it("a new module cannot reuse an old passive marker after login and explicit logout both occurred with storage unavailable", async () => {
  const view = mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  storageFault = "open";
  await login();
  await waitFor(() => expect(isCurrentSession(current!)).toBe(true));
  await act(async () => {
    await revokeSession(current!);
  });
  view.unmount();
  storageFault = undefined;
  current = identity(uid, "66666666-6666-4666-8666-666666666666");
  vi.resetModules();
  const remote = (await import("@/components/generation/draft-save"))
    .branchJournal;
  const authority = await import("@/lib/request");
  authority.setCSRFToken(current.csrf_token);
  await remote.activateAccount(
    uid,
    current.session_id,
    authority.getSessionGeneration(),
  );
  expect(await remote.list(context)).toEqual([]);
  expect(JSON.stringify([...records.values()])).not.toContain(
    "recover my unfinished input",
  );
});

it.each(["missing", "legacy"])(
  "activation drops unknown full rows when durable eligibility metadata is %s",
  async (metadata) => {
    const view = mount();
    await readyIdentity();
    await branchJournal.write(branch());
    view.unmount();
    const key = JSON.stringify(["", ""]);
    if (metadata === "missing") records.delete(key);
    else {
      const old = records.get(key) as Record<string, unknown>;
      delete old.mode;
      delete old.activeSession;
    }
    vi.resetModules();
    const remote = (await import("@/components/generation/draft-save"))
      .branchJournal;
    const authority = await import("@/lib/request");
    authority.setCSRFToken(current!.csrf_token);
    await remote.activateAccount(
      uid,
      current!.session_id,
      authority.getSessionGeneration(),
    );
    expect(await remote.list(context)).toEqual([]);
    expect(JSON.stringify([...records.values()])).not.toContain("old-command");
  },
);

it("observed external login and logout revoke a passive witness even when identity activation cannot access storage", async () => {
  const emit = identityMessages();
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  storageFault = "open";
  current = identity(uid, "55555555-5555-4555-8555-555555555555");
  await act(async () => {
    emit();
  });
  await waitFor(() =>
    expect(
      screen.getByText(`private ${uid}`).getAttribute("data-session"),
    ).toBe(current!.session_id),
  );
  expect(storageFailures).toBeGreaterThan(0);
  current = null;
  await act(async () => {
    emit();
  });
  await waitFor(() => expect(screen.queryByText(`private ${uid}`)).toBeNull());
  expect(JSON.stringify([...records.values()])).toContain(
    "recover my unfinished input",
  );
  storageFault = undefined;
  await login();
  expect(await branchJournal.list(context)).toEqual([]);
  expect(JSON.stringify([...records.values()])).not.toContain(
    "recover my unfinished input",
  );
});

it("a self-originated identity notification does not revoke normal passive text recovery", async () => {
  const emit = identityMessages();
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  await expire();
  const before = calls.length;
  await act(async () => {
    emit(sessionIdentitySource);
  });
  expect(calls).toHaveLength(before);
  await login();
  expect(await branchJournal.list(context)).toHaveLength(1);
});

it("an observed external identity message synchronously fences the writer before its foreground read finishes", async () => {
  const emit = identityMessages();
  mount();
  await readyIdentity();
  await branchJournal.write(branch());
  const send = transport.send!;
  let release!: () => void;
  transport.send = async (config) => {
    if (config.url === "/api/session")
      await new Promise<void>((resolve) => {
        release = resolve;
      });
    return send(config);
  };
  let write: Promise<unknown>;
  await act(async () => {
    emit();
    const stale = branch();
    stale.localRecordRevision++;
    write = branchJournal.write(stale).then(
      () => "written",
      () => "fenced",
    );
  });
  await waitFor(() => expect(release).toBeTypeOf("function"));
  try {
    expect(await write!).toBe("fenced");
  } finally {
    await act(async () => {
      release();
    });
  }
});

it("an observed external identity message fences an activation already awaiting server proof", async () => {
  const emit = identityMessages();
  mount();
  await readyIdentity();
  const send = transport.send!;
  const releases: (() => void)[] = [];
  transport.send = async (config) => {
    if (config.url === "/api/session")
      await new Promise<void>((resolve) => {
        releases.push(resolve);
      });
    return send(config);
  };
  const old = branchJournal
    .activateAccount(uid, current!.session_id, getSessionGeneration())
    .then(
      () => "activated",
      () => "fenced",
    );
  await waitFor(() => expect(releases).toHaveLength(1));
  await act(async () => {
    emit();
  });
  await waitFor(() => expect(releases).toHaveLength(2));
  try {
    await act(async () => {
      releases[0]();
    });
    expect(await old).toBe("fenced");
  } finally {
    await act(async () => {
      releases[1]();
    });
  }
});
