import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
  act,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({
  role: "platform_admin",
  actor: "actor-1",
  creation: vi.fn(),
  business: vi.fn(),
  overview: vi.fn(),
  logs: vi.fn(),
  detail: vi.fn(),
  audit: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  SessionGate: ({
    children,
    admin,
  }: {
    children: React.ReactNode;
    admin?: boolean;
  }) => (admin && mocks.role !== "platform_admin" ? <p>无权访问</p> : children),
  useSession: () => ({
    actor: { id: mocks.actor, platform_role: mocks.role, revision: "1" },
    session_id: "session-1",
  }),
}));
vi.mock("@/api/analytics", () => ({
  getCreationAnalytics: mocks.creation,
  getBusinessAnalyticsCapability: mocks.business,
}));
vi.mock("@/api/diagnostics", () => ({
  getDiagnosticsOverview: mocks.overview,
  listDiagnosticLogs: mocks.logs,
  getDiagnosticLog: mocks.detail,
}));
vi.mock("@/api/identity", () => ({ getPlatformSecurityEvents: mocks.audit }));
vi.mock("@/components/insights/creation-chart", () => ({
  default: () => <p>动态趋势图</p>,
}));
import { Analytics } from "@/app/analytics/_components/analytics";
import { Diagnostics } from "@/app/health/_components/diagnostics";
import { AuditEvents } from "@/app/audit/_components/audit-events";
const now = "2026-11-13T10:00:00Z";
const coverage = {
  retained: 2,
  capacity: 5000,
  evicted: 0,
  retention_seconds: 86400,
  started_at: now,
  oldest_at: now,
  missing_services: ["agent", "canvas"],
  restart_loss: true,
  status: "partial",
  source: "process_memory",
};
const creation = {
  accepted_operations: 2,
  completed_operations: 1,
  outputs: 3,
  attempts: 4,
  terminal_success_rate: 0,
  from: now,
  to: now,
  as_of: now,
  scope: "personal",
  timezone: "Asia/Shanghai",
  coverage: {
    truncated: false,
    max_operations: 10000,
    capabilities: [],
    source: "operation_facts",
    status: "available",
  },
  daily: [],
  models: [],
  entries: [],
  statuses: [],
  failed_operations: 0,
  cancelled_operations: 0,
  partial_operations: 0,
  unknown_operations: 0,
};
beforeEach(() => {
  mocks.role = "platform_admin";
  mocks.actor = "actor-1";
  mocks.creation.mockResolvedValue(creation);
  mocks.business.mockResolvedValue({ as_of: now, status: "unavailable" });
  mocks.overview.mockResolvedValue({
    as_of: now,
    coverage,
    requests: 0,
    server_error_rate: null,
    server_errors: 0,
    p95_ms: null,
    panics: 0,
    client_errors: 0,
    rate_limited: 0,
  });
  mocks.logs.mockResolvedValue({
    as_of: now,
    coverage,
    items: [],
    next_cursor: null,
  });
  mocks.audit.mockResolvedValue({ as_of: now, items: [], next_cursor: "" });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("creator gets personal analysis with no management or business/export links and no invented people metrics", async () => {
  mocks.role = "creator";
  render(<Analytics />);
  await screen.findByText("已受理任务");
  expect(
    screen.getByRole("link", { name: "我的分析" }).getAttribute("href"),
  ).toBe("/analytics");
  expect(screen.queryByRole("link", { name: "运行诊断" })).toBeNull();
  expect(screen.queryByRole("tab", { name: "经营" })).toBeNull();
  expect(screen.queryByText("导出未开放")).toBeNull();
  expect(screen.queryByText("有效创作用户")).toBeNull();
  expect(mocks.creation.mock.calls[0][0]).toEqual({ scope: "personal" });
});
it("does not apply draft filters until submit, then all requests use the applied source/scope", async () => {
  render(<Analytics />);
  await screen.findByText("已受理任务");
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  fireEvent.change(screen.getByLabelText("创作来源"), {
    target: { value: "canvas" },
  });
  expect(mocks.creation).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByLabelText("分析范围"), {
    target: { value: "platform" },
  });
  fireEvent.click(screen.getByRole("button", { name: "筛选" }));
  await waitFor(() => expect(mocks.creation).toHaveBeenCalledTimes(2));
  expect(mocks.creation.mock.calls[1][0]).toEqual({
    scope: "platform",
    origin: "canvas",
  });
  fireEvent.click(screen.getByRole("button", { name: "重置" }));
  await waitFor(() => expect(mocks.creation).toHaveBeenCalledTimes(3));
  expect(mocks.creation.mock.calls[2][0]).toEqual({ scope: "personal" });
});
it("business exposes unavailable capability only, with disabled export and unknown profit", async () => {
  render(<Analytics />);
  fireEvent.mouseDown(screen.getByRole("tab", { name: "经营" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("经营数据未接入");
  expect(
    (screen.getByRole("button", { name: "导出未开放" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.queryByText(/¥/)).toBeNull();
  expect(screen.getByText("贡献利润")).toBeTruthy();
  expect(mocks.business).toHaveBeenCalledTimes(1);
});
it("diagnostic zero denominator is inapplicable and never full traffic coverage", async () => {
  render(<Diagnostics />);
  await screen.findByText("已观测请求");
  expect(screen.getAllByText("不适用").length).toBe(2);
  expect(screen.getByText("仅后端")).toBeTruthy();
  expect(screen.getByText(/不代表全站流量/)).toBeTruthy();
  expect(screen.queryByText("4/4")).toBeNull();
});
it("audit detail uses safe authorized strings, pagination and closes/restores invoking focus", async () => {
  const event = {
    id: "evt-1",
    action: "user.disabled",
    result: "success",
    reason: "合成安全原因",
    created_at: now,
    actor_id: "actor",
    target_user_id: "target",
    team_id: "",
    request_id: "request",
    safe_diff: { status: "active → disabled", secret: "must never show" },
  };
  mocks.audit
    .mockResolvedValueOnce({ as_of: now, items: [event], next_cursor: "next" })
    .mockResolvedValue({ as_of: now, items: [], next_cursor: "" });
  render(<AuditEvents />);
  const trigger = await screen.findByRole("button", { name: "查看审计 evt-1" });
  fireEvent.click(trigger);
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("active → disabled")).toBeTruthy();
  expect(within(dialog).queryByText("must never show")).toBeNull();
  fireEvent.keyDown(dialog, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await waitFor(() => expect(mocks.audit).toHaveBeenCalledTimes(2));
  expect(mocks.audit.mock.calls[1][0].cursor).toBe("next");
  expect(screen.getByRole("button", { name: "上一页" })).toBeTruthy();
});
it("identity switch immediately drops private analytics and ignores late old response", async () => {
  let oldResolve!: (v: typeof creation) => void;
  mocks.creation
    .mockImplementationOnce(
      () =>
        new Promise((r) => {
          oldResolve = r;
        }),
    )
    .mockResolvedValue({ ...creation, accepted_operations: 9 });
  const view = render(<Analytics />);
  mocks.actor = "actor-2";
  view.rerender(<Analytics />);
  await screen.findByText("9");
  await act(async () => oldResolve({ ...creation, accepted_operations: 888 }));
  expect(screen.queryByText("888")).toBeNull();
});

it("same-scope refresh retains content and a pending status, then replaces the snapshot", async () => {
  let resolve!: (value: typeof creation) => void;
  mocks.creation.mockResolvedValueOnce(creation).mockImplementationOnce(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  render(<Analytics />);
  await screen.findByText("已受理任务");
  fireEvent.click(screen.getByRole("button", { name: "刷新" }));
  await waitFor(() => expect(mocks.creation).toHaveBeenCalledTimes(2));
  expect(screen.getByText("已受理任务")).toBeTruthy();
  expect(screen.getByText(/正在刷新，保留上次结果/)).toBeTruthy();
  await act(async () => resolve({ ...creation, accepted_operations: 7 }));
  expect(await screen.findByText("7")).toBeTruthy();
});
it("mobile filter sheet keeps unsubmitted draft when dismissed and applies explicitly", async () => {
  render(<Analytics />);
  await screen.findByText("已受理任务");
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  let dialog = await screen.findByRole("dialog");
  fireEvent.change(within(dialog).getByLabelText("创作来源"), {
    target: { value: "agent" },
  });
  fireEvent.keyDown(dialog, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(mocks.creation).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  dialog = await screen.findByRole("dialog");
  expect(
    (within(dialog).getByLabelText("创作来源") as HTMLSelectElement).value,
  ).toBe("agent");
  fireEvent.click(within(dialog).getByRole("button", { name: "筛选" }));
  await waitFor(() => expect(mocks.creation).toHaveBeenCalledTimes(2));
  expect(mocks.creation.mock.calls[1][0].origin).toBe("agent");
});
it("403 refresh closes audit private detail, and retry cannot reopen the old authorized row", async () => {
  const event = {
    id: "private-audit",
    action: "user.disabled",
    result: "success",
    reason: "private-reason",
    created_at: now,
    actor_id: "actor",
    target_user_id: "target",
    team_id: "",
    request_id: "request",
    safe_diff: {},
  };
  mocks.audit
    .mockResolvedValueOnce({ as_of: now, items: [event], next_cursor: "" })
    .mockRejectedValueOnce({ status: 403 })
    .mockImplementation(() => new Promise(() => {}));
  render(<AuditEvents />);
  fireEvent.click(
    await screen.findByRole("button", { name: "查看审计 private-audit" }),
  );
  expect(await screen.findByText("private-reason")).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  fireEvent.click(screen.getByRole("button", { name: "刷新" }));
  await screen.findByText("无权读取此范围");
  expect(
    screen.queryByRole("button", { name: "查看审计 private-audit" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(screen.queryByText("private-reason")).toBeNull();
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("keeps advanced fields in the filter sheet and exposes only an applied summary in the page", async () => {
  render(<Analytics />);
  await screen.findByText("已受理任务");
  expect(screen.queryByLabelText("模型版本 UUID")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByLabelText("模型版本 UUID")).toBeTruthy();
  fireEvent.change(within(dialog).getByLabelText("创作来源"), {
    target: { value: "canvas" },
  });
  fireEvent.click(within(dialog).getByRole("button", { name: "筛选" }));
  expect(await screen.findByText("已应用 1 项条件")).toBeTruthy();
});

it("keeps applied platform filters and datetime drafts consistent after business tab round-trip", async () => {
  render(<Analytics />);
  await screen.findByText("已受理任务");
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  fireEvent.change(screen.getByLabelText("分析范围"), {
    target: { value: "platform" },
  });
  fireEvent.change(screen.getByLabelText("创作来源"), {
    target: { value: "canvas" },
  });
  fireEvent.change(screen.getByLabelText("开始时间"), {
    target: { value: "2026-11-01T00:00" },
  });
  fireEvent.change(screen.getByLabelText("结束时间（不含）"), {
    target: { value: "2026-11-02T00:00" },
  });
  fireEvent.click(screen.getByRole("button", { name: "筛选" }));
  await waitFor(() => expect(mocks.creation).toHaveBeenCalledTimes(2));
  const applied = {
    scope: "platform",
    origin: "canvas",
    from: "2026-10-31T16:00:00.000Z",
    to: "2026-11-01T16:00:00.000Z",
  };
  expect(mocks.creation.mock.calls.at(-1)![0]).toEqual(applied);
  fireEvent.mouseDown(screen.getByRole("tab", { name: "经营" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("经营数据未接入");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "创作" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("已受理任务");
  expect(screen.getByText("已应用 4 项条件")).toBeTruthy();
  expect(mocks.creation.mock.calls.at(-1)![0]).toEqual(applied);
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  expect((screen.getByLabelText("分析范围") as HTMLSelectElement).value).toBe(
    "platform",
  );
  expect((screen.getByLabelText("创作来源") as HTMLSelectElement).value).toBe(
    "canvas",
  );
  expect((screen.getByLabelText("开始时间") as HTMLInputElement).value).toBe(
    "2026-11-01T00:00",
  );
  fireEvent.click(screen.getByRole("button", { name: "筛选" }));
  expect(mocks.creation.mock.calls.at(-1)![0]).toEqual(applied);
  expect(screen.getByText("已应用 4 项条件")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重置" }));
  await waitFor(() =>
    expect(mocks.creation.mock.calls.at(-1)![0]).toEqual({ scope: "personal" }),
  );
  expect(screen.queryByText(/已应用 \d+ 项条件/)).toBeNull();
});
it("keeps unsubmitted drafts through tab round-trip but clears them synchronously on identity change", async () => {
  const view = render(<Analytics />);
  await screen.findByText("已受理任务");
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  fireEvent.change(screen.getByLabelText("分析范围"), {
    target: { value: "platform" },
  });
  fireEvent.change(screen.getByLabelText("创作来源"), {
    target: { value: "canvas" },
  });
  fireEvent.click(screen.getByRole("button", { name: "筛选" }));
  await waitFor(() => expect(mocks.creation).toHaveBeenCalledTimes(2));
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  fireEvent.change(screen.getByLabelText("创作来源"), {
    target: { value: "agent" },
  });
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  fireEvent.mouseDown(screen.getByRole("tab", { name: "经营" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("经营数据未接入");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "创作" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("已受理任务");
  expect(mocks.creation.mock.calls.at(-1)![0]).toEqual({
    scope: "platform",
    origin: "canvas",
  });
  expect(screen.getByText("已应用 2 项条件")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  expect((screen.getByLabelText("创作来源") as HTMLSelectElement).value).toBe(
    "agent",
  );
  expect((screen.getByLabelText("分析范围") as HTMLSelectElement).value).toBe(
    "platform",
  );
  mocks.actor = "actor-2";
  view.rerender(<Analytics />);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByText(/已应用 \d+ 项条件/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "筛选条件" }));
  expect((screen.getByLabelText("分析范围") as HTMLSelectElement).value).toBe(
    "personal",
  );
  expect((screen.getByLabelText("创作来源") as HTMLSelectElement).value).toBe(
    "",
  );
});

async function openLogs() {
  render(<Diagnostics />);
  fireEvent.mouseDown(screen.getByRole("tab", { name: "日志" }), {
    button: 0,
    ctrlKey: false,
  });
}
const logRow = {
  id: "log-id",
  request_id: "private-log",
  status: 200,
  level: "info",
  timestamp: now,
  elapsed_ms: 1,
  route: "/api/test",
  method: "GET",
  service: "backend",
};
const logPage = {
  as_of: now,
  coverage,
  items: [logRow],
  first_cursor: "signed-first",
  next_cursor: "signed-next",
};

it.each([401, 403])(
  "detail %i removes its private list until a new authorized list succeeds",
  async (status) => {
    mocks.logs
      .mockResolvedValueOnce(logPage)
      .mockRejectedValueOnce({ status })
      .mockResolvedValueOnce({
        ...logPage,
        items: [{ ...logRow, request_id: "reauthorized" }],
      });
    mocks.detail.mockRejectedValueOnce({ status });
    await openLogs();
    fireEvent.click(
      await screen.findByRole("button", { name: "查看日志 private-log" }),
    );
    await screen.findByText("无权读取此范围");
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByText("private-log")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await screen.findByText("无权读取此范围");
    expect(screen.queryByText("private-log")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await screen.findByText("reauthorized");
  },
);

it.each(["audit", "logs"] as const)(
  "%s navigates using signed history and refreshes from a fresh first page",
  async (kind) => {
    const mock = mocks[kind];
    mock.mockImplementation(async (q) => ({
      ...logPage,
      items: [],
      next_cursor: q.cursor === "signed-next" ? null : "signed-next",
    }));
    if (kind === "audit") render(<AuditEvents />);
    else await openLogs();
    async function press(name: string, calls: number) {
      const button = await screen.findByRole("button", { name });
      await waitFor(() =>
        expect((button as HTMLButtonElement).disabled).toBe(false),
      );
      fireEvent.click(button);
      await waitFor(() => expect(mock).toHaveBeenCalledTimes(calls));
    }
    await press("下一页", 2);
    await press("上一页", 3);
    await press("下一页", 4);
    await press("刷新", 5);
    expect(mock.mock.calls.map(([q]) => q.cursor)).toEqual([
      undefined,
      "signed-next",
      "signed-first",
      "signed-next",
      undefined,
    ]);
    expect(await screen.findByText(/新快照/)).toBeTruthy();
  },
);
it("recovers from an expired second audit page by retrying the first page", async () => {
  mocks.audit
    .mockResolvedValueOnce({
      as_of: now,
      items: [],
      first_cursor: "first",
      next_cursor: "expired",
    })
    .mockRejectedValueOnce({ status: 422 })
    .mockResolvedValueOnce({
      as_of: now,
      items: [],
      first_cursor: "new",
      next_cursor: null,
    });
  render(<AuditEvents />);
  fireEvent.click(await screen.findByRole("button", { name: "下一页" }));
  fireEvent.click(await screen.findByRole("button", { name: "重试" }));
  await screen.findByText("暂无安全事件");
  expect(mocks.audit.mock.calls.at(-1)![0].cursor).toBeUndefined();
  expect(screen.getByText(/新快照/)).toBeTruthy();
});
