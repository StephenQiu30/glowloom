import { render } from "../../../../feedback";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProviderTrial } from "@/app/models/_components/provider-trial";
import { createProviderTrialQuote, getConfigReceipt } from "@/api/settings";
import { confirmOperationQuote, getOperation } from "@/api/operations";
import { listProjects } from "@/api/projects";

vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "actor" } }),
}));
vi.mock("@/api/settings", () => ({
  createProviderTrialQuote: vi.fn(),
  getConfigReceipt: vi.fn(),
}));
vi.mock("@/api/operations", () => ({
  confirmOperationQuote: vi.fn(),
  getOperation: vi.fn(),
}));
vi.mock("@/api/projects", () => ({ listProjects: vi.fn() }));
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
    localStorage.clear();
    vi.mocked(listProjects).mockResolvedValue({
      items: [{ id: "project", name: "测试项目" }],
    });
  });
  it("prepares a free quote without automatic confirmation and persists identities only", async () => {
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
    await screen.findByText("测试项目");
    fireEvent.change(screen.getByLabelText("扣费项目"), {
      target: { value: "project" },
    });
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
    const saved = localStorage.getItem(key)!;
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
    expect(JSON.parse(localStorage.getItem(key)!).confirmRequested).toBe(true);
    expect(
      screen.queryByRole("button", { name: "准备另一项试运行" }),
    ).toBeNull();
    const confirmKey = JSON.parse(saved).confirm;
    expect(vi.mocked(confirmOperationQuote).mock.calls[0][2]?.headers).toEqual({
      "Idempotency-Key": confirmKey,
    });
  });
  it("recovers a quote through its receipt without creating or charging again", async () => {
    localStorage.setItem(
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
