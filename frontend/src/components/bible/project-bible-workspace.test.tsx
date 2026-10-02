import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectBibleWorkspace } from "./project-bible-workspace";
import * as query from "./bible-queries";
import { getProject } from "@/components/project/queries";
import { ApiError } from "@/lib/request";
import { saveBibleIntent } from "./bible-intent";
const navigation = vi.hoisted(() => ({
  params: "entry_id=44444444-4444-4444-8444-444444444444",
  replace: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(navigation.params),
  usePathname: () => "/projects/33333333-3333-4333-8333-333333333333/bible",
  useRouter: () => ({ replace: navigation.replace }),
}));
vi.mock("@/components/project/queries", () => ({ getProject: vi.fn() }));
vi.mock("./bible-queries", async (original) => ({
  ...(await original<typeof import("./bible-queries")>()),
  listBible: vi.fn(),
  getBibleDetail: vi.fn(),
  freshBibleScope: vi.fn(),
  getBibleHistory: vi.fn(),
  getBibleSnapshot: vi.fn(),
}));
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  projectId: "33333333-3333-4333-8333-333333333333",
};
const page = {
  entries: [],
  current_actor_id: identity.actorId,
  current_org_id: identity.orgId,
};
beforeEach(() => {
  vi.resetAllMocks();
  navigation.params = "entry_id=44444444-4444-4444-8444-444444444444";
  sessionStorage.clear();
  vi.mocked(getProject).mockResolvedValue({
    id: identity.projectId,
    name: "合成项目",
    status: "active",
    is_delete: false,
  } as Awaited<ReturnType<typeof getProject>>);
  vi.mocked(query.freshBibleScope).mockResolvedValue(page);
  vi.mocked(query.getBibleHistory).mockResolvedValue({ versions: [] });
  vi.mocked(query.getBibleSnapshot).mockRejectedValue(
    new ApiError(404, "not_found"),
  );
});
afterEach(cleanup);
function renderPage(client: QueryClient) {
  return render(
    <QueryClientProvider client={client}>
      <ProjectBibleWorkspace projectId={identity.projectId} />
    </QueryClientProvider>,
  );
}
it("warm上下文与正文缓存不构成fresh Principal证明，deferred403前后均不挂私有正文子树", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(
    query.bibleContextKey(identity.origin, identity.projectId),
    page,
  );
  client.setQueryData(
    [
      ...query.bibleScopeKey(identity),
      "detail",
      "character",
      "44444444-4444-4444-8444-444444444444",
    ],
    { private_body: "旧私密正文" },
  );
  let reject!: (cause: unknown) => void;
  vi.mocked(query.listBible).mockImplementation(
    () =>
      new Promise((_, no) => {
        reject = no;
      }),
  );
  renderPage(client);
  await waitFor(() => expect(query.listBible).toHaveBeenCalled());
  expect(query.getBibleDetail).not.toHaveBeenCalled();
  expect(screen.queryByText("旧私密正文")).toBeNull();
  await act(async () => reject(new ApiError(403, "forbidden")));
  await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  expect(query.getBibleDetail).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "新建角色" })).toBeNull();
});
it("scope改变只按新actor初始化恢复，不重放旧键，私有读取始终绑定新scope", async () => {
  const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    }),
    newPage = {
      ...page,
      current_actor_id: "77777777-7777-4777-8777-777777777777",
    };
  vi.mocked(query.listBible).mockResolvedValue(newPage);
  vi.mocked(query.freshBibleScope).mockResolvedValue(newPage);
  vi.mocked(query.getBibleDetail).mockRejectedValue(
    new ApiError(404, "not_found"),
  );
  renderPage(client);
  await waitFor(() => expect(query.getBibleDetail).toHaveBeenCalled());
  expect(vi.mocked(query.getBibleDetail).mock.calls[0][0].actorId).toBe(
    newPage.current_actor_id,
  );
  expect(screen.queryByText("旧私密正文")).toBeNull();
});

it("刷新未知原意图优先于历史URL，只挂一个恢复Dialog；Escape保持原键且不读取历史正文", async () => {
  navigation.params += "&version_id=55555555-5555-4555-8555-555555555555";
  saveBibleIntent(sessionStorage, {
    ...identity,
    version: 1,
    key: "66666666-6666-4666-8666-666666666666",
    command: {
      action: "create",
      kind: "character",
      body: {
        expected_revision: 0,
        character: { name: "原完整待核验角色", definition: {} },
      },
    },
  });
  vi.mocked(query.listBible).mockResolvedValue(page);
  vi.mocked(query.getBibleDetail).mockRejectedValue(
    new ApiError(404, "not_found"),
  );
  renderPage(
    new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  await screen.findByRole("button", { name: "人工使用原键和原输入核验" });
  await waitFor(() =>
    expect(screen.getByRole("dialog").contains(document.activeElement)).toBe(
      true,
    ),
  );
  await waitFor(() =>
    expect(document.querySelectorAll("[data-bible-dialog]")).toHaveLength(1),
  );
  expect(query.getBibleHistory).not.toHaveBeenCalled();
  expect(query.getBibleSnapshot).not.toHaveBeenCalled();
  const replay = screen.getByRole("button", {
    name: "人工使用原键和原输入核验",
  });
  replay.focus();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(document.activeElement).toBe(replay);
  expect(
    screen.getByRole("button", { name: "人工使用原键和原输入核验" }),
  ).toBeTruthy();
  expect(navigation.replace).not.toHaveBeenCalled();
});

it("历史URL正常恢复时阻断后台新动作，关闭后将焦点返回本项目section并保留其他URL参数", async () => {
  navigation.params +=
    "&version_id=55555555-5555-4555-8555-555555555555&copy_source=77777777-7777-4777-8777-777777777777";
  vi.mocked(query.listBible).mockResolvedValue(page);
  vi.mocked(query.getBibleDetail).mockRejectedValue(
    new ApiError(404, "not_found"),
  );
  const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    }),
    view = renderPage(client);
  await screen.findByRole("heading", { name: "不可变设定版本历史" });
  const backgroundCreate = Array.from(document.querySelectorAll("button")).find(
    (button) => button.textContent === "新建角色",
  );
  expect(backgroundCreate).toHaveProperty("disabled", true);
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  const href = vi.mocked(navigation.replace).mock.calls.at(-1)?.[0] as string;
  expect(href).toContain("copy_source=77777777-7777-4777-8777-777777777777");
  expect(href).not.toContain("version_id");
  navigation.params = href.split("?")[1] ?? "";
  view.rerender(
    <QueryClientProvider client={client}>
      <ProjectBibleWorkspace projectId={identity.projectId} />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "当前项目设定集",
    ),
  );
});
