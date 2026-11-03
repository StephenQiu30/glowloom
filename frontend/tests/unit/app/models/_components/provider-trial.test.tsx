import { RequestError } from "@/lib/request";
import { pageBusinessMemory } from "@/lib/page-business-memory";
import { render as renderView } from "../../../../feedback";
import { SWRConfig } from "swr";
import type { ReactNode } from "react";
function render(ui: ReactNode) {
  return renderView(
    <SWRConfig
      value={{
        provider: () => new Map(),
        dedupingInterval: 0,
        errorRetryCount: 0,
      }}
    >
      {ui}
    </SWRConfig>,
  );
}
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProviderTrial } from "@/components/catalog/provider-trial";
import {
  createProviderTrialQuote,
  getConfigReceipt,
  getProviderTrialRecovery,
} from "@/api/settings";
import { confirmOperationQuote, getOperation } from "@/api/operations";
import { listProjects } from "@/api/projects";

vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "actor" } }),
}));
vi.mock("@/api/settings", () => ({
  createProviderTrialQuote: vi.fn(),
  getConfigReceipt: vi.fn(),
  getProviderTrialRecovery: vi.fn(),
}));
vi.mock("@/api/operations", () => ({
  confirmOperationQuote: vi.fn(),
  getOperation: vi.fn(),
}));
vi.mock("@/api/projects", () => ({ listProjects: vi.fn() }));
beforeEach(() => {
  vi.clearAllMocks();
  pageBusinessMemory.clear();
  vi.mocked(getProviderTrialRecovery).mockResolvedValue({ found: false });
  vi.mocked(listProjects).mockResolvedValue({
    items: [{ id: "project", name: "测试项目" }],
  });
});
afterEach(cleanup);
const release: API.ModelRelease = {
  id: "release",
  required_modes: ["text_to_text"],
  upstream_cost: { currency: "USD", max_micros: 100 },
};
const key = "glowloom.trial.v1.actor.release";
describe("provider trial consent and recovery", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.clearAllMocks();
    pageBusinessMemory.clear();
    vi.mocked(listProjects).mockResolvedValue({
      items: [{ id: "project", name: "测试项目" }],
    });
  });
  it("prepares a free quote without automatic confirmation and keeps only identities in page memory", async () => {
    vi.mocked(createProviderTrialQuote).mockResolvedValue({
      operation_id: "operation",
      quote_micros: 100,
    });
    vi.mocked(getOperation).mockResolvedValue({
      id: "operation",
      project_id: "project",
      status: "quoted",
      quote_micros: 100,
    });
    render(<ProviderTrial release={release} onVerified={vi.fn()} />);
    fireEvent.click(screen.getByText("试运行此发布"));
    fireEvent.keyDown(
      await screen.findByRole("combobox", { name: "扣费项目" }),
      { key: "ArrowDown" },
    );
    fireEvent.click(await screen.findByRole("option", { name: "测试项目" }));
    fireEvent.change(screen.getByLabelText("提示词"), {
      target: { value: "private prompt fixture" },
    });
    fireEvent.change(screen.getByLabelText("本次平台费用上限（元）"), {
      target: { value: "1" },
    });
    fireEvent.submit(
      screen.getByRole("button", { name: "准备免费报价" }).closest("form")!,
    );
    await screen.findByRole("button", { name: "确认费用并执行" });
    expect(confirmOperationQuote).not.toHaveBeenCalled();
    const saved = pageBusinessMemory.getItem(key)!;
    expect(saved).toContain("operation");
    expect(saved).not.toContain("private prompt fixture");
    expect(saved).not.toContain("api_key");
    vi.mocked(confirmOperationQuote).mockRejectedValue(
      new Error("response lost"),
    );
    fireEvent.submit(
      screen.getByRole("button", { name: "确认费用并执行" }).closest("form")!,
    );
    await screen.findByText("response lost");
    expect(JSON.parse(pageBusinessMemory.getItem(key)!).confirmRequested).toBe(
      true,
    );
    expect(
      screen.queryByRole("button", { name: "准备另一项试运行" }),
    ).toBeNull();
    const confirmKey = JSON.parse(saved).confirm;
    expect(vi.mocked(confirmOperationQuote).mock.calls[0][2]?.headers).toEqual({
      "Idempotency-Key": confirmKey,
    });
  });
  it("recovers a quote through its receipt without creating or charging again", async () => {
    pageBusinessMemory.setItem(
      key,
      JSON.stringify({
        request: "request",
        confirm: "confirmation",
        project: "project",
      }),
    );
    vi.mocked(getConfigReceipt).mockResolvedValue({
      result: { operation_id: "operation", project_id: "project" },
    });
    vi.mocked(getOperation).mockResolvedValue({
      id: "operation",
      project_id: "project",
      status: "submitted",
      quote_micros: 100,
    });
    render(<ProviderTrial release={release} onVerified={vi.fn()} />);
    fireEvent.click(screen.getByText("试运行此发布"));
    await act(async () =>
      fireEvent.click(
        screen.getByRole("button", { name: "查询报价回执 / 刷新任务" }),
      ),
    );
    await waitFor(() =>
      expect(screen.getByText(/任务状态：submitted/)).toBeTruthy(),
    );
    expect(createProviderTrialQuote).not.toHaveBeenCalled();
    expect(confirmOperationQuote).not.toHaveBeenCalled();
  });
});
it("服务测试中的正式发布必须先选项目，参数使用模型表单且不暴露JSON", async () => {
  render(
    <ProviderTrial
      embedded
      release={{
        id: "music-release",
        required_modes: ["music_song"],
        upstream_cost: { currency: "CNY", max_micros: 480000 },
      }}
      version={{
        id: "music-version",
        param_schema: [
          {
            field: "duration_ms",
            label: "时长",
            type: "integer",
            component: "input",
            min: 30000,
            max: 240000,
            default: 60000,
          },
        ],
        limits: { max_outputs: 1 },
      }}
      onVerified={() => {}}
    />,
  );
  const prepare = await screen.findByRole("button", { name: "准备验证" });
  expect(prepare).toHaveProperty("disabled", true);
  expect(screen.queryByLabelText("参数 JSON")).toBeNull();
  expect(screen.queryByLabelText(/参考素材列表/)).toBeNull();
  expect(createProviderTrialQuote).not.toHaveBeenCalled();
  expect(confirmOperationQuote).not.toHaveBeenCalled();
});

it("表单控件的空值同步不能清空已选验证模式和参考素材入口", async () => {
  const view = render(
    <ProviderTrial
      embedded
      release={{
        id: "image-release",
        required_modes: ["text2image", "image2image"],
      }}
      version={{
        limits: {
          roles: {
            reference_image: {
              types: ["image"],
              min_count: 1,
              max_count: 1,
              for_modes: ["image2image"],
            },
          },
        },
        param_schema: [],
      }}
      onVerified={vi.fn()}
    />,
  );
  fireEvent.keyDown(await screen.findByRole("combobox", { name: "验证模式" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "image2image" }));
  await screen.findByText("参考图片（必选）");
  const nativeMode = view.container.querySelector('select[name="mode"]');
  expect(nativeMode).toBeTruthy();
  fireEvent.change(nativeMode!, { target: { value: "" } });
  expect(
    screen.getByRole("combobox", { name: "验证模式" }).textContent,
  ).toContain("image2image");
  expect(screen.getByText("参考图片（必选）")).toBeTruthy();
  expect(createProviderTrialQuote).not.toHaveBeenCalled();
});

it("整页刷新丢失本地标识后从服务端恢复原收费任务", async () => {
  vi.mocked(getProviderTrialRecovery).mockResolvedValue({
    found: true,
    operation_id: "persisted-operation",
    project_id: "project",
    request_id: "original-request",
    confirm_request_id: "original-confirm",
    status: "submitted",
    supplier_currency: "CNY",
    supplier_max_micros: 480000,
  });
  vi.mocked(getOperation).mockResolvedValue({
    id: "persisted-operation",
    project_id: "project",
    status: "submitted",
    model_name: "火山音乐",
    mode: "music_song",
    params: { duration_ms: 60000 },
    output_count: 1,
  });
  render(<ProviderTrial embedded release={release} onVerified={() => {}} />);
  await screen.findByText(/任务状态：submitted/);
  expect(screen.queryByRole("button", { name: "准备验证" })).toBeNull();
  expect(createProviderTrialQuote).not.toHaveBeenCalled();
  expect(confirmOperationQuote).not.toHaveBeenCalled();
});
it("报价被明确拒绝后可以修正输入，不锁成未知收费任务", async () => {
  vi.mocked(createProviderTrialQuote).mockRejectedValue(
    new RequestError("invalid_request", 422),
  );
  render(<ProviderTrial release={release} onVerified={() => {}} />);
  fireEvent.click(screen.getByText("试运行此发布"));
  fireEvent.keyDown(await screen.findByRole("combobox", { name: "扣费项目" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "测试项目" }));
  fireEvent.change(screen.getByLabelText("提示词"), {
    target: { value: "fixture" },
  });
  fireEvent.change(screen.getByLabelText("本次平台费用上限（元）"), {
    target: { value: "1" },
  });
  fireEvent.submit(
    screen.getByRole("button", { name: "准备免费报价" }).closest("form")!,
  );
  await waitFor(() => expect(createProviderTrialQuote).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "准备免费报价" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  expect(pageBusinessMemory.getItem(key)).toBeNull();
});

it("恢复收费报价时显示服务端冻结的验证摘要", async () => {
  vi.mocked(getProviderTrialRecovery).mockResolvedValue({
    found: true,
    operation_id: "quoted-operation",
    project_id: "project",
    request_id: "quote-request",
    confirm_request_id: "stable-confirm",
    status: "quoted",
    supplier_currency: "CNY",
    supplier_max_micros: 480000,
  });
  vi.mocked(getOperation).mockResolvedValue({
    id: "quoted-operation",
    project_id: "project",
    status: "quoted",
    model_name: "火山音乐",
    mode: "music_song",
    params: { duration_ms: 60000 },
    inputs: [{ role: "prompt", text: "fixture song" }],
    output_count: 1,
    quote_micros: 100,
  });
  render(<ProviderTrial embedded release={release} onVerified={() => {}} />);
  await screen.findByLabelText("本次验证摘要");
  expect(screen.getByText(/模型：火山音乐/)).toBeDefined();
  expect(screen.getByText(/模式：music_song/)).toBeDefined();
  expect(screen.getByText(/0.480000/)).toBeDefined();
  expect(confirmOperationQuote).not.toHaveBeenCalled();
  expect(createProviderTrialQuote).not.toHaveBeenCalled();
});
