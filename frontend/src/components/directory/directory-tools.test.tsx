import {
  cleanup,
  fireEvent,
  render,
  screen,
  act,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DirectoryTools } from "./directory-tools";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
const snapshot = {
  as_of: "2026-10-15T00:00:00Z",
  filter: { status: "deleted" },
  total_users: 2,
  verified_users: 1,
  active_team_users: 0,
  membership_count: 0,
  account_states: { active: 0, disabled: 0, deleted: 2 },
};
const pending = {
  id: "11111111-1111-4111-8111-111111111111",
  status: "pending",
  as_of: snapshot.as_of,
  row_count: 2,
  expires_at: null,
};
const filter = { status: "deleted", verified: true };
beforeEach(() => {
  URL.createObjectURL = vi.fn(() => "blob:export");
  URL.revokeObjectURL = vi.fn();
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
async function create() {
  fireEvent.click(await screen.findByText("创建 CSV 导出"));
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "valid password" },
  });
  fireEvent.click(screen.getByText("确认操作"));
  await screen.findByText("导出状态：排队中");
}
it("统计与导出保持同一筛选，未完成的nullable到期时间不显示1970日期，过期文件禁止下载", async () => {
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/platform/user-statistics") return snapshot;
    if (url === "/api/me/reauthentication") return undefined;
    if (url === "/api/platform/user-exports") return pending;
    if (url.includes("/user-exports/"))
      return {
        ...pending,
        status: "completed",
        expires_at: new Date(Date.now() - 1000).toISOString(),
      };
    throw new Error("unexpected request");
  });
  render(<DirectoryTools filter={filter} />);
  await screen.findByText("账号总数");
  expect(request).toHaveBeenCalledWith(
    "/api/platform/user-statistics",
    expect.objectContaining({ params: filter }),
  );
  await create();
  expect(request).toHaveBeenCalledWith(
    "/api/platform/user-exports",
    expect.objectContaining({
      data: { filter },
      headers: { "Idempotency-Key": expect.any(String) },
    }),
  );
  expect(screen.queryByText(/1970/)).toBeNull();
  expect(screen.getByText("下载 CSV")).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByText("检查导出状态"));
  await screen.findByText("导出状态：已到期");
  expect(screen.getByText("下载 CSV")).toHaveProperty("disabled", true);
  expect(URL.createObjectURL).not.toHaveBeenCalled();
});
it("下载被撤权不创建任何本地输出", async () => {
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/platform/user-statistics") return snapshot;
    if (url === "/api/me/reauthentication") return undefined;
    if (url.endsWith("/download")) throw new RequestError("forbidden", 403);
    if (url === "/api/platform/user-exports") return pending;
    return {
      ...pending,
      status: "completed",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    };
  });
  render(<DirectoryTools filter={filter} />);
  await create();
  fireEvent.click(screen.getByText("检查导出状态"));
  await screen.findByText("导出状态：可下载");
  fireEvent.click(screen.getByText("下载 CSV"));
  await screen.findByRole("alert");
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  expect(screen.queryByText("下载 CSV")).toBeNull();
  expect(screen.queryByText("账号总数")).toBeNull();
  expect(request).toHaveBeenCalledWith(
    expect.stringMatching(/\/download$/),
    expect.objectContaining({
      responseType: "blob",
      signal: expect.any(AbortSignal),
    }),
  );
});
it("离开当前视图后迟到CSV不能触发下载", async () => {
  let resolve!: (blob: Blob) => void;
  const download = new Promise<Blob>((r) => (resolve = r));
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/platform/user-statistics") return snapshot;
    if (url === "/api/me/reauthentication") return undefined;
    if (url.endsWith("/download")) return download;
    if (url === "/api/platform/user-exports") return pending;
    return {
      ...pending,
      status: "completed",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    };
  });
  const view = render(<DirectoryTools filter={filter} />);
  await create();
  fireEvent.click(screen.getByText("检查导出状态"));
  await screen.findByText("导出状态：可下载");
  fireEvent.click(screen.getByText("下载 CSV"));
  view.unmount();
  await act(async () => resolve(new Blob(["private,rows"])));
  expect(URL.createObjectURL).not.toHaveBeenCalled();
});
it("团队统计保留member角色及暂停筛选，普通失败不伪造为零", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("temporarily_unavailable", 503),
  );
  render(
    <DirectoryTools
      teamID="team"
      filter={{ role: "member", status: "suspended" }}
    />,
  );
  await screen.findByRole("alert");
  expect(screen.queryByText("成员关系总数")).toBeNull();
  expect(request).toHaveBeenCalledWith(
    "/api/teams/team/member-statistics",
    expect.objectContaining({
      params: { role: "member", status: "suspended" },
    }),
  );
});

it("下载等待期间身份代次变化，即使回执成功也不保存旧身份CSV", async () => {
  const { setCSRFToken } = await import("@/lib/request");
  let resolve!: (blob: Blob) => void;
  const late = new Promise<Blob>((r) => (resolve = r));
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/platform/user-statistics") return snapshot;
    if (url === "/api/me/reauthentication") return undefined;
    if (url.endsWith("/download")) return late;
    if (url === "/api/platform/user-exports") return pending;
    return {
      ...pending,
      status: "completed",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    };
  });
  render(<DirectoryTools filter={filter} />);
  await create();
  fireEvent.click(screen.getByText("检查导出状态"));
  await screen.findByText("导出状态：可下载");
  fireEvent.click(screen.getByText("下载 CSV"));
  setCSRFToken("new-export-view-identity");
  await act(async () => resolve(new Blob(["old private output"])));
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  setCSRFToken(null);
});

it("超过10000行的409明确要求缩小筛选，不误报资料冲突或成功", async () => {
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/platform/user-statistics") return snapshot;
    if (url === "/api/me/reauthentication") return undefined;
    throw new RequestError("export_limit_exceeded", 409);
  });
  render(<DirectoryTools filter={filter} />);
  fireEvent.click(await screen.findByText("创建 CSV 导出"));
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "valid password" },
  });
  fireEvent.click(screen.getByText("确认操作"));
  await screen.findByText(/当前筛选超过导出上限/);
  expect(screen.queryByText(/导出状态/)).toBeNull();
});
