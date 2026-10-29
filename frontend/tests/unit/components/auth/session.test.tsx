import { requireSession } from "@/components/auth/contracts";
import { isCurrentSession } from "@/components/auth/session-renewal";
import { branchJournal } from "@/components/generation/draft-save";
import {
  cleanup,
  act,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { StrictMode } from "react";
import { ThemeProvider, useTheme } from "next-themes";
import { afterEach, expect, it, vi } from "vitest";
import {
  SessionGate,
  SessionProvider,
  revokeSession,
  useOptionalSession,
} from "@/components/auth/session";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  usePathname: () => navigation.pathname,
  useRouter: () => router,
}));
const router = { replace: vi.fn() };
const navigation = { pathname: "/" };
const notifications = vi.hoisted(() => ({ show: vi.fn(), dismiss: vi.fn() }));

it("guest canvas redirect retains the full canvas location for login recovery", async () => {
  const destination =
    "/canvas?project_id=project&canvas_id=canvas&scope_id=scope";
  window.history.replaceState(null, "", destination);
  vi.mocked(request).mockRejectedValue(
    new RequestError("unauthenticated", 401),
  );
  try {
    render(
      <SessionProvider>
        <SessionGate>
          <p>private canvas</p>
        </SessionGate>
      </SessionProvider>,
    );
    await waitFor(() =>
      expect(router.replace).toHaveBeenCalledWith(
        `/login?returnTo=${encodeURIComponent(destination)}`,
      ),
    );
  } finally {
    window.history.replaceState(null, "", "/");
  }
});

it("a startup 503 keeps the canvas location and never opens login", async () => {
  window.history.replaceState(
    null,
    "",
    "/canvas?canvas_id=canvas&project_id=project",
  );
  vi.mocked(request).mockRejectedValue(
    new RequestError("dependency_unavailable", 503),
  );
  try {
    render(
      <SessionProvider>
        <SessionGate>
          <p>private canvas</p>
        </SessionGate>
      </SessionProvider>,
    );
    await screen.findByText("暂时无法确认登录状态");
    expect(router.replace).not.toHaveBeenCalled();
    expect(window.location.pathname).toBe("/canvas");
  } finally {
    window.history.replaceState(null, "", "/");
  }
});
vi.mock("sonner", () => ({
  toast: Object.assign(notifications.show, { dismiss: notifications.dismiss }),
}));
afterEach(() => {
  navigation.pathname = "/";
  cleanup();
  vi.resetAllMocks();
});
it("startup announces one pending state without rendering private children", () => {
  vi.mocked(request).mockReturnValue(new Promise(() => {}));
  const { container } = render(
    <SessionProvider>
      <SessionGate>
        <p>private content</p>
      </SessionGate>
    </SessionProvider>,
  );
  expect(screen.getByRole("status").textContent).toBe("正在验证登录状态…");
  expect(screen.getAllByRole("status")).toHaveLength(1);
  expect(screen.queryByText("private content")).toBeNull();
  expect(container.querySelector('[aria-busy="true"]')).toBeTruthy();
  expect(screen.queryByText("正在准备创作空间")).toBeNull();
  expect(screen.queryByRole("progressbar")).toBeNull();
});

it("protected failure stops loading and retry returns to the pending boundary", async () => {
  vi.mocked(request)
    .mockRejectedValueOnce(new RequestError("temporarily_unavailable", 503))
    .mockReturnValueOnce(new Promise(() => {}));
  render(
    <SessionProvider>
      <SessionGate>
        <p>private content</p>
      </SessionGate>
    </SessionProvider>,
  );
  await screen.findByRole("heading", { name: "暂时无法确认登录状态" });
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByText("private content")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(await screen.findByRole("status")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});
it("public session failure uses one short toast and keeps the page free of a banner", async () => {
  vi.mocked(request)
    .mockRejectedValueOnce(new RequestError("invalid_receipt"))
    .mockRejectedValueOnce(new RequestError("unauthenticated", 401));
  render(
    <SessionProvider>
      <SessionGate allowGuest>
        <p>public content</p>
      </SessionGate>
    </SessionProvider>,
  );
  await screen.findByText("public content");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.queryByLabelText("会话状态")).toBeNull();
  await waitFor(() => expect(notifications.show).toHaveBeenCalledTimes(1));
  expect(notifications.show).toHaveBeenCalledWith(
    "暂时无法确认登录状态",
    expect.objectContaining({
      action: expect.objectContaining({ label: "重试" }),
    }),
  );
  const [title, options] = notifications.show.mock.lastCall!;
  expect(title).not.toMatch(/回执|服务端|结果未确认/);
  act(() => options.action.onClick());
  await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
  expect(notifications.dismiss).toHaveBeenCalledWith(options.id);
  expect(router.replace).not.toHaveBeenCalled();
});

it("normal anonymous browsing does not show any session notification", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("unauthenticated", 401),
  );
  render(
    <SessionProvider>
      <SessionGate allowGuest>
        <p>public content</p>
      </SessionGate>
    </SessionProvider>,
  );
  await screen.findByText("public content");
  expect(notifications.show).not.toHaveBeenCalled();
  expect(screen.queryByRole("alert")).toBeNull();
});

it("unmounting a public page ignores a late failure", async () => {
  let reject!: (error: unknown) => void;
  vi.mocked(request).mockReturnValue(
    new Promise((_, fail) => {
      reject = fail;
    }),
  );
  const view = render(
    <SessionProvider>
      <SessionGate allowGuest>
        <p>public content</p>
      </SessionGate>
    </SessionProvider>,
  );
  view.unmount();
  await act(async () => reject(new RequestError("invalid_receipt")));
  expect(notifications.show).not.toHaveBeenCalled();
});

it("StrictMode emits only one toast for a current failed session check", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("temporarily_unavailable", 503),
  );
  const view = render(
    <StrictMode>
      <SessionProvider>
        <SessionGate allowGuest>
          <p>public content</p>
        </SessionGate>
      </SessionProvider>
    </StrictMode>,
  );
  await screen.findByText("public content");
  expect(notifications.show).toHaveBeenCalledTimes(1);
  const options = notifications.show.mock.lastCall![1];
  view.unmount();
  expect(notifications.dismiss).toHaveBeenCalledWith(options.id);
});

const validSession = {
  actor: {
    id: "11111111-1111-4111-8111-111111111111",
    email: "creator@example.com",
    display_name: "Creator",
    platform_role: "creator",
    status: "active",
    revision: "1",
    capabilities: [],
  },
  session_id: "22222222-2222-4222-8222-222222222222",
  csrf_token: "proof",
  credential_epoch: "1",
  must_change_password: false,
  persistent: false,
  last_active_at: new Date().toISOString(),
  absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
};
function LocalThemeChange() {
  const { setTheme } = useTheme();
  return <button onClick={() => setTheme("dark")}>切换测试主题</button>;
}
function SavedThemeState() {
  const { theme } = useTheme();
  return <p>主题偏好：{theme}</p>;
}
it.each(["light", "dark", "system"])(
  "默认深色仍恢复已保存的 %s 偏好",
  async (theme) => {
    localStorage.clear();
    vi.stubGlobal("matchMedia", (media: string) => ({
      matches: false,
      media,
      addListener() {},
      removeListener() {},
    }));
    vi.mocked(request).mockImplementation(async (url) =>
      url === "/api/session"
        ? validSession
        : { theme, prompt_suffix: "", revision: "2" },
    );
    const view = render(
      <ThemeProvider attribute="class" defaultTheme="dark" enableSystem>
        <SessionProvider>
          <SessionGate>
            <SavedThemeState />
          </SessionGate>
        </SessionProvider>
      </ThemeProvider>,
    );
    await screen.findByText(`主题偏好：${theme}`);
    await waitFor(() =>
      expect(
        document.documentElement.classList.contains(
          theme === "dark" ? "dark" : "light",
        ),
      ).toBe(true),
    );
    view.unmount();
    vi.unstubAllGlobals();
    localStorage.clear();
    document.documentElement.removeAttribute("class");
    document.documentElement.removeAttribute("style");
  },
);
it("账号偏好读取失败时保持默认深色，不被浅色系统外观覆盖", async () => {
  localStorage.clear();
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: false,
    media,
    addListener() {},
    removeListener() {},
  }));
  let rejectPreferences!: (error: Error) => void;
  const pendingPreferences = new Promise((_, reject) => {
    rejectPreferences = reject;
  });
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/session") return validSession;
    return pendingPreferences;
  });
  const view = render(
    <ThemeProvider attribute="class" defaultTheme="dark" enableSystem>
      <SessionProvider>
        <SessionGate>
          <p>深色默认内容</p>
        </SessionGate>
      </SessionProvider>
    </ThemeProvider>,
  );
  await screen.findByText("深色默认内容");
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/me/preferences",
      expect.objectContaining({ method: "GET" }),
    ),
  );
  await act(async () =>
    rejectPreferences(new Error("preferences unavailable")),
  );
  await waitFor(() =>
    expect(document.documentElement.classList.contains("dark")).toBe(true),
  );
  view.unmount();
  vi.unstubAllGlobals();
  localStorage.clear();
  document.documentElement.removeAttribute("class");
  document.documentElement.removeAttribute("style");
});
it("主题 setter 随选择变化时不重新读取并覆盖账号默认主题", async () => {
  localStorage.clear();
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: false,
    media,
    addListener() {},
    removeListener() {},
  }));
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session"
      ? validSession
      : { theme: "system", prompt_suffix: "", revision: "1" },
  );
  const view = render(
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
      <SessionProvider>
        <SessionGate>
          <LocalThemeChange />
        </SessionGate>
      </SessionProvider>
    </ThemeProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "切换测试主题" }));
  await waitFor(() =>
    expect(document.documentElement.classList.contains("dark")).toBe(true),
  );
  expect(
    vi
      .mocked(request)
      .mock.calls.filter(([url]) => url === "/api/me/preferences"),
  ).toHaveLength(1);
  view.unmount();
  vi.unstubAllGlobals();
  localStorage.clear();
  document.documentElement.removeAttribute("class");
  document.documentElement.removeAttribute("style");
});
it.each(["authenticated", "guest"])(
  "toast retry keeps the page mounted while checking, then reaches %s",
  async (terminal) => {
    let finish!: (value: unknown) => void;
    let fail!: (error: unknown) => void;
    const pending = new Promise((resolve, reject) => {
      finish = resolve;
      fail = reject;
    });
    vi.mocked(request)
      .mockRejectedValueOnce(new RequestError("temporarily_unavailable", 503))
      .mockReturnValueOnce(pending)
      .mockResolvedValue(validSession);
    render(
      <SessionProvider>
        <SessionGate allowGuest>
          <input aria-label="public draft" defaultValue="existing idea" />
        </SessionGate>
      </SessionProvider>,
    );
    const draft = await screen.findByLabelText("public draft");
    fireEvent.change(draft, { target: { value: "keep this idea" } });
    await waitFor(() => expect(notifications.show).toHaveBeenCalled());
    const options = notifications.show.mock.lastCall![1];
    act(() => options.action.onClick());
    expect(screen.getByLabelText("public draft")).toBe(draft);
    expect(draft).toHaveProperty("value", "keep this idea");
    await act(async () => {
      if (terminal === "authenticated") finish(validSession);
      else fail(new RequestError("unauthenticated", 401));
    });
    await waitFor(() =>
      expect(notifications.dismiss).toHaveBeenCalledWith(options.id),
    );
    expect(screen.queryByLabelText("会话状态")).toBeNull();
    expect(screen.getByLabelText("public draft")).toBeTruthy();
    if (terminal === "guest")
      expect(screen.getByLabelText("public draft")).toBe(draft);
  },
);

it("旧服务端改密标记为true也正常渲染已登录内容，不强制跳转", async () => {
  vi.mocked(request).mockResolvedValue({
    ...validSession,
    must_change_password: true,
  });
  render(
    <SessionProvider>
      <SessionGate>
        <p>正常账号页面</p>
      </SessionGate>
    </SessionProvider>,
  );
  expect(await screen.findByText("正常账号页面")).toBeTruthy();
  expect(router.replace).not.toHaveBeenCalledWith("/reset-password");
});

it("persistent session survives page gate remount without another startup or theme reset", async () => {
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session" ? validSession : { theme: "light" },
  );
  const view = render(
    <SessionProvider>
      <SessionGate key="home">
        <p>first page</p>
      </SessionGate>
    </SessionProvider>,
  );
  await screen.findByText("first page");
  view.rerender(
    <SessionProvider>
      <SessionGate key="explore">
        <p>next page</p>
      </SessionGate>
    </SessionProvider>,
  );
  expect(screen.getByText("next page")).toBeTruthy();
  expect(screen.queryByRole("status")).toBeNull();
  expect(
    vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
  ).toHaveLength(1);
  expect(
    vi
      .mocked(request)
      .mock.calls.filter(([url]) => url === "/api/me/preferences"),
  ).toHaveLength(1);
});

it("background route check keeps confirmed content until a current 401 revokes it", async () => {
  let reject!: (error: unknown) => void;
  const pending = new Promise((_, fail) => {
    reject = fail;
  });
  vi.mocked(request).mockImplementation(async (url) => {
    if (url !== "/api/session") return { theme: "light" };
    return navigation.pathname === "/" ? validSession : pending;
  });
  const view = render(
    <SessionProvider>
      <SessionGate>
        <p>first private page</p>
      </SessionGate>
    </SessionProvider>,
  );
  await screen.findByText("first private page");
  navigation.pathname = "/subjects";
  view.rerender(
    <SessionProvider>
      <SessionGate key="subjects">
        <p>next private page</p>
      </SessionGate>
    </SessionProvider>,
  );
  expect(screen.getByText("next private page")).toBeTruthy();
  expect(screen.queryByRole("status")).toBeNull();
  await act(async () => reject(new RequestError("unauthenticated", 401)));
  await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));
  expect(screen.queryByText("next private page")).toBeNull();
  expect(
    vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
  ).toHaveLength(2);
  expect(
    vi
      .mocked(request)
      .mock.calls.filter(([url]) => url === "/api/me/preferences"),
  ).toHaveLength(1);
});

it("a saved local light theme remains visible while account preferences are pending", async () => {
  localStorage.setItem("theme", "light");
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: false,
    media,
    addListener() {},
    removeListener() {},
  }));
  let resolvePreferences!: (value: unknown) => void;
  const pending = new Promise((resolve) => {
    resolvePreferences = resolve;
  });
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session" ? validSession : pending,
  );
  const view = render(
    <ThemeProvider attribute="class" defaultTheme="dark">
      <SessionProvider>
        <SessionGate>
          <SavedThemeState />
        </SessionGate>
      </SessionProvider>
    </ThemeProvider>,
  );
  await screen.findByText("主题偏好：light");
  expect(document.documentElement.classList.contains("light")).toBe(true);
  await act(async () => resolvePreferences({ theme: "light" }));
  expect(screen.getByText("主题偏好：light")).toBeTruthy();
  view.unmount();
  vi.unstubAllGlobals();
  localStorage.clear();
  document.documentElement.removeAttribute("class");
  document.documentElement.removeAttribute("style");
});

function LogoutWithoutNavigation() {
  const session = useOptionalSession();
  return session ? (
    <button onClick={() => void revokeSession(session)}>
      退出当前测试账号
    </button>
  ) : (
    <p>会话已清理</p>
  );
}
it("explicit logout clears the persistent identity before another route check", async () => {
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session" ? validSession : {},
  );
  render(
    <SessionProvider>
      <SessionGate allowGuest>
        <LogoutWithoutNavigation />
      </SessionGate>
    </SessionProvider>,
  );
  const logout = await screen.findByRole("button", {
    name: "退出当前测试账号",
  });
  await waitFor(() =>
    expect(isCurrentSession(requireSession(validSession))).toBe(true),
  );
  fireEvent.click(logout);
  await screen.findByText("会话已清理");
  expect(screen.queryByRole("button", { name: "退出当前测试账号" })).toBeNull();
  expect(
    vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
  ).toHaveLength(1);
});

function SessionIdentity() {
  const session = useOptionalSession();
  return <p>{session?.actor.display_name ?? "guest identity"}</p>;
}
it("a different identity returned by a route check waits for private draft cleanup", async () => {
  const next = {
    ...validSession,
    csrf_token: "next-proof",
    actor: {
      ...validSession.actor,
      id: "33333333-3333-4333-8333-333333333333",
      display_name: "Next creator",
    },
  };
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session"
      ? navigation.pathname === "/"
        ? validSession
        : next
      : { theme: "light" },
  );
  const view = render(
    <SessionProvider>
      <SessionGate allowGuest>
        <SessionIdentity />
      </SessionGate>
    </SessionProvider>,
  );
  await screen.findByText("Creator");
  let finish!: () => void;
  const pending = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const clear = vi.spyOn(branchJournal, "clear").mockReturnValue(pending);
  try {
    navigation.pathname = "/explore";
    view.rerender(
      <SessionProvider>
        <SessionGate key="next" allowGuest>
          <SessionIdentity />
        </SessionGate>
      </SessionProvider>,
    );
    await waitFor(() => expect(clear).toHaveBeenCalledOnce());
    expect(screen.queryByText("Next creator")).toBeNull();
    expect(screen.queryByText("Creator")).toBeNull();
  } finally {
    await act(async () => finish());
    clear.mockRestore();
  }
  await screen.findByText("Next creator");
});

it("background preemptive renewal never sends foreground activity GET and hides stop timers", async () => {
  vi.useFakeTimers();
  document.cookie = "glowloom_csrf=proof; path=/";
  const now = Date.now();
  const renewed = {
    ...validSession,
    session_protocol: "v2",
    refresh_generation: "0",
    upgrade_available: false,
    server_time: new Date(now).toISOString(),
    idle_expires_at: new Date(now + 43200000).toISOString(),
    access_expires_at: new Date(now + 120000).toISOString(),
    refresh_after_at: new Date(now + 60000).toISOString(),
  };
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session" || url === "/api/session/refresh"
      ? renewed
      : { theme: "light" },
  );
  const view = render(
    <SessionProvider>
      <SessionGate>
        <input aria-label="private draft" defaultValue="keep" />
      </SessionGate>
    </SessionProvider>,
  );
  try {
    await act(async () => {});
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60001);
    });
    expect(
      vi
        .mocked(request)
        .mock.calls.filter(([url]) => url === "/api/session/refresh"),
    ).toHaveLength(1);
    expect(
      vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
    ).toHaveLength(1);
    expect(screen.getByLabelText("private draft")).toHaveProperty(
      "value",
      "keep",
    );
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(120000);
    });
    expect(
      vi
        .mocked(request)
        .mock.calls.filter(([url]) => url === "/api/session/refresh"),
    ).toHaveLength(1);
  } finally {
    view.unmount();
    document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
    vi.useRealTimers();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
  }
});

it("offline then online foreground restoration preserves the mounted draft", async () => {
  let fail = false;
  vi.mocked(request).mockImplementation(async (url) => {
    if (url !== "/api/session") return { theme: "light" };
    if (fail) throw new RequestError("request_unconfirmed");
    return validSession;
  });
  render(
    <SessionProvider>
      <SessionGate>
        <input aria-label="offline draft" defaultValue="keep" />
      </SessionGate>
    </SessionProvider>,
  );
  const draft = await screen.findByLabelText("offline draft");
  fail = true;
  await act(async () => window.dispatchEvent(new Event("online")));
  expect(screen.getByLabelText("offline draft")).toBe(draft);
  expect(draft).toHaveProperty("value", "keep");
  fail = false;
  await act(async () => window.dispatchEvent(new Event("online")));
  expect(screen.getByLabelText("offline draft")).toBe(draft);
});

it("expired startup refresh is followed by one actual foreground GET", async () => {
  document.cookie = "glowloom_csrf=proof; path=/";
  let reads = 0;
  const now = Date.now();
  const active = {
    ...validSession,
    session_protocol: "v2",
    refresh_generation: "0",
    upgrade_available: false,
    server_time: new Date(now).toISOString(),
    idle_expires_at: new Date(now + 43200000).toISOString(),
    access_expires_at: new Date(now + 900000).toISOString(),
    refresh_after_at: new Date(now + 840000).toISOString(),
  };
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/session" && ++reads === 1)
      throw new RequestError("access_expired", 401);
    return url === "/api/session" || url === "/api/session/refresh"
      ? active
      : { theme: "light" };
  });
  try {
    render(
      <SessionProvider>
        <SessionGate>
          <p>restored private content</p>
        </SessionGate>
      </SessionProvider>,
    );
    await screen.findByText("restored private content");
    await waitFor(() =>
      expect(
        vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
      ).toHaveLength(2),
    );
    expect(
      vi
        .mocked(request)
        .mock.calls.filter(([url]) => url === "/api/session/refresh"),
    ).toHaveLength(1);
  } finally {
    document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
  }
});
it("missing readable proof at the visible renewal boundary stops without blind refresh and clears identity", async () => {
  vi.useFakeTimers();
  const now = Date.now();
  const active = {
    ...validSession,
    session_protocol: "v2",
    refresh_generation: "0",
    upgrade_available: false,
    server_time: new Date(now).toISOString(),
    idle_expires_at: new Date(now + 43200000).toISOString(),
    access_expires_at: new Date(now + 120000).toISOString(),
    refresh_after_at: new Date(now + 60000).toISOString(),
  };
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session" ? active : { theme: "light" },
  );
  const view = render(
    <SessionProvider>
      <SessionGate allowGuest>
        <SessionIdentity />
      </SessionGate>
    </SessionProvider>,
  );
  try {
    await act(async () => {});
    expect(screen.getByText("Creator")).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60001);
    });
    expect(screen.getByText("guest identity")).toBeTruthy();
    expect(
      vi
        .mocked(request)
        .mock.calls.filter(([url]) => url === "/api/session/refresh"),
    ).toHaveLength(0);
  } finally {
    view.unmount();
    vi.useRealTimers();
  }
});

it("idle boundary checks authoritative refresh without clipping access or locally expiring another tab's active session", async () => {
  vi.useFakeTimers();
  document.cookie = "glowloom_csrf=proof; path=/";
  const now = Date.now();
  const active = {
    ...validSession,
    session_protocol: "v2",
    refresh_generation: "0",
    upgrade_available: false,
    server_time: new Date(now).toISOString(),
    idle_expires_at: new Date(now + 30000).toISOString(),
    access_expires_at: new Date(now + 900000).toISOString(),
    refresh_after_at: new Date(now + 840000).toISOString(),
  };
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session"
      ? active
      : url === "/api/session/refresh"
        ? {
            ...active,
            server_time: new Date(now + 30000).toISOString(),
            idle_expires_at: new Date(now + 43200000).toISOString(),
          }
        : { theme: "light" },
  );
  const view = render(
    <SessionProvider>
      <SessionGate>
        <p>other tab kept session active</p>
      </SessionGate>
    </SessionProvider>,
  );
  try {
    await act(async () => {});
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30001);
    });
    expect(screen.getByText("other tab kept session active")).toBeTruthy();
    expect(
      vi
        .mocked(request)
        .mock.calls.filter(([url]) => url === "/api/session/refresh"),
    ).toHaveLength(1);
    expect(
      vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
    ).toHaveLength(1);
  } finally {
    view.unmount();
    document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
    vi.useRealTimers();
  }
});

it("two guest tabs converge on remote identity loss without broadcast echoes or repeated cleanup", async () => {
  const { setCSRFToken } = await import("@/lib/request");
  setCSRFToken(null);
  const peers = new Set<{
    onmessage: ((event: { data: unknown }) => void) | null;
  }>();
  let posts = 0;
  class Channel {
    onmessage: ((event: { data: unknown }) => void) | null = null;
    constructor() {
      peers.add(this);
    }
    postMessage() {
      posts++;
      if (posts < 8)
        queueMicrotask(() => {
          for (const peer of peers)
            peer.onmessage?.({
              data: { type: "identity-changed", source: "other-tab" },
            });
        });
    }
    close() {
      peers.delete(this);
    }
  }
  vi.stubGlobal("BroadcastChannel", Channel);
  vi.mocked(request).mockRejectedValue(
    new RequestError("unauthenticated", 401),
  );
  const clear = vi.spyOn(branchJournal, "clear").mockResolvedValue(undefined);
  try {
    render(
      <>
        <SessionProvider>
          <SessionGate allowGuest>
            <p>tab A guest</p>
          </SessionGate>
        </SessionProvider>
        <SessionProvider>
          <SessionGate allowGuest>
            <p>tab B guest</p>
          </SessionGate>
        </SessionProvider>
      </>,
    );
    await screen.findByText("tab A guest");
    await screen.findByText("tab B guest");
    await act(async () => {
      for (const peer of peers)
        peer.onmessage?.({
          data: { type: "identity-changed", source: "other-tab" },
        });
    });
    expect(posts).toBe(0);
    expect(
      vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
    ).toHaveLength(4);
    expect(clear).toHaveBeenCalledTimes(2);
  } finally {
    cleanup();
    clear.mockRestore();
    vi.unstubAllGlobals();
  }
});

it("temporary timer failure is visible and retries pure refresh with bounded backoff while keeping draft", async () => {
  vi.useFakeTimers();
  document.cookie = "glowloom_csrf=proof; path=/";
  const now = Date.now();
  let refreshes = 0;
  const active = {
    ...validSession,
    session_protocol: "v2",
    refresh_generation: "0",
    upgrade_available: false,
    server_time: new Date(now).toISOString(),
    idle_expires_at: new Date(now + 43200000).toISOString(),
    access_expires_at: new Date(now + 120000).toISOString(),
    refresh_after_at: new Date(now + 60000).toISOString(),
  };
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/session/refresh") {
      refreshes++;
      if (refreshes === 1)
        throw new RequestError("temporarily_unavailable", 503);
      return active;
    }
    return url === "/api/session" ? active : { theme: "light" };
  });
  const view = render(
    <SessionProvider>
      <SessionGate>
        <input aria-label="retry draft" defaultValue="keep this" />
      </SessionGate>
    </SessionProvider>,
  );
  try {
    await act(async () => {});
    const draft = screen.getByLabelText("retry draft");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60001);
    });
    expect(refreshes).toBe(1);
    expect(notifications.show).toHaveBeenCalledWith(
      "登录续期暂不可用，正在自动重试。",
      expect.anything(),
    );
    expect(screen.getByLabelText("retry draft")).toBe(draft);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4999);
    });
    expect(refreshes).toBe(2);
    expect(screen.getByLabelText("retry draft")).toBe(draft);
    expect(draft).toHaveProperty("value", "keep this");
    expect(
      vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
    ).toHaveLength(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(refreshes).toBe(2);
  } finally {
    view.unmount();
    document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
    vi.useRealTimers();
  }
});
it.each(["hidden", "offline", "unmount"])(
  "temporary failure backoff stops on %s",
  async (boundary) => {
    vi.useFakeTimers();
    document.cookie = "glowloom_csrf=proof; path=/";
    const now = Date.now();
    let refreshes = 0;
    const active = {
      ...validSession,
      session_protocol: "v2",
      refresh_generation: "0",
      upgrade_available: false,
      server_time: new Date(now).toISOString(),
      idle_expires_at: new Date(now + 43200000).toISOString(),
      access_expires_at: new Date(now + 120000).toISOString(),
      refresh_after_at: new Date(now + 60000).toISOString(),
    };
    vi.mocked(request).mockImplementation(async (url) => {
      if (url === "/api/session/refresh") {
        refreshes++;
        throw new RequestError("temporarily_unavailable", 503);
      }
      return url === "/api/session" ? active : { theme: "light" };
    });
    const view = render(
      <SessionProvider>
        <SessionGate>
          <input aria-label="stopped draft" defaultValue="keep" />
        </SessionGate>
      </SessionProvider>,
    );
    try {
      await act(async () => {});
      await act(async () => {
        await vi.advanceTimersByTimeAsync(60001);
      });
      expect(refreshes).toBe(1);
      act(() => {
        if (boundary === "hidden") {
          Object.defineProperty(document, "visibilityState", {
            configurable: true,
            value: "hidden",
          });
          document.dispatchEvent(new Event("visibilitychange"));
        } else if (boundary === "offline") {
          Object.defineProperty(navigator, "onLine", {
            configurable: true,
            value: false,
          });
          window.dispatchEvent(new Event("offline"));
        } else view.unmount();
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(60000);
      });
      expect(refreshes).toBe(1);
    } finally {
      view.unmount();
      document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: "visible",
      });
      Object.defineProperty(navigator, "onLine", {
        configurable: true,
        value: true,
      });
      vi.useRealTimers();
    }
  },
);

it.each(["origin_forbidden", "origin_mismatch"])(
  "proxy %s renders trusted canonical link and preserves confirmed draft",
  async (code) => {
    vi.stubGlobal("location", new URL("http://localhost:3200/account"));
    let mismatch = false;
    vi.mocked(request).mockImplementation(async (url) => {
      if (url !== "/api/session") return { theme: "light" };
      if (mismatch)
        throw new RequestError(code, 403, {
          canonical_origin: "http://127.0.0.1:3200",
        });
      return validSession;
    });
    try {
      render(
        <SessionProvider>
          <SessionGate>
            <input aria-label="origin draft" defaultValue="keep" />
          </SessionGate>
        </SessionProvider>,
      );
      const draft = await screen.findByLabelText("origin draft");
      mismatch = true;
      await act(async () => window.dispatchEvent(new Event("focus")));
      expect(
        (
          await screen.findByRole("link", { name: "使用可信网站地址打开" })
        ).getAttribute("href"),
      ).toBe("http://127.0.0.1:3200/account");
      expect(screen.getByLabelText("origin draft")).toBe(draft);
      expect(draft).toHaveProperty("value", "keep");
    } finally {
      cleanup();
      vi.unstubAllGlobals();
    }
  },
);

it("429 renewal honors a valid server retry budget instead of retrying at five seconds", async () => {
  vi.useFakeTimers();
  document.cookie = "glowloom_csrf=proof; path=/";
  const now = Date.now();
  let refreshes = 0;
  const active = {
    ...validSession,
    session_protocol: "v2",
    refresh_generation: "0",
    upgrade_available: false,
    server_time: new Date(now).toISOString(),
    idle_expires_at: new Date(now + 43200000).toISOString(),
    access_expires_at: new Date(now + 120000).toISOString(),
    refresh_after_at: new Date(now + 60000).toISOString(),
  };
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/session/refresh") {
      refreshes++;
      if (refreshes === 1)
        throw new RequestError("rate_limited", 429, {
          retry_after_seconds: 20,
        });
      return active;
    }
    return url === "/api/session" ? active : { theme: "light" };
  });
  const view = render(
    <SessionProvider>
      <SessionGate>
        <input aria-label="budget draft" defaultValue="keep" />
      </SessionGate>
    </SessionProvider>,
  );
  try {
    await act(async () => {});
    await act(async () => {
      await vi.advanceTimersByTimeAsync(65000);
    });
    expect(refreshes).toBe(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15000);
    });
    expect(refreshes).toBe(2);
    expect(
      vi.mocked(request).mock.calls.filter(([url]) => url === "/api/session"),
    ).toHaveLength(1);
  } finally {
    view.unmount();
    document.cookie = "glowloom_csrf=; Max-Age=0; path=/";
    vi.useRealTimers();
  }
});

it("coalesced successful v1 upgrade receipt cannot create a zero-delay retry loop", async () => {
  vi.useFakeTimers();
  let upgrades = 0;
  const now = Date.now();
  const active = {
    ...validSession,
    session_protocol: "v1",
    upgrade_available: true,
    server_time: new Date(now).toISOString(),
    idle_expires_at: new Date(now + 43200000).toISOString(),
  };
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/session/upgrade") {
      upgrades++;
      if (upgrades > 1) throw new RequestError("temporarily_unavailable", 503);
      return active;
    }
    return url === "/api/session" ? active : { theme: "light" };
  });
  const view = render(
    <SessionProvider>
      <SessionGate>
        <p>coalesced legacy identity</p>
      </SessionGate>
    </SessionProvider>,
  );
  try {
    await act(async () => {});
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(upgrades).toBe(1);
    expect(screen.getByText("coalesced legacy identity")).toBeTruthy();
  } finally {
    view.unmount();
    vi.useRealTimers();
  }
});
