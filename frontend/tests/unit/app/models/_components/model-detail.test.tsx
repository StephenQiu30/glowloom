import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { ModelDetail } from "@/app/models/_components/model-detail";
import { RequestError } from "@/lib/request";

const api = vi.hoisted(() => ({ model: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "admin" } }),
}));
vi.mock("@/api/settings", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/settings")>()),
  getAdminModel: api.model,
  listModelReleases: async () => ({ items: [] }),
  listCapabilityDefaults: async () => ({ items: [] }),
}));

beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

it("详情读取失败时结束加载并允许重试同一模型", async () => {
  api.model.mockRejectedValueOnce(
    new RequestError("dependency_unavailable", 503),
  );
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  await waitFor(() => expect(screen.queryByText("正在加载配置…")).toBeNull());
  expect(screen.getByRole("alert").textContent).toContain("服务暂不可用");
  api.model.mockResolvedValueOnce({
    model: { id: "model-a", display_name: "待接入模型", status: "disabled" },
    versions: [],
    prices: [],
  });
  fireEvent.click(screen.getByRole("button", { name: "重试加载" }));
  await screen.findByRole("heading", { name: "待接入模型" });
  expect(screen.queryByRole("button", { name: "重试加载" })).toBeNull();
  expect(api.model.mock.calls.map(([input]) => input.id)).toEqual([
    "model-a",
    "model-a",
  ]);
});
