import { pageBusinessMemory } from "@/lib/page-business-memory";
import { render as renderView } from "../../../../feedback";
import { SWRConfig } from "swr";
import type { ReactNode } from "react";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ConnectionDetail } from "@/app/providers/_components/connection-detail";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  detail: vi.fn(),
  config: vi.fn(),
  keys: vi.fn(),
  save: vi.fn(),
  receipt: vi.fn(),
  disable: vi.fn(),
  update: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin-fixture", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  getAdminProvider: api.detail,
  getProviderConfig: api.config,
  listProviderCredentials: api.keys,
  setAdminCredential: api.save,
  getConfigReceipt: api.receipt,
  getProviderOnboarding: vi.fn().mockResolvedValue({ models: [] }),
  disableAdminCredential: api.disable,
  testAdminCredential: vi.fn(),
  getProviderCheck: vi.fn(),
  listProviderChecks: vi
    .fn()
    .mockResolvedValue({ items: [], current_checks: [] }),
  listProviderTestModels: vi.fn().mockResolvedValue([]),
  testProviderReachability: vi.fn(),
  previewProviderUpgrade: vi.fn(),
  applyProviderPreset: vi.fn(),
  updateAdminProvider: api.update,
}));
beforeEach(() => {
  vi.clearAllMocks();
  pageBusinessMemory.clear();
  api.detail.mockResolvedValue({
    provider: {
      id: "connection",
      name: "OpenRouter",
      region: "overseas",
      status: "disabled",
      revision: 1,
    },
  });
  api.config.mockResolvedValue({ id: "config", version: 1 });
  api.keys.mockResolvedValue([]);
});
afterEach(cleanup);
function render(node: ReactNode) {
  return renderView(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      {node}
    </SWRConfig>,
  );
}
it("Key 只进入提交请求，同页重新打开恢复只查询安全回执", async () => {
  api.save.mockRejectedValue(new RequestError("request_unconfirmed"));
  api.receipt.mockResolvedValue({ result: { id: "saved" } });
  const onSaved = vi.fn();
  const view = render(
    <ConnectionDetail id="connection" presets={[]} onSaved={onSaved} />,
  );
  const input = await screen.findByLabelText("API Key");
  expect(screen.queryByRole("tablist")).toBeNull();
  expect(api.config).not.toHaveBeenCalled();
  expect(api.keys).not.toHaveBeenCalled();
  fireEvent.change(input, {
    target: { value: "fixture-secret-do-not-persist" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存 API Key" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  expect((input as HTMLInputElement).value).toBe("");
  expect(api.save.mock.calls[0][1].secret.api_key).toBe(
    "fixture-secret-do-not-persist",
  );
  const stored = pageBusinessMemory.getItem(
    "glowloom.catalog.pending.v1.admin-fixture.connection.connection",
  );
  expect(stored).toContain("credential.set");
  expect(stored).not.toContain("fixture-secret");
  view.unmount();
  render(<ConnectionDetail id="connection" presets={[]} onSaved={onSaved} />);
  fireEvent.click(await screen.findByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(api.receipt).toHaveBeenCalledOnce());
  expect(api.save).toHaveBeenCalledOnce();
  expect(onSaved).toHaveBeenCalledOnce();
  expect(pageBusinessMemory.length).toBe(0);
});
it("旧 Key 可单独停用，当前 Key 不受影响", async () => {
  api.keys.mockResolvedValue([
    { id: "old-key", last4: "1234", status: "draining" },
    { id: "active-key", last4: "5678", status: "active" },
  ]);
  api.disable.mockResolvedValue({ id: "old-key", status: "disabled" });
  render(<ConnectionDetail id="connection" presets={[]} onSaved={() => {}} />);
  fireEvent.click(await screen.findByRole("button", { name: "连接设置" }));
  fireEvent.click(
    (await screen.findAllByRole("button", { name: "紧急停用" }))[0],
  );
  fireEvent.click(screen.getByRole("button", { name: "确认停用此 Key" }));
  await waitFor(() => expect(api.disable).toHaveBeenCalledOnce());
  expect(api.disable.mock.calls[0][0]).toEqual({
    id: "connection",
    credential_id: "old-key",
  });
});

it("向导的 Key 写入未确认时，详情禁止绕过并使用原回执恢复", async () => {
  pageBusinessMemory.setItem(
    "glowloom.catalog.pending.v1.admin-fixture.preset.onboarding.credential",
    JSON.stringify({
      id: "original-key-request",
      action: "credential.set",
      target: "connection",
    }),
  );
  api.receipt.mockResolvedValue({
    result: {
      id: "credential",
      provider_id: "connection",
      last4: "test",
      status: "active",
    },
  });
  const onSaved = vi.fn();
  render(<ConnectionDetail id="connection" presets={[]} onSaved={onSaved} />);
  const input = await screen.findByLabelText("API Key");
  expect((input as HTMLInputElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
  expect(api.save).not.toHaveBeenCalled();
});

it("已配置密钥只提供更换入口，轮换输入和记录不显示尾号", async () => {
  api.detail.mockResolvedValue({
    provider: { id: "connection", name: "连接", status: "disabled" },
    credential: { id: "key", status: "active", last4: "abcd" },
  });
  api.keys.mockResolvedValue([{ id: "key", status: "active", last4: "5678" }]);
  render(<ConnectionDetail id="connection" presets={[]} onSaved={() => {}} />);
  const replace = await screen.findByRole("button", { name: "更换 Key" });
  expect(screen.queryByText(/abcd|5678|••••/)).toBeNull();
  fireEvent.click(replace);
  const input = await screen.findByLabelText("API Key");
  expect(input.getAttribute("placeholder")).toBe("已配置，输入新密钥以更换");
  expect(screen.queryByText(/abcd|5678|••••/)).toBeNull();
});

it("在连接详情顶部重命名未启用的连接", async () => {
  api.update.mockResolvedValue({
    id: "connection",
    name: "OpenRouter 备用",
    revision: 2,
  });
  const onSaved = vi.fn();
  render(<ConnectionDetail id="connection" presets={[]} onSaved={onSaved} />);
  fireEvent.click(await screen.findByRole("button", { name: "编辑名称" }));
  const input = await screen.findByLabelText("连接名称");
  fireEvent.change(input, { target: { value: "OpenRouter 备用" } });
  fireEvent.click(screen.getByRole("button", { name: "保存连接名称" }));
  await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
  expect(api.update).toHaveBeenCalledWith(
    { id: "connection" },
    { name: "OpenRouter 备用", expected_revision: 1 },
    { headers: { "Idempotency-Key": expect.any(String) } },
  );
});
