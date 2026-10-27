import { render } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { StrictMode, useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import {
  SpaceProvider,
  SpaceSwitcher,
  useSpace,
} from "@/components/spaces/space-context";
import { request, setCreationScope } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
  subscribeScopeDenied: (fn: () => void) => {
    mocks.denied = fn;
    return () => {};
  },
}));
const mocks = vi.hoisted(() => ({
  clear: vi.fn().mockResolvedValue(undefined),
  denied: () => {},
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: "actor" } }),
  clearPrivateResources: mocks.clear,
}));
const personal = {
  id: "personal",
  kind: "personal",
  name: "个人空间",
  role: "owner",
  writable: true,
  team_id: "",
};
const team = {
  id: "team",
  kind: "team",
  name: "团队空间",
  role: "member",
  writable: true,
  team_id: "team-id",
};
function Child() {
  const [value, setValue] = useState("");
  const space = useSpace();
  return (
    <>
      <SpaceSwitcher />
      <input
        aria-label="私有草稿"
        value={value}
        onChange={(e) => setValue(e.target.value)}
      />
      <span>{space.selected?.id}</span>
    </>
  );
}
async function chooseSpace(value: "team" | "personal") {
  const trigger = screen.getByRole("combobox", { name: "当前创作空间" });
  await waitFor(() => expect(trigger).toHaveProperty("disabled", false));
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  fireEvent.click(
    await screen.findByRole("option", {
      name: value === "team" ? /团队空间/ : /个人空间/,
    }),
  );
}
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
  mocks.clear.mockResolvedValue(undefined);
  setCreationScope(null);
});
it("StrictMode 初始化、切换清除私有视图并拒绝失效团队选择", async () => {
  vi.mocked(request).mockResolvedValue({ items: [personal, team] });
  render(
    <StrictMode>
      <SpaceProvider>
        <Child />
      </SpaceProvider>
    </StrictMode>,
  );
  await screen.findByRole("textbox", { name: "私有草稿" });
  fireEvent.change(screen.getByLabelText("私有草稿"), {
    target: { value: "old secret" },
  });
  await chooseSpace("team");
  await waitFor(() => expect(screen.getByText("team")).toBeTruthy());
  expect(screen.getByLabelText("私有草稿")).toHaveProperty("value", "");
  vi.mocked(request).mockResolvedValue({ items: [personal] });
  await chooseSpace("personal");
  await screen.findByText("personal");
  expect(mocks.clear).toHaveBeenCalled();
});
it("403 重新确认空间后只在真实成员丢失时回个人空间", async () => {
  let items = [personal, team];
  vi.mocked(request).mockImplementation(async () => ({ items }));
  render(
    <SpaceProvider>
      <Child />
    </SpaceProvider>,
  );
  await screen.findByLabelText("私有草稿");
  await chooseSpace("team");
  await screen.findByText("team");
  const triggerScopeDenied = () => mocks.denied();
  triggerScopeDenied();
  await waitFor(() => expect(request).toHaveBeenCalledTimes(3));
  expect(screen.getByText("team")).toBeTruthy();
  items = [personal];
  triggerScopeDenied();
  await screen.findByText("personal");
  expect(await screen.findByText(/已返回个人空间/)).toBeTruthy();
});

it("同一空间失去管理员角色也清除旧私有视图", async () => {
  let items = [personal, { ...team, role: "team_admin" }];
  vi.mocked(request).mockImplementation(async () => ({ items }));
  render(
    <SpaceProvider>
      <Child />
    </SpaceProvider>,
  );
  await screen.findByLabelText("私有草稿");
  await chooseSpace("team");
  await screen.findByText("team");
  fireEvent.change(screen.getByLabelText("私有草稿"), {
    target: { value: "admin private" },
  });
  items = [personal, { ...team, role: "member" }];
  mocks.denied();
  await waitFor(() =>
    expect(screen.getByLabelText("私有草稿")).toHaveProperty("value", ""),
  );
});

it("同账号首次建立空间及重新挂载不清除本地恢复分支", async () => {
  vi.mocked(request).mockResolvedValue({ items: [personal] });
  const view = render(
    <SpaceProvider>
      <Child />
    </SpaceProvider>,
  );
  await screen.findByText("personal");
  expect(mocks.clear).not.toHaveBeenCalled();
  view.unmount();
  render(
    <SpaceProvider>
      <Child />
    </SpaceProvider>,
  );
  await screen.findByText("personal");
  expect(mocks.clear).not.toHaveBeenCalled();
});

it("business viewer403 revalidates once and retains readable team and draft", async () => {
  vi.mocked(request).mockResolvedValue({
    items: [personal, { ...team, role: "viewer", writable: false }],
  });
  render(
    <SpaceProvider>
      <Child />
    </SpaceProvider>,
  );
  await screen.findByLabelText("私有草稿");
  await chooseSpace("team");
  await screen.findByText("team");
  const draft = screen.getByLabelText("私有草稿");
  fireEvent.change(draft, { target: { value: "viewer idea" } });
  const count = vi.mocked(request).mock.calls.length;
  const clears = mocks.clear.mock.calls.length;
  mocks.denied();
  mocks.denied();
  await waitFor(() => expect(request).toHaveBeenCalledTimes(count + 1));
  expect(screen.getByText("team")).toBeTruthy();
  expect(screen.getByLabelText("私有草稿")).toBe(draft);
  expect(draft).toHaveProperty("value", "viewer idea");
  expect(mocks.clear).toHaveBeenCalledTimes(clears);
});
