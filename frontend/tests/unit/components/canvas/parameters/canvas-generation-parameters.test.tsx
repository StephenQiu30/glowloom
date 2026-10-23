import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CanvasGenerationParameters } from "@/components/canvas/parameters/canvas-generation-parameters";

const state = vi.hoisted(() => ({
  catalogError: null as Error | null,
  mutateOptions: vi.fn(),
  mutateCatalog: vi.fn(),
}));
vi.mock(
  "@/components/canvas/runtime/components/account/account-provider",
  () => ({
    useAccount: () => ({
      account: { id: "test" },
      ready: true,
      signingOut: false,
    }),
  }),
);
vi.mock("@/components/canvas/runtime/store/account-slice", () => ({
  accountScope: () => "test-account",
}));
vi.mock(
  "@/components/canvas/runtime/components/feedback/error-notification",
  () => ({ useErrorNotification: vi.fn() }),
);
vi.mock("swr", () => ({
  default: (key: string[]) =>
    key[0] === "canvas-generation-options"
      ? {
          data: {
            parameters:
              key[2] === "image"
                ? [
                    {
                      key: "size",
                      label: "清晰度",
                      values: ["1K", "2K", "4K"],
                      defaultValue: "2K",
                    },
                  ]
                : [
                    {
                      key: "ratio",
                      label: "画面比例",
                      values: ["1:1", "16:9"],
                    },
                    {
                      key: "duration",
                      label: "时长",
                      numeric: true,
                      values: ["5", "10"],
                    },
                  ],
          },
          mutate: state.mutateOptions,
        }
      : {
          data: [
            {
              connectionId: "connection",
              modelId: "model",
              imageSizes: ["1K", "2K", "4K"],
              nativeCapabilities: { ratios: ["16:9", "9:16"] },
            },
          ],
          error: state.catalogError,
          mutate: state.mutateCatalog,
        },
}));
afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  state.catalogError = null;
});
const parameters = {
  modelSelection: { connectionId: "connection", modelId: "model" },
  modelParameters: { ratio: "16:9", duration: 5 },
};

it("uses model capabilities and emits native values without clearing a selected option", () => {
  const change = vi.fn();
  render(
    <CanvasGenerationParameters
      type="video"
      parameters={parameters}
      onChange={change}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "生成参数" }));
  expect(screen.queryByRole("radio", { name: "画面比例 1:1" })).toBeNull();
  fireEvent.click(screen.getByRole("radio", { name: "画面比例 16:9" }));
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("radio", { name: "画面比例 9:16" }));
  expect(change).toHaveBeenLastCalledWith(
    expect.objectContaining({
      modelParameters: { ratio: "9:16", duration: 5 },
    }),
  );
  fireEvent.click(screen.getByRole("radio", { name: "时长 10" }));
  expect(change).toHaveBeenLastCalledWith(
    expect.objectContaining({
      modelParameters: { ratio: "16:9", duration: 10 },
    }),
  );
});

it("retains unsupported saved values and retries both capability sources after failure", () => {
  state.catalogError = new Error("unavailable");
  const change = vi.fn();
  render(
    <CanvasGenerationParameters
      type="video"
      parameters={{
        ...parameters,
        modelParameters: { ratio: "4:3", duration: 5 },
      }}
      onChange={change}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "生成参数" }));
  expect(
    (screen.getByRole("radio", { name: "4:3" }) as HTMLButtonElement).disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("radio", { name: "画面比例 9:16" }));
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(state.mutateOptions).toHaveBeenCalledOnce();
  expect(state.mutateCatalog).toHaveBeenCalledOnce();
});

it("maps image ratio choices to native size and disables unsupported size conversions", () => {
  const change = vi.fn();
  const view = render(
    <CanvasGenerationParameters
      type="image"
      parameters={{
        modelSelection: parameters.modelSelection,
        modelParameters: { size: "1K" },
      }}
      onChange={change}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "生成参数" }));
  fireEvent.click(screen.getByRole("radio", { name: "画面比例 9:16" }));
  expect(change).toHaveBeenLastCalledWith(
    expect.objectContaining({ modelParameters: { size: "765x1360" } }),
  );
  change.mockClear();
  view.rerender(
    <CanvasGenerationParameters
      type="image"
      parameters={{
        modelSelection: parameters.modelSelection,
        modelParameters: { size: "4K" },
      }}
      onChange={change}
    />,
  );
  const ratio = screen.getByRole("radio", {
    name: "画面比例 9:16",
  }) as HTMLButtonElement;
  expect(ratio.disabled).toBe(true);
  fireEvent.click(ratio);
  expect(change).not.toHaveBeenCalled();
});
