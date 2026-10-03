import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import {
  characterBindingKey,
  listCharacterBindings,
  resolveCharacterBinding,
} from "./character-binding-queries";
const mocked = vi.hoisted(() => ({
  workspace: vi.fn(),
  list: vi.fn(),
  detail: vi.fn(),
  snapshot: vi.fn(),
  version: vi.fn(),
}));
vi.mock("@/api/bible", () => ({ getBibleVersion: mocked.version }));
vi.mock("./source-queries", async (original) => ({
  ...(await original<typeof import("./source-queries")>()),
  getWorkspace: mocked.workspace,
}));
vi.mock("@/components/bible/bible-queries", () => ({
  listBible: mocked.list,
  getBibleDetail: mocked.detail,
  getBibleSnapshot: mocked.snapshot,
}));
const id = (n: number) =>
  `${n.toString().repeat(8)}-${n.toString().repeat(4)}-4${n.toString().repeat(3)}-8${n.toString().repeat(3)}-${n.toString().repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const workspace = {
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
  state: { project_id: scope.projectId },
};
const summary = (n: number, flags = {}) => ({
  head: { id: id(n), confirmed_version_id: id(7), deleted: false, ...flags },
  name: `角色${n}`,
});
const snapshot = (entry = id(4), version = id(7), name = "原确认名称") => ({
  id: version,
  entry_id: entry,
  org_id: scope.orgId,
  project_id: scope.projectId,
  kind: "character",
  number: 1,
  actor_id: scope.actorId,
  created_at: "2026-10-02T00:00:00Z",
  origin: "manual",
  content_sha256: "a".repeat(64),
  character: {
    name,
    aliases: ["  小雨😀  ", "雨队长"],
    definition: {},
    looks: [{ id: id(6), name: "默认造型", default: true }],
  },
});
beforeEach(() => {
  vi.resetAllMocks();
  mocked.workspace.mockResolvedValue(workspace);
  mocked.list.mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    entries: [summary(4)],
    next_cursor: "frozen-cursor",
  });
  mocked.detail.mockResolvedValue({
    head: { id: id(4), deleted: false, confirmed_version_id: id(7) },
    resolved_id: id(4),
    current: {
      id: id(8),
      kind: "character",
      character: { name: "未确认当前名称" },
    },
  });
  mocked.snapshot.mockResolvedValue({
    id: id(7),
    kind: "character",
    character: { name: "原确认名称" },
  });
  mocked.version.mockResolvedValue(snapshot());
});
it("scope proof与正式列表并行；每页只接受confirmed活动身份，cursor不重写", async () => {
  let resolve!: (value: unknown) => void;
  mocked.workspace.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  mocked.list.mockResolvedValue({
    entries: [
      summary(4),
      summary(5, { confirmed_version_id: undefined }),
      summary(6, { redirect_id: id(4) }),
      summary(9, { deleted: true }),
    ],
    next_cursor: "opaque",
  });
  const pending = listCharacterBindings(scope, "original");
  expect(mocked.workspace).toHaveBeenCalledOnce();
  expect(mocked.list).toHaveBeenCalledWith(
    scope.projectId,
    "character",
    { cursor: "original", limit: 25 },
    undefined,
    scope,
  );
  resolve(workspace);
  expect(await pending).toEqual({
    items: [
      {
        id: id(4),
        name: "原确认名称",
        aliases: ["  小雨😀  ", "雨队长"],
        confirmedVersionId: id(7),
      },
    ],
    next_cursor: "opaque",
  });
  expect(characterBindingKey(scope)).toEqual([
    "project",
    scope.projectId,
    "script",
    scope.origin,
    scope.actorId,
    scope.orgId,
    "character-bindings",
  ]);
});
it("同页多个确认快照并行读取，名称和别名只来自exact确认版本", async () => {
  mocked.list.mockResolvedValue({ entries: [summary(4), summary(5)] });
  const releases: ((value: unknown) => void)[] = [];
  mocked.version.mockImplementation(
    () => new Promise((resolve) => releases.push(resolve)),
  );
  const pending = listCharacterBindings(scope);
  await vi.waitFor(() => expect(mocked.version).toHaveBeenCalledTimes(2));
  releases[0](snapshot(id(4), id(7), "确认名称一"));
  releases[1](snapshot(id(5), id(7), "确认名称二"));
  expect((await pending).items.map((item) => item.name)).toEqual([
    "确认名称一",
    "确认名称二",
  ]);
  expect(mocked.version.mock.calls[0][0]).toEqual({
    pid: scope.projectId,
    kind: "character",
    id: id(4),
    version: id(7),
  });
});
it.each([
  { project_id: id(9) },
  { org_id: id(9) },
  { entry_id: id(9) },
  { id: id(9) },
  { character: { ...snapshot().character, unknown: "不丢弃未知字段" } },
])("确认快照必须匹配owner/entry/pin，异常整页拒绝 %j", async (change) => {
  mocked.version.mockResolvedValue({ ...snapshot(), ...change });
  await expect(listCharacterBindings(scope)).rejects.toMatchObject({
    status: 502,
  });
});
it("cursor反复返回原值时拒绝继续分页，不能静默循环", async () => {
  mocked.list.mockResolvedValue({ entries: [], next_cursor: "same" });
  await expect(listCharacterBindings(scope, "same")).rejects.toMatchObject({
    status: 502,
  });
});
it("principal/org改变不释放私有候选，不解析任何新binding", async () => {
  mocked.workspace.mockResolvedValue({ ...workspace, current_actor_id: id(9) });
  await expect(listCharacterBindings(scope)).rejects.toMatchObject({
    code: "scope_changed",
  });
  await expect(resolveCharacterBinding(scope, id(4))).rejects.toMatchObject({
    code: "scope_changed",
  });
  expect(mocked.snapshot).not.toHaveBeenCalled();
  expect(mocked.version).not.toHaveBeenCalled();
});
it("显式选择读取exact immutable confirmed版本，不能把未确认current冒充确认", async () => {
  expect(await resolveCharacterBinding(scope, id(4))).toEqual({
    character_id: id(4),
    character_version_id: id(7),
    name: "原确认名称",
    aliases: [],
  });
  expect(mocked.snapshot).toHaveBeenCalledWith(
    scope,
    "character",
    id(4),
    id(7),
    undefined,
  );
});
it.each([
  { confirmed_version_id: undefined },
  { deleted: true },
  { redirect_id: id(5) },
])("选择后权限或身份失效拒绝新binding %j", async (flags) => {
  mocked.detail.mockResolvedValue({
    head: { id: id(4), deleted: false, confirmed_version_id: id(7), ...flags },
    resolved_id: id(4),
    current: { id: id(7), kind: "character" },
  });
  await expect(resolveCharacterBinding(scope, id(4))).rejects.toBeInstanceOf(
    ApiError,
  );
});
