// @vitest-environment jsdom

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  connectProviderPreset,
  getConfigReceipt,
  readPresetModelDirectory,
  setAdminCredential,
} from "@/api/settings";
import { RequestError } from "@/lib/request";
import { PresetConnect } from "@/app/providers/_components/preset-connect";

vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ session_id: "wizard-test" }),
}));

vi.mock("@/api/settings", () => ({
  connectProviderPreset: vi.fn(),
  getConfigReceipt: vi.fn(),
  readPresetModelDirectory: vi.fn(),
  setAdminCredential: vi.fn(),
}));

const presets: API.Preset[] = [
  {
    id: "vendor-one",
    version_id: "version-one",
    document: {
      name: "供应商一",
      config: { adapter_key: "openai", capabilities: ["text"] },
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

const connect = vi.mocked(connectProviderPreset);
const receipt = vi.mocked(getConfigReceipt);
const credential = vi.mocked(setAdminCredential);
const directory = vi.mocked(readPresetModelDirectory);

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function openWizard() {
  const saved = vi.fn();
  const cancelled = vi.fn();
  render(
    <PresetConnect
      presets={presets}
      initialPreset="vendor-one"
      adapters={[
        { adapter_key: "openai", check_capabilities: ["model_discovery"] },
      ]}
      onCancel={cancelled}
      onCreated={vi.fn()}
      onSaved={saved}
    />,
  );
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-secret" },
  });
  return { saved, cancelled };
}

describe("two-step provider connection", () => {
  it("keeps the key and model selection across steps and writes only on save", async () => {
    connect.mockResolvedValue({ provider_id: "connection-one" });
    credential.mockResolvedValue({
      id: "key-one",
      provider_id: "connection-one",
      status: "active",
    });
    const { saved } = openWizard();

    fireEvent.click(screen.getByRole("button", { name: "查看预置模型" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "基础模型" }));
    fireEvent.click(screen.getByRole("button", { name: "上一步" }));
    expect((screen.getByLabelText("API Key") as HTMLInputElement).value).toBe(
      "fixture-secret",
    );
    expect(connect).not.toHaveBeenCalled();
    expect(credential).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "查看预置模型" }));
    fireEvent.click(screen.getByRole("button", { name: "保存接入" }));

    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect.mock.calls[0]?.[1]).toMatchObject({
      version_id: "version-one",
      selected_models_only: true,
      catalog_version: "catalog-one",
      candidate_keys: ["basic"],
    });
    expect(credential.mock.calls[0]?.[1]).toMatchObject({
      secret: { api_key: "fixture-secret" },
    });
  });

  it("clears the old key when switching providers and cancels without saving", () => {
    const { cancelled } = openWizard();
    fireEvent.change(screen.getByLabelText("供应商"), {
      target: { value: "vendor-two" },
    });
    expect((screen.getByLabelText("API Key") as HTMLInputElement).value).toBe(
      "",
    );
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(cancelled).toHaveBeenCalledOnce();
    expect(connect).not.toHaveBeenCalled();
  });

  it("checks the original credential receipt without creating another connection", async () => {
    connect.mockResolvedValue({ provider_id: "connection-one" });
    credential.mockRejectedValueOnce(new Error("response lost"));
    const { saved } = openWizard();

    fireEvent.click(screen.getByRole("button", { name: "查看预置模型" }));
    fireEvent.click(screen.getByRole("button", { name: "仅保存连接" }));
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

    expect(receipt).toHaveBeenCalledWith({
      id: requestID,
      action: "credential.set",
      target_id: "connection-one",
    });
    expect(connect).toHaveBeenCalledOnce();
    expect(credential).toHaveBeenCalledOnce();
  });

  it("reads the directory before saving and stores only the explicitly added model", async () => {
    directory.mockResolvedValue({ model_ids: ["basic-1", "fresh-model"] });
    connect.mockResolvedValue({ provider_id: "connection-one" });
    credential.mockResolvedValue({
      id: "key-one",
      provider_id: "connection-one",
      status: "active",
    });
    const { saved } = openWizard();

    fireEvent.click(screen.getByRole("button", { name: "读取模型" }));
    fireEvent.click(await screen.findByRole("button", { name: "fresh-model" }));
    fireEvent.click(screen.getByRole("button", { name: "加入所选" }));

    expect(directory).toHaveBeenCalledWith(
      { id: "vendor-one" },
      {
        version_id: "version-one",
        api_key: "fixture-secret",
      },
    );
    expect(connect).not.toHaveBeenCalled();
    expect(credential).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "保存接入" }));

    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect.mock.calls[0]?.[1]).toMatchObject({
      candidate_keys: [],
      custom_candidates: [
        { provider_model_id: "fresh-model", capability: "text.generate" },
      ],
    });
  });

  it("offers preset selection when directory access is rejected", async () => {
    directory.mockRejectedValue(
      new RequestError("model_directory_credential_rejected", 422),
    );
    openWizard();
    fireEvent.click(screen.getByRole("button", { name: "读取模型" }));
    await screen.findByText(/暂时无法读取目录/);
    fireEvent.click(screen.getByRole("button", { name: "查看预置模型" }));
    expect(
      screen
        .getByRole("checkbox", { name: "基础模型" })
        .getAttribute("aria-checked"),
    ).toBe("false");
    expect(connect).not.toHaveBeenCalled();
  });

  it("checks an uncertain create before ever submitting the credential", async () => {
    connect.mockRejectedValueOnce(new Error("response lost"));
    credential.mockResolvedValue({
      id: "key-one",
      provider_id: "connection-one",
      status: "active",
    });
    const { saved } = openWizard();
    fireEvent.click(screen.getByRole("button", { name: "查看预置模型" }));
    fireEvent.click(screen.getByRole("button", { name: "仅保存连接" }));
    await screen.findByText(/接入结果待确认/);

    const createID = connect.mock.calls[0]?.[1]?.request_id;
    receipt
      .mockResolvedValueOnce({
        request_id: createID,
        action: "preset.connect",
        target_id: "vendor-one",
        result: { provider_id: "connection-one" },
      })
      .mockRejectedValueOnce(new RequestError("not_found", 404));
    expect(credential).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "核对并继续保存" }));

    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect).toHaveBeenCalledOnce();
    expect(credential).toHaveBeenCalledOnce();
  });

  it("recovers a late create after reopening the wizard without carrying its password", async () => {
    let complete: (result: API.PresetConnection) => void = () => {};
    connect.mockReturnValueOnce(
      new Promise((resolve) => {
        complete = resolve;
      }),
    );
    receipt.mockRejectedValue(new RequestError("not_found", 404));
    credential.mockResolvedValue({
      id: "key-one",
      provider_id: "connection-one",
      status: "active",
    });
    const previous = openWizard();
    fireEvent.click(screen.getByRole("button", { name: "查看预置模型" }));
    fireEvent.click(screen.getByRole("button", { name: "仅保存连接" }));
    cleanup();
    await act(async () => complete({ provider_id: "connection-one" }));
    expect(credential).not.toHaveBeenCalled();
    expect(previous.saved).not.toHaveBeenCalled();

    const saved = vi.fn();
    render(
      <PresetConnect
        presets={presets}
        initialPreset="vendor-two"
        onCreated={vi.fn()}
        onSaved={saved}
      />,
    );
    const key = screen.getByLabelText("API Key") as HTMLInputElement;
    expect(key.value).toBe("");
    expect((screen.getByLabelText("供应商") as HTMLSelectElement).value).toBe(
      "vendor-one",
    );
    fireEvent.change(key, { target: { value: "fresh-fixture-key" } });
    fireEvent.click(screen.getByRole("button", { name: "核对并继续保存" }));
    await waitFor(() => expect(saved).toHaveBeenCalledWith("connection-one"));
    expect(connect).toHaveBeenCalledOnce();
    expect(credential.mock.calls[0]?.[1]?.secret).toEqual({
      api_key: "fresh-fixture-key",
    });
  });

  it("keeps AK and SK separate and does not offer an incompatible directory action", async () => {
    connect.mockResolvedValue({ provider_id: "music-connection" });
    credential.mockResolvedValue({
      id: "music-key",
      provider_id: "music-connection",
      status: "active",
    });
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
    expect(screen.queryByRole("button", { name: "读取模型" })).toBeNull();
    fireEvent.change(screen.getByLabelText("Access Key ID"), {
      target: { value: "fixture-ak" },
    });
    fireEvent.change(screen.getByLabelText("Secret Access Key"), {
      target: { value: "fixture-sk" },
    });
    fireEvent.click(screen.getByRole("button", { name: "手动添加模型" }));
    fireEvent.click(screen.getByRole("button", { name: "仅保存连接" }));
    await waitFor(() => expect(saved).toHaveBeenCalledWith("music-connection"));
    expect(credential.mock.calls[0]?.[1]?.secret).toEqual({
      access_key_id: "fixture-ak",
      secret_access_key: "fixture-sk",
    });
  });
});
