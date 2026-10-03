import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { ProviderRevisionForm } from "./settings-providers";
import { ModelVersionForm } from "./admin-model-forms";
import type { Capability, ModelDetail, Provider } from "./admin-queries";

const api = vi.hoisted(() => ({
  updateAdminProvider: vi.fn(),
  publishAdminModelVersion: vi.fn(),
}));
vi.mock("@/api/settings", async (original) => ({
  ...(await original<typeof import("@/api/settings")>()),
  ...api,
}));
const id = "20000000-0000-4000-8000-000000000001";
const time = "2026-10-01T00:00:00Z";
const provider: Provider = {
  id,
  key: "fixture",
  name: "测试渠道",
  adapter_key: "mock",
  region: "domestic",
  status: "active",
  concurrency_limit: 1,
  rate_limit_per_min: 10,
  revision: 7,
  create_time: time,
  update_time: time,
};
const detail: ModelDetail = {
  model: {
    id,
    provider_id: id,
    model_key: "fixture",
    display_name: "测试模型",
    capability: "image.generate",
    current_version_id: id,
    status: "disabled",
    revision: 7,
    create_time: time,
    update_time: time,
  },
  versions: [
    {
      id,
      version_no: 3,
      provider_model_id: "fixture-model",
      modes: ["text_to_image"],
      limits: { max_outputs: 1 },
      param_schema: [],
      supports_query: false,
      supports_cancel: false,
      supports_callback: false,
      expected_max_ms: 120000,
      moderation: "platform",
      queue: "agent.mock",
      create_time: time,
    },
  ],
  prices: [],
};
const capability: Capability = {
  id,
  key: "image.generate",
  modes: ["text_to_image"],
  input_roles: [],
  output_type: "image",
};
function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            mutations: { retry: false },
            queries: { retry: false },
          },
        })
      }
    >
      {children}
    </QueryClientProvider>
  );
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
describe("管理员修订与发布", () => {
  it("409 使用读取时的修订，保留输入并提示重新读取", async () => {
    const saved = vi.fn();
    api.updateAdminProvider.mockRejectedValue(
      new ApiError(409, "revision_conflict"),
    );
    render(<ProviderRevisionForm provider={provider} saved={saved} />, {
      wrapper,
    });
    fireEvent.change(screen.getByLabelText("最大并发数"), {
      target: { value: "5" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存渠道配置" }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain("最新版本"),
    );
    expect(api.updateAdminProvider.mock.calls[0][1]).toEqual({
      concurrency_limit: 5,
      rate_limit_per_min: 10,
      status: "active",
      expected_revision: 7,
    });
    expect(screen.getByLabelText("最大并发数")).toHaveProperty("value", "5");
    expect(saved).not.toHaveBeenCalled();
  });
  it("非法 JSON 阻止配置发布", async () => {
    render(
      <ModelVersionForm
        detail={detail}
        capability={capability}
        done={vi.fn()}
      />,
      { wrapper },
    );
    fireEvent.change(screen.getByLabelText("输入与输出限制（JSON 对象）"), {
      target: { value: "{invalid-json" },
    });
    fireEvent.click(screen.getByRole("button", { name: "发布配置 v4" }));
    await waitFor(() =>
      expect(screen.getByText("请输入合法 JSON")).toBeTruthy(),
    );
    expect(api.publishAdminModelVersion).not.toHaveBeenCalled();
  });
  it("表单打开后固定发布修订，不用后台刷新掩盖冲突", async () => {
    api.publishAdminModelVersion.mockRejectedValue(
      new ApiError(409, "revision_conflict"),
    );
    const { rerender } = render(
      <ModelVersionForm
        detail={detail}
        capability={capability}
        done={vi.fn()}
      />,
      { wrapper },
    );
    rerender(
      <ModelVersionForm
        detail={{ ...detail, model: { ...detail.model, revision: 9 } }}
        capability={capability}
        done={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "发布配置 v4" }));
    await waitFor(() =>
      expect(api.publishAdminModelVersion).toHaveBeenCalledOnce(),
    );
    expect(api.publishAdminModelVersion.mock.calls[0][1]).toMatchObject({
      expected_revision: 7,
      version_no: 4,
      limits: { max_outputs: 1 },
      param_schema: [],
    });
    expect(screen.getByRole("alert").textContent).toContain("最新版本");
  });
});
