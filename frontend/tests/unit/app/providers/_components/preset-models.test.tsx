import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PresetModels } from "@/app/providers/_components/preset-models";
vi.mock("@/api/settings", () => ({
  listAdminModels: async () => ({ items: [] }),
  listAdminProviders: async () => ({ items: [] }),
  getAdminModel: vi.fn(),
  listModelReleases: vi.fn(),
}));
afterEach(cleanup);
it("手工按次模型模板要求管理员明确金额和来源时间，不填虚构价表", () => {
  const change = vi.fn();
  render(
    <PresetModels adapter="openrouter" templates={[]} onChange={change} />,
  );
  fireEvent.click(screen.getByRole("radio", { name: "直接添加新模型" }));
  fireEvent.change(screen.getByLabelText("模型显示名称"), {
    target: { value: "Reviewed fixture" },
  });
  fireEvent.change(screen.getByLabelText("上游模型标识"), {
    target: { value: "fixture/model" },
  });
  const button = screen.getByRole("button", {
    name: "确认添加模型模板",
  }) as HTMLButtonElement;
  expect(button.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("基础单价（人民币微元/次）"), {
    target: { value: "10" },
  });
  fireEvent.change(screen.getByLabelText("上游成本上限（人民币微元/次）"), {
    target: { value: "20" },
  });
  fireEvent.change(screen.getByLabelText("计价核验来源"), {
    target: { value: "https://example.invalid/prices" },
  });
  fireEvent.change(screen.getByLabelText("计价核验时间"), {
    target: { value: "2026-11-01T01:00" },
  });
  fireEvent.click(button);
  expect(change).toHaveBeenCalledOnce();
  const template = change.mock.calls[0][0][0];
  expect(template.price.rule).toEqual({ base_micros: 10 });
  expect(template.price.unit).toBe("per_request");
  expect(template.price.source_url).toBe("https://example.invalid/prices");
  expect(template.upstream_cost.max_micros).toBe(20);
});
