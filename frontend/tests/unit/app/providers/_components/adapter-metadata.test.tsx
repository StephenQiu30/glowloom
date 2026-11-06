import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { PresetEditor } from "@/app/providers/_components/preset-editor";
import { PresetConnect } from "@/app/providers/_components/preset-connect";
import { ProviderLogo } from "@/app/providers/_components/provider-logo";
import { providerLabel } from "@/app/providers/_components/provider-label";
import { pageBusinessMemory } from "@/lib/page-business-memory";
const api = vi.hoisted(() => ({
  preview: vi.fn(),
  save: vi.fn(),
  version: vi.fn(),
  connect: vi.fn(),
  credential: vi.fn(),
  test: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin-metadata", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  previewProviderPreset: api.preview,
  createProviderPreset: api.save,
  publishProviderPresetVersion: api.version,
  connectProviderPreset: api.connect,
  setAdminCredential: api.credential,
  testAdminCredential: api.test,
  getConfigReceipt: vi.fn(),
}));
vi.mock("@/app/providers/_components/preset-models", () => ({
  PresetModels: () => null,
}));
const metadata: API.AdapterMetadata = {
  adapter_key: "deepseek",
  display_name: "目录里的服务名",
  logo_key: "unknown-local-logo",
  capabilities: ["text"],
  protocol_ids: ["deepseek"],
  credential_fields: [{ name: "api_key", type: "password", required: true }],
  check_capabilities: ["credential", "model_test"],
};
const preset: API.Preset = {
  id: "preset",
  version_id: "version",
  origin: "builtin",
  document: {
    schema_version: 1,
    name: "我的文本服务",
    config: {
      adapter_key: "deepseek",
      base_url: "https://api.deepseek.com",
      region: "domestic",
      capabilities: ["text"],
      timeout_seconds: 120,
      concurrency_limit: 2,
      rate_limit_per_min: 30,
    },
    model_templates: [{ name: "Text" }],
  },
};
beforeEach(() => {
  vi.clearAllMocks();
  pageBusinessMemory.clear();
  api.preview.mockImplementation(async ({ document_json }) => ({
    document: JSON.parse(document_json),
    hash: "checked",
  }));
  api.save.mockResolvedValue({ id: "saved" });
  api.connect.mockResolvedValue({ provider_id: "connection" });
  api.credential.mockResolvedValue({
    id: "credential",
    provider_id: "connection",
    status: "active",
    last4: "test",
  });
});
afterEach(cleanup);
it("安全元数据决定名称且未知图标有可读文字回退", () => {
  expect(providerLabel("deepseek", [metadata])).toBe("目录里的服务名");
  render(<ProviderLogo adapterKey="deepseek" metadata={metadata} />);
  expect(screen.getByText("目录里的服务名")).toBeTruthy();
  expect(screen.queryByRole("img")).toBeNull();
});
it("表单能力及 schema-v1 声明来自安全目录，预览和保存不触发验证", async () => {
  render(
    <PresetEditor
      presets={[preset]}
      adapters={[metadata]}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "高级设置" }));
  expect(screen.getByLabelText("文本")).toBeTruthy();
  expect(screen.queryByLabelText("视频")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "校验并预览" }));
  const save = screen.getByRole("button", {
    name: "保存新版本",
  }) as HTMLButtonElement;
  await waitFor(() => expect(save.disabled).toBe(false));
  const document = JSON.parse(api.preview.mock.calls[0][0].document_json);
  expect(document.adapter_declaration).toEqual({
    schema_version: 1,
    adapter_key: "deepseek",
    protocol_id: "deepseek",
    base_url: "https://api.deepseek.com",
    region: "domestic",
    capabilities: ["text"],
  });
  expect(JSON.stringify(document)).not.toContain("credential_fields");
  fireEvent.click(save);
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  expect(api.test).not.toHaveBeenCalled();
});
it("凭据输入使用安全字段，保存只建立连接与凭据", async () => {
  const saved = vi.fn();
  render(
    <PresetConnect
      presets={[preset]}
      adapters={[metadata]}
      onCreated={vi.fn()}
      onSaved={saved}
    />,
  );
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-secret-test" },
  });
  fireEvent.click(screen.getByRole("button", { name: "手动添加模型" }));
  fireEvent.click(screen.getByRole("button", { name: "仅保存连接" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.test).not.toHaveBeenCalled();
  expect(api.credential).toHaveBeenCalledWith(
    { id: "connection" },
    { label: "平台凭据", secret: { api_key: "fixture-secret-test" } },
    expect.anything(),
  );
});

it("离开原连接后，迟到的创建响应不保存 Key 或归属到新连接", async () => {
  let resolve!: (result: { provider_id: string }) => void;
  api.connect.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const oldSaved = vi.fn();
  const old = render(
    <PresetConnect
      presets={[preset]}
      adapters={[metadata]}
      onCreated={vi.fn()}
      onSaved={oldSaved}
    />,
  );
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "old-key-test" },
  });
  fireEvent.click(screen.getByRole("button", { name: "手动添加模型" }));
  fireEvent.click(screen.getByRole("button", { name: "仅保存连接" }));
  await waitFor(() => expect(api.connect).toHaveBeenCalledOnce());
  old.unmount();
  const nextSaved = vi.fn();
  render(
    <PresetConnect
      presets={[preset]}
      adapters={[metadata]}
      onCreated={vi.fn()}
      onSaved={nextSaved}
    />,
  );
  await act(async () => {
    resolve({ provider_id: "old-connection" });
  });
  await waitFor(() => expect(api.credential).not.toHaveBeenCalled());
  expect(oldSaved).not.toHaveBeenCalled();
  expect(nextSaved).not.toHaveBeenCalled();
});
