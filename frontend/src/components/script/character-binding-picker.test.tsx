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
import { CharacterBindingPicker } from "./character-binding-picker";
import { characterBindingKey } from "./character-binding-queries";
const api = vi.hoisted(() => ({ list: vi.fn(), resolve: vi.fn() }));
vi.mock("./character-binding-queries", async (original) => ({
  ...(await original<typeof import("./character-binding-queries")>()),
  listCharacterBindings: api.list,
  resolveCharacterBinding: api.resolve,
}));
const id = (n: number) =>
  `${n.toString().repeat(8)}-${n.toString().repeat(4)}-4${n.toString().repeat(3)}-8${n.toString().repeat(3)}-${n.toString().repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const old = { character_id: id(4), character_version_id: id(5) };
const page = {
  items: [
    {
      id: id(6),
      name: "合成确认角色😀",
      aliases: ["  小雨😀  "],
      confirmedVersionId: id(7),
    },
  ],
  next_cursor: "original",
};
function mount(
  client: QueryClient,
  props: Partial<React.ComponentProps<typeof CharacterBindingPicker>> = {},
) {
  const change = vi.fn();
  const ui = render(
    <QueryClientProvider client={client}>
      <CharacterBindingPicker
        scope={scope}
        label="场景1行1"
        binding={old}
        locked={false}
        onChange={change}
        onPendingChange={vi.fn()}
        {...props}
      />
    </QueryClientProvider>,
  );
  return { ...ui, change };
}
beforeEach(() => {
  vi.resetAllMocks();
  api.list.mockResolvedValue(page);
  api.resolve.mockResolvedValue({
    character_id: id(6),
    character_version_id: id(7),
    name: "合成确认角色😀",
    aliases: ["  小雨😀  "],
  });
});
afterEach(cleanup);
it("保留旧pin不自动重绑；warm私有候选必须等fresh proof，403不显示旧缓存", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(characterBindingKey(scope), {
    pages: [page],
    pageParams: [undefined],
  });
  let reject!: (cause: Error) => void;
  api.list.mockImplementation(
    () =>
      new Promise((_, r) => {
        reject = r;
      }),
  );
  const { change } = mount(client);
  expect(api.list).not.toHaveBeenCalled();
  expect(
    screen.getByText(old.character_version_id, { exact: false }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "场景1行1选择正式角色" }));
  expect(screen.queryByRole("option", { name: /合成确认角色/ })).toBeNull();
  await act(async () => reject(new Error("403当前授权不可读取")));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByRole("option", { name: /合成确认角色/ })).toBeNull();
  expect(change).not.toHaveBeenCalled();
});
it("完整cursor分页且显式选择exact确认pin，清空同时清ID与pin", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  api.list.mockResolvedValueOnce(page).mockResolvedValueOnce({
    items: [
      {
        id: id(8),
        name: "第二页真实角色",
        aliases: [],
        confirmedVersionId: id(9),
      },
    ],
  });
  const { change } = mount(client);
  fireEvent.click(screen.getByRole("button", { name: "场景1行1选择正式角色" }));
  await screen.findByRole("option", { name: /合成确认角色/ });
  expect(
    screen.getByRole("option", { name: /合成确认角色/ }).textContent,
  ).toContain("  小雨😀  ");
  fireEvent.click(screen.getByRole("button", { name: "读取更多确认角色" }));
  await screen.findByRole("option", { name: /第二页真实角色/ });
  expect(api.list.mock.calls[1][1]).toBe("original");
  fireEvent.change(
    screen.getByRole("combobox", { name: "场景1行1正式确认角色" }),
    { target: { value: id(6) } },
  );
  await waitFor(() =>
    expect(change).toHaveBeenCalledWith({
      character_id: id(6),
      character_version_id: id(7),
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "场景1行1清除角色绑定" }));
  expect(change).toHaveBeenLastCalledWith(undefined);
});
it("scope切换或卸载不会消费旧异步binding", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  let resolve!: (value: unknown) => void;
  api.resolve.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const pending = vi.fn();
  const { change, unmount } = mount(client, { onPendingChange: pending });
  fireEvent.click(screen.getByRole("button", { name: "场景1行1选择正式角色" }));
  await screen.findByRole("option", { name: /合成确认角色/ });
  fireEvent.change(
    screen.getByRole("combobox", { name: "场景1行1正式确认角色" }),
    { target: { value: id(6) } },
  );
  unmount();
  await act(async () =>
    resolve({
      character_id: id(6),
      character_version_id: id(7),
      name: "角色",
      aliases: [],
    }),
  );
  expect(change).not.toHaveBeenCalled();
  expect(pending).toHaveBeenLastCalledWith(false);
});
it("选择fresh失败保留旧pin与草稿，新的确认事实不沿列表cached pin绑定", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  api.resolve.mockRejectedValueOnce(new Error("409身份发生变化"));
  const { change } = mount(client);
  fireEvent.click(screen.getByRole("button", { name: "场景1行1选择正式角色" }));
  await screen.findByRole("option", { name: /合成确认角色/ });
  const select = screen.getByRole("combobox", { name: "场景1行1正式确认角色" });
  fireEvent.change(select, { target: { value: id(6) } });
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(change).not.toHaveBeenCalled();
  expect(
    screen.getByText(old.character_version_id, { exact: false }),
  ).toBeTruthy();
  api.resolve.mockResolvedValueOnce({
    character_id: id(6),
    character_version_id: id(9),
    name: "新确认角色",
    aliases: [],
  });
  fireEvent.change(select, { target: { value: id(6) } });
  await waitFor(() =>
    expect(change).toHaveBeenCalledWith({
      character_id: id(6),
      character_version_id: id(9),
    }),
  );
});
it("切换项目卸载旧scope，延迟成功不能更新新项目行或释放私有候选", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  let resolve!: (value: unknown) => void;
  api.resolve.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const pending = vi.fn();
  const { change, rerender } = mount(client, { onPendingChange: pending });
  fireEvent.click(screen.getByRole("button", { name: "场景1行1选择正式角色" }));
  await screen.findByRole("option", { name: /合成确认角色/ });
  fireEvent.change(
    screen.getByRole("combobox", { name: "场景1行1正式确认角色" }),
    { target: { value: id(6) } },
  );
  const signal = api.resolve.mock.calls[0][2] as AbortSignal;
  rerender(
    <QueryClientProvider client={client}>
      <CharacterBindingPicker
        scope={{ ...scope, projectId: id(9) }}
        label="场景1行1"
        binding={undefined}
        locked={false}
        onChange={change}
        onPendingChange={pending}
      />
    </QueryClientProvider>,
  );
  expect(signal.aborted).toBe(true);
  expect(screen.queryByRole("option", { name: /合成确认角色/ })).toBeNull();
  await act(async () =>
    resolve({
      character_id: id(6),
      character_version_id: id(7),
      name: "角色",
      aliases: [],
    }),
  );
  expect(change).not.toHaveBeenCalled();
  expect(screen.getByText("未绑定正式角色")).toBeTruthy();
});
it("后续页权限失败隐藏原候选且保留原pin，重试不自动选择", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  api.list
    .mockResolvedValueOnce(page)
    .mockRejectedValueOnce(new Error("403当前项目不可读取"));
  const { change } = mount(client);
  fireEvent.click(screen.getByRole("button", { name: "场景1行1选择正式角色" }));
  await screen.findByRole("option", { name: /合成确认角色/ });
  fireEvent.click(screen.getByRole("button", { name: "读取更多确认角色" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByRole("option", { name: /合成确认角色/ })).toBeNull();
  expect(
    screen.getByText(old.character_version_id, { exact: false }),
  ).toBeTruthy();
  expect(change).not.toHaveBeenCalled();
});
