import { render } from "../../../tests/feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { StrictMode, useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { SpaceProvider, SpaceSwitcher, useSpace } from "./space-context";
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
  fireEvent.change(screen.getByLabelText("当前创作空间"), {
    target: { value: "team" },
  });
  await waitFor(() => expect(screen.getByText("team")).toBeTruthy());
  expect(screen.getByLabelText("私有草稿")).toHaveProperty("value", "");
  vi.mocked(request).mockResolvedValue({ items: [personal] });
  fireEvent.change(screen.getByLabelText("当前创作空间"), {
    target: { value: "personal" },
  });
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
  fireEvent.change(screen.getByLabelText("当前创作空间"), {
    target: { value: "team" },
  });
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
  fireEvent.change(screen.getByLabelText("当前创作空间"), {
    target: { value: "team" },
  });
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
