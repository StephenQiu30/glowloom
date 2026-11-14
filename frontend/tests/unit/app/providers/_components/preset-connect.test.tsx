// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  connectProviderPreset,
  createCustomProviderConnection,
  getConfigReceipt,
  setAdminCredential,
  readPresetModelDirectory,
} from "@/api/settings";
import { RequestError } from "@/lib/request";
import { PresetConnect } from "@/app/providers/_components/preset-connect";
let sessionID = "";
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ session_id: sessionID, actor: { id: sessionID } }),
}));
vi.mock("@/api/settings", () => ({
  connectProviderPreset: vi.fn(),
  createCustomProviderConnection: vi.fn(),
  getConfigReceipt: vi.fn(),
  setAdminCredential: vi.fn(),
  readPresetModelDirectory: vi.fn(),
}));
vi.mock("@/api/media", () => ({
  uploadProviderLogo: vi.fn(),
  getProviderLogo: vi.fn(),
}));
const presets: API.Preset[] = [
  {
    id: "vendor-one",
    version_id: "version-one",
    document: {
      name: "供应商一",
      config: {
        adapter_key: "openai",
        base_url: "https://api.example.com/v1",
        capabilities: ["text", "image"],
      },
    },
    model_catalog: {
      version: "catalog-one",
      models: [
        {
          key: "basic",
          display_name: "基础模型",
          provider_model_id: "basic-1",
          capability: "text.generate",
        },
      ],
    },
  },
  {
    id: "vendor-two",
    version_id: "version-two",
    document: { name: "供应商二", config: { adapter_key: "deepseek" } },
  },
];
const connect = vi.mocked(connectProviderPreset),
  custom = vi.mocked(createCustomProviderConnection),
  receipt = vi.mocked(getConfigReceipt),
  credential = vi.mocked(setAdminCredential);
beforeEach(() => {
  sessionID = crypto.randomUUID();
  receipt.mockRejectedValue(new RequestError("not_found", 404));
  credential.mockResolvedValue({
    id: "key-one",
    provider_id: "connection-one",
    status: "active",
  });
  connect.mockResolvedValue({ provider_id: "connection-one" });
  custom.mockResolvedValue({ provider_id: "connection-one" });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
function openWizard() {
  const saved = vi.fn(),
    cancelled = vi.fn();
  const view = render(
    <PresetConnect
      presets={presets}
      initialPreset="vendor-one"
      onCancel={cancelled}
      onCreated={vi.fn()}
      onSaved={saved}
    />,
  );
  return { saved, cancelled, ...view };
}
function key(value = "fixture-secret") {
  fireEvent.change(screen.getByLabelText("API Key"), { target: { value } });
}
function save() {
  fireEvent.click(screen.getByRole("button", { name: "保存供应商" }));
}
describe("supplier-first connection", () => {
  it("saves a named supplier first without selecting models or reading a directory", async () => {
    const { saved } = openWizard();
    key();
    expect(screen.queryByRole("button", { name: "选择模型" })).toBeNull();
    expect(screen.queryByLabelText("手动添加模型 ID")).toBeNull();
    expect(screen.queryByRole("checkbox", { name: "基础模型" })).toBeNull();
    expect(connect).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("名称"), {
      target: { value: "团队供应商" },
    });
    save();
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect.mock.calls[0]?.[1]).toMatchObject({
      name: "团队供应商",
      version_id: "version-one",
      selected_models_only: true,
      candidate_keys: [],
      custom_candidates: [],
    });
    expect(credential.mock.calls[0]?.[1]?.secret).toEqual({
      api_key: "fixture-secret",
    });
    expect(readPresetModelDirectory).not.toHaveBeenCalled();
  });
  it("offers custom name, protocol, address and key in the same dialog", async () => {
    const { saved } = openWizard();
    fireEvent.mouseDown(screen.getByRole("tab", { name: "自定义" }), {
      button: 0,
      ctrlKey: false,
    });
    fireEvent.change(screen.getByLabelText("名称"), {
      target: { value: "团队网关" },
    });
    expect(screen.getByLabelText("协议").textContent).toContain("OpenAI 兼容");
    fireEvent.change(screen.getByLabelText("请求地址"), {
      target: { value: "https://gateway.example.com/v1" },
    });
    key();
    save();
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(custom.mock.calls[0]?.[0]).toMatchObject({
      name: "团队网关",
      config: {
        adapter_key: "openai",
        base_url: "https://gateway.example.com/v1",
      },
    });
    expect(connect).not.toHaveBeenCalled();
  });
  it("switching provider clears its key and cancellation writes nothing", async () => {
    const { cancelled } = openWizard();
    key();
    fireEvent.keyDown(screen.getByRole("combobox", { name: "供应商" }), {
      key: "ArrowDown",
    });
    fireEvent.keyDown(await screen.findByRole("option", { name: "供应商二" }), {
      key: "Enter",
    });
    expect((screen.getByLabelText("API Key") as HTMLInputElement).value).toBe(
      "",
    );
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(cancelled).toHaveBeenCalledOnce();
    expect(connect).not.toHaveBeenCalled();
    expect(custom).not.toHaveBeenCalled();
  });
  it("recovers the original credential receipt without another supplier", async () => {
    credential.mockRejectedValueOnce(new Error("response lost"));
    const { saved } = openWizard();
    key();
    save();
    const retry = await screen.findByRole("button", { name: "核对并继续保存" });
    const requestID =
      credential.mock.calls[0]?.[2]?.headers?.["Idempotency-Key"];
    receipt.mockResolvedValue({
      request_id: String(requestID),
      action: "credential.set",
      target_id: "connection-one",
      result: {
        id: "key-one",
        provider_id: "connection-one",
        status: "active",
      },
    });
    fireEvent.click(retry);
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect).toHaveBeenCalledOnce();
    expect(credential).toHaveBeenCalledOnce();
  });
  it("checks uncertain creation before credential writes and freezes the request", async () => {
    connect.mockRejectedValueOnce(new Error("response lost"));
    const { saved } = openWizard();
    key();
    save();
    await screen.findByText(/接入结果待确认/);
    const input = connect.mock.calls[0]?.[1];
    expect((screen.getByLabelText("名称") as HTMLInputElement).disabled).toBe(
      true,
    );
    receipt
      .mockResolvedValueOnce({
        request_id: input?.request_id,
        action: "preset.connect",
        target_id: "vendor-one",
        result: { provider_id: "connection-one" },
      })
      .mockRejectedValueOnce(new RequestError("not_found", 404));
    expect(credential).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "核对并继续保存" }));
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect).toHaveBeenCalledOnce();
  });
  it("restores a late create after reopening without retaining a secret", async () => {
    let complete: (result: API.PresetConnection) => void = () => {};
    connect.mockReturnValueOnce(
      new Promise((resolve) => {
        complete = resolve;
      }),
    );
    const previous = openWizard();
    key();
    save();
    cleanup();
    await act(async () => complete({ provider_id: "connection-one" }));
    expect(credential).not.toHaveBeenCalled();
    expect(previous.saved).not.toHaveBeenCalled();
    const { saved } = openWizard();
    expect((screen.getByLabelText("API Key") as HTMLInputElement).value).toBe(
      "",
    );
    key("fresh-fixture-key");
    fireEvent.click(screen.getByRole("button", { name: "核对并继续保存" }));
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect).toHaveBeenCalledOnce();
    expect(credential.mock.calls[0]?.[1]?.secret).toEqual({
      api_key: "fresh-fixture-key",
    });
  });
  it("does not reuse another login session's pending supplier", async () => {
    connect.mockRejectedValueOnce(new Error("response lost"));
    openWizard();
    key();
    save();
    await screen.findByText(/接入结果待确认/);
    cleanup();
    sessionID = crypto.randomUUID();
    openWizard();
    expect(screen.queryByRole("button", { name: "核对并继续保存" })).toBeNull();
    expect((screen.getByLabelText("API Key") as HTMLInputElement).value).toBe(
      "",
    );
  });
  it("recovers custom creation using the original receipt and nonsecret input", async () => {
    custom.mockRejectedValueOnce(new Error("response lost"));
    openWizard();
    fireEvent.mouseDown(screen.getByRole("tab", { name: "自定义" }), {
      button: 0,
      ctrlKey: false,
    });
    fireEvent.change(screen.getByLabelText("名称"), {
      target: { value: "团队网关" },
    });
    fireEvent.change(screen.getByLabelText("请求地址"), {
      target: { value: "https://gateway.example.com/v1" },
    });
    key();
    save();
    await screen.findByText(/接入结果待确认/);
    const original = custom.mock.calls[0]?.[0];
    cleanup();
    const { saved } = openWizard();
    key();
    expect(
      screen.getByRole("tab", { name: "自定义" }).getAttribute("aria-selected"),
    ).toBe("true");
    receipt
      .mockResolvedValueOnce({
        request_id: original?.request_id,
        action: "provider.custom.connect",
        target_id: "00000000-0000-0000-0000-000000000000",
        result: { provider_id: "connection-one" },
      })
      .mockRejectedValueOnce(new RequestError("not_found", 404));
    fireEvent.click(screen.getByRole("button", { name: "核对并继续保存" }));
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(custom).toHaveBeenCalledOnce();
  });
  it("keeps AK and SK separate for a registered supplier protocol", async () => {
    const saved = vi.fn();
    render(
      <PresetConnect
        presets={[
          {
            id: "music",
            version_id: "music-version",
            document: {
              name: "音乐",
              config: { adapter_key: "volcengine_music" },
            },
          },
        ]}
        adapters={[
          {
            adapter_key: "volcengine_music",
            credential_fields: [
              { name: "access_key_id", required: true },
              { name: "secret_access_key", required: true },
            ],
          },
        ]}
        onCreated={vi.fn()}
        onSaved={saved}
      />,
    );
    fireEvent.change(screen.getByLabelText("Access Key ID"), {
      target: { value: "fixture-ak" },
    });
    fireEvent.change(screen.getByLabelText("Secret Access Key"), {
      target: { value: "fixture-sk" },
    });
    save();
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(credential.mock.calls[0]?.[1]?.secret).toEqual({
      access_key_id: "fixture-ak",
      secret_access_key: "fixture-sk",
    });
  });
});

it("does not replay a create whose receipt has a malformed result", async () => {
  connect.mockRejectedValueOnce(new Error("response lost"));
  openWizard();
  key();
  save();
  await screen.findByText(/接入结果待确认/);
  const request = connect.mock.calls[0][1].request_id;
  receipt.mockResolvedValue({
    request_id: request,
    action: "preset.connect",
    target_id: "vendor-one",
    result: { unexpected: true },
  });
  fireEvent.click(screen.getByRole("button", { name: "核对并继续保存" }));
  await waitFor(() => expect(receipt).toHaveBeenCalledOnce());
  expect(connect).toHaveBeenCalledOnce();
  expect(credential).not.toHaveBeenCalled();
});
