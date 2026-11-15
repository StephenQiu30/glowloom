import { pageBusinessMemory } from "@/lib/page-business-memory";
import { render as renderView } from "../../../../feedback";
import { SWRConfig } from "swr";
import type { ReactNode } from "react";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
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
  const view = render(<ConnectionDetail id="connection" onSaved={onSaved} />);
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
  render(<ConnectionDetail id="connection" onSaved={onSaved} />);
  fireEvent.click(await screen.findByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(api.receipt).toHaveBeenCalledOnce());
  expect(api.save).toHaveBeenCalledOnce();
  expect(onSaved).toHaveBeenCalledOnce();
  expect(pageBusinessMemory.length).toBe(0);
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
  render(<ConnectionDetail id="connection" onSaved={onSaved} />);
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
  render(<ConnectionDetail id="connection" onSaved={() => {}} />);
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
  render(<ConnectionDetail id="connection" onSaved={onSaved} />);
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

it("接入模型使用已读取的连接与凭据，不跳转或发起验证", async () => {
  const onAddModel = vi.fn();
  api.detail.mockResolvedValue({
    provider: {
      id: "connection",
      name: "DeepSeek",
      adapter_key: "deepseek",
      status: "active",
    },
    credential: { id: "credential", status: "active" },
  });
  render(
    <ConnectionDetail
      id="connection"

      onSaved={() => {}}
      onAddModel={onAddModel}
    />,
  );
  const trigger = await screen.findByRole("button", { name: "接入模型" });
  fireEvent.click(trigger);
  expect(onAddModel).toHaveBeenCalledWith(
    expect.objectContaining({
      provider: expect.objectContaining({ id: "connection" }),
      credential: expect.objectContaining({ id: "credential" }),
    }),
    trigger,
  );
  expect(api.config).not.toHaveBeenCalled();
  expect(api.save).not.toHaveBeenCalled();
});

it("模型保存刷新连接时保留原触发按钮，避免返回焦点丢失", async () => {
  const detail = {
    provider: {
      id: "connection",
      name: "DeepSeek",
      adapter_key: "deepseek",
      status: "active",
    },
    credential: { id: "credential", status: "active" },
  };
  api.detail.mockResolvedValue(detail);
  const cache = new Map();
  const value = { provider: () => cache, dedupingInterval: 0 };
  const onAddModel = vi.fn();
  const node = (revision: number) => (
    <SWRConfig value={value}>
      <ConnectionDetail
        id="connection"
        revision={revision}

        onSaved={() => {}}
        onAddModel={onAddModel}
      />
    </SWRConfig>
  );
  const view = renderView(node(0));
  const trigger = await screen.findByRole("button", { name: "接入模型" });
  let finish!: (result: typeof detail) => void;
  api.detail.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  view.rerender(node(1));
  await waitFor(() => expect(api.detail).toHaveBeenCalledTimes(2));
  expect(screen.getByRole("button", { name: "接入模型" })).toBe(trigger);
  await act(async () => finish(detail));
  expect(screen.getByRole("button", { name: "接入模型" })).toBe(trigger);
});

it.each(["disabled", "active"])(
  "Switch 从 %s 状态提交启停并显示服务器回写",
  async (status) => {
    const next = status === "active" ? "disabled" : "active";
    api.detail.mockResolvedValue({
      provider: { id: "connection", name: "OpenRouter", status, revision: 3 },
      credential: { id: "credential", status: "active" },
    });
    api.update.mockImplementation(async () => {
      api.detail.mockResolvedValue({
        provider: {
          id: "connection",
          name: "OpenRouter",
          status: next,
          revision: 4,
        },
        credential: { id: "credential", status: "active" },
      });
      return { id: "connection", status: next, revision: 4 };
    });
    render(<ConnectionDetail id="connection" onSaved={() => {}} />);
    const control = await screen.findByRole("switch", { name: "启用连接" });
    expect(control.getAttribute("aria-checked")).toBe(
      String(status === "active"),
    );
    expect(screen.queryByRole("button", { name: "连接诊断" })).toBeNull();
    expect(screen.queryByRole("button", { name: "连接设置" })).toBeNull();
    fireEvent.click(control);
    await waitFor(() => expect(api.update).toHaveBeenCalledOnce());
    expect(api.update).toHaveBeenCalledWith(
      { id: "connection" },
      { expected_revision: 3, status: next },
      { headers: { "Idempotency-Key": expect.any(String) } },
    );
    await waitFor(() =>
      expect(control.getAttribute("aria-checked")).toBe(
        String(next === "active"),
      ),
    );
    expect(api.config).not.toHaveBeenCalled();
    expect(api.keys).not.toHaveBeenCalled();
  },
);

it("启停保存期间阻止重复请求，失败不伪造新状态", async () => {
  api.detail.mockResolvedValue({
    provider: {
      id: "connection",
      name: "OpenRouter",
      status: "active",
      revision: 3,
    },
    credential: { id: "credential", status: "active" },
  });
  let reject!: (error: unknown) => void;
  api.update.mockImplementation(
    () =>
      new Promise((_, fail) => {
        reject = fail;
      }),
  );
  render(<ConnectionDetail id="connection" onSaved={() => {}} />);
  const control = await screen.findByRole("switch", { name: "启用连接" });
  fireEvent.click(control);
  await waitFor(() => expect(control.hasAttribute("disabled")).toBe(true));
  fireEvent.click(control);
  expect(api.update).toHaveBeenCalledOnce();
  expect(control.getAttribute("aria-checked")).toBe("true");
  await act(async () => reject(new RequestError("revision_conflict", 409)));
  expect(control.getAttribute("aria-checked")).toBe("true");
});
