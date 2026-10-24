import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CanvasModelSelect } from "@/components/canvas/parameters/canvas-model-select";
const state = vi.hoisted(() => ({ retry: vi.fn().mockResolvedValue([]) }));
vi.mock("swr", () => ({ default: () => ({ data: [], mutate: state.retry }) }));
vi.mock(
  "@/components/canvas/runtime/components/account/account-provider",
  () => ({
    useAccount: () => ({
      account: { id: "canvas-test" },
      ready: true,
      signingOut: false,
    }),
  }),
);
vi.mock("@/components/canvas/runtime/store/account-slice", () => ({
  accountScope: () => "canvas-test",
}));
vi.mock(
  "@/components/canvas/runtime/components/feedback/error-notification",
  () => ({ useErrorNotification: vi.fn(), notifyError: vi.fn() }),
);
afterEach(cleanup);
it("无模型时从模型菜单重试，不触发模型选择或丢失已有模型", () => {
  const select = vi.fn();
  render(
    <CanvasModelSelect
      type="image"
      selected={{ connectionId: "saved", modelId: "model" }}
      onSelect={select}
    />,
  );
  expect(screen.queryByRole("button", { name: "重新读取模型" })).toBeNull();
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成模型" }), {
    key: "ArrowDown",
  });
  expect(
    screen
      .getByRole("option", { name: "已保存的模型" })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("option", { name: "重新读取模型" }));
  expect(state.retry).toHaveBeenCalledOnce();
  expect(select).not.toHaveBeenCalled();
});
