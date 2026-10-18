import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../tests/feedback";
import { PresetConnect } from "./preset-connect";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  connect: vi.fn(),
  key: vi.fn(),
  receipt: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin-fixture", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  connectProviderPreset: api.connect,
  setAdminCredential: api.key,
  getConfigReceipt: api.receipt,
}));
const presets: API.Preset[] = [
  {
    id: "preset",
    version_id: "version",
    document: {
      name: "OpenRouter",
      model_templates: [{ name: "Reviewed text" }],
    },
  },
];
const created = vi.fn();
const saved = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  api.connect.mockResolvedValue({ provider_id: "connection" });
  api.key.mockResolvedValue({
    id: "credential",
    provider_id: "connection",
    last4: "test",
    status: "active",
  });
});
afterEach(cleanup);
function setup() {
  render(
    <PresetConnect presets={presets} onCreated={created} onSaved={saved} />,
  );
  fireEvent.change(screen.getByLabelText("平台预设"), {
    target: { value: "preset" },
  });
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存连接与 API Key" }));
}
it("选预设和 Key 即可保存，不要求名称、技术标识或 JSON", async () => {
  setup();
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledWith(
    { id: "preset" },
    expect.objectContaining({ version_id: "version", name: "" }),
  );
  expect(api.connect.mock.calls[0][1]).not.toHaveProperty("key");
  expect(api.key).toHaveBeenCalledWith(
    { id: "connection" },
    { label: "平台统一 Key", secret: { api_key: "fixture-api-key-test" } },
    expect.anything(),
  );
  expect(screen.getByLabelText("API Key").getAttribute("type")).toBe(
    "password",
  );
  expect((screen.getByLabelText("API Key") as HTMLInputElement).value).toBe("");
  expect(JSON.stringify(localStorage)).not.toContain("fixture-api-key-test");
});
it("Key 保存失败后继续写原连接，不重复创建", async () => {
  api.key.mockRejectedValueOnce(new RequestError("invalid_request", 422));
  setup();
  await waitFor(() => expect(api.key).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "继续保存 API Key",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "继续保存 API Key" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledOnce();
  expect(api.key).toHaveBeenCalledTimes(2);
});
it("创建回执丢失后查询原结果，不自动再次创建或补发 Key", async () => {
  api.connect.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockResolvedValue({ result: { provider_id: "connection" } });
  setup();
  await screen.findByRole("button", { name: "查询回执" });
  fireEvent.click(screen.getByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(created).toHaveBeenCalledWith("connection"));
  expect(api.connect).toHaveBeenCalledOnce();
  expect(api.key).not.toHaveBeenCalled();
  expect(JSON.stringify(localStorage)).not.toContain("fixture-api-key-test");
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  fireEvent.click(screen.getByRole("button", { name: "继续保存 API Key" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledOnce();
});
it("Key 写入未知时恢复只查回执，不再次写 Key", async () => {
  api.key.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockResolvedValue({
    result: {
      id: "credential",
      provider_id: "connection",
      status: "active",
      last4: "test",
    },
  });
  setup();
  await waitFor(() => expect(api.key).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "查询回执" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledOnce();
  expect(api.key).toHaveBeenCalledOnce();
});

it("错误连接的 Key 回执不能被认作保存成功", async () => {
  api.key.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockResolvedValue({
    result: {
      id: "credential",
      provider_id: "other",
      status: "active",
      last4: "test",
    },
  });
  setup();
  await waitFor(() => expect(api.key).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "查询回执" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(api.receipt).toHaveBeenCalledOnce());
  expect(saved).not.toHaveBeenCalled();
  expect(localStorage.length).toBeGreaterThan(0);
});

it("创建结果未知时先查回执，仅在未找到时重放原请求标识", async () => {
  api.connect.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockRejectedValueOnce(new RequestError("not_found", 404));
  setup();
  await waitFor(() => expect(api.connect).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "保存连接与 API Key",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存连接与 API Key" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.receipt).toHaveBeenCalledOnce();
  expect(api.connect).toHaveBeenCalledTimes(2);
  expect(api.connect.mock.calls[0][1].request_id).toBe(
    api.connect.mock.calls[1][1].request_id,
  );
});
it("刷新后沿用待确认凭据的连接和请求，不要求再次新建", async () => {
  localStorage.setItem(
    "glowloom.catalog.pending.v1.admin-fixture.preset.onboarding.credential",
    JSON.stringify({
      id: "original-key-request",
      action: "credential.set",
      target: "connection",
    }),
  );
  api.receipt.mockRejectedValueOnce(new RequestError("not_found", 404));
  render(
    <PresetConnect presets={presets} onCreated={created} onSaved={saved} />,
  );
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "继续保存 API Key",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "继续保存 API Key" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).not.toHaveBeenCalled();
  expect(api.key.mock.calls[0][0]).toEqual({ id: "connection" });
  expect(api.key.mock.calls[0][2].headers["Idempotency-Key"]).toBe(
    "original-key-request",
  );
});
