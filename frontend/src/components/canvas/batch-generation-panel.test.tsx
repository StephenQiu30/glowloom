import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { BatchGenerationPanel } from "./batch-generation-panel";
import { createBatchRow, createBatchTable } from "./batch-table";

const calls = vi.hoisted(() => ({
  quote: vi.fn(),
  confirm: vi.fn(),
  models: vi.fn(),
}));
vi.mock("@/components/catalog/queries", () => ({
  MODELS_KEY: ["models"],
  queryModels: calls.models,
}));
vi.mock("@/components/operation/queries", () => ({
  OPERATIONS_KEY: ["operations"],
  quoteGeneration: calls.quote,
  confirmGeneration: calls.confirm,
}));
vi.mock("@/components/catalog/model-params-form", () => ({
  ModelParamsForm: ({
    onSubmit,
    disabled,
    submitLabel,
  }: {
    onSubmit: (value: object) => void;
    disabled: boolean;
    submitLabel: string;
  }) => (
    <button
      disabled={disabled}
      onClick={() => onSubmit({ quality: "standard" })}
    >
      {submitLabel}
    </button>
  ),
}));
vi.mock("@/components/operation/quote-confirm-dialog", () => ({
  QuoteConfirmDialog: ({
    open,
    quote,
    onConfirm,
  }: {
    open: boolean;
    quote: { items: unknown[]; total_micros: number };
    onConfirm: (ids: string[]) => Promise<void>;
  }) =>
    open ? (
      <div role="dialog">
        <p>
          {quote.items.length} 行共 {quote.total_micros}
        </p>
        <button
          onClick={() => {
            void onConfirm([]).catch(() => {});
          }}
        >
          确认全部费用
        </button>
      </div>
    ) : null,
}));

const modelId = "114747cf-9a3f-4d53-8566-4c1fd1a63348";
function mount() {
  const config = createBatchTable();
  config.modelProfileId = modelId;
  config.mode = "generate";
  config.rows = Array.from({ length: 500 }, () => createBatchRow(config));
  let revision = 7;
  const save = vi.fn(async () => {
    revision = 8;
    return true;
  });
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={cache}>
      <BatchGenerationPanel
        projectId="1f882629-1c1d-45c7-a42d-858424d77b27"
        source={() => ({
          canvas_id: "825c02c6-2bc1-4f37-87f2-86b1b76f93b3",
          node_id: "1848eae1-a3dc-47f6-8e4a-560087494152",
          revision,
        })}
        config={config}
        nodes={[]}
        disabled={false}
        onChange={vi.fn()}
        onPersist={save}
        onBusy={vi.fn()}
        onRetryCleared={vi.fn()}
      />
    </QueryClientProvider>,
  );
  return { config, save, cache };
}
beforeEach(() => {
  calls.models.mockResolvedValue({
    items: [
      {
        id: modelId,
        key: "synthetic-image",
        display_name: "合成模型",
        capability: "image.generate",
        status: "active",
        input_roles: [],
        current_version: {
          id: crypto.randomUUID(),
          modes: ["generate"],
          param_schema: [],
        },
      },
    ],
    next_cursor: null,
  });
  calls.quote.mockImplementation(async (_project, items, _key, labels) => ({
    batch_id: crypto.randomUUID(),
    expires_at: "2099-10-01T12:00:00Z",
    items: items.map((_item: unknown, index: number) => ({
      operation_id: crypto.randomUUID(),
      target_label: labels[index],
      quote_micros: 10,
      errors: [],
    })),
    total_micros: items.length * 10,
    available_micros: 10000,
    confirmable: true,
  }));
  calls.confirm.mockResolvedValue({});
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
describe("全表批量费用与确认边界", () => {
  it("先保存最新修订，再为500行获取4份真实报价，一次确认依次提交原批次", async () => {
    const { config, save } = mount();
    fireEvent.click(
      await screen.findByRole("button", { name: "保存参数并预览全表费用" }),
    );
    await screen.findByText("500 行共 5000");
    expect(save).toHaveBeenCalledOnce();
    expect(calls.quote.mock.calls.map((call) => call[1].length)).toEqual([
      150, 150, 150, 50,
    ]);
    expect(
      calls.quote.mock.calls
        .flatMap((call) => call[1])
        .map((item) => item.source.row_id),
    ).toEqual(config.rows.map((row) => row.id));
    expect(
      calls.quote.mock.calls
        .flatMap((call) => call[1])
        .every((item) => item.source.revision === 8),
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "确认全部费用" }));
    await screen.findByText("500 行已确认。", { exact: false });
    expect(calls.confirm).toHaveBeenCalledTimes(4);
    expect(
      new Set(calls.confirm.mock.calls.map((call) => call[1].batch_id)).size,
    ).toBe(4);
  });
  it("第二组网络失败保留第一组事实，核验使用完全相同幂等键且不另发报价", async () => {
    calls.confirm
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new ApiError(0, "dependency_unavailable"))
      .mockResolvedValue({});
    mount();
    fireEvent.click(
      await screen.findByRole("button", { name: "保存参数并预览全表费用" }),
    );
    await screen.findByText("500 行共 5000");
    fireEvent.click(screen.getByRole("button", { name: "确认全部费用" }));
    await screen.findByText(/已有 150 行确认。当前组确认结果未知/);
    expect(
      (
        screen.getByRole("button", {
          name: "保存参数并预览全表费用",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    const original = calls.confirm.mock.calls[1];
    await act(async () =>
      fireEvent.click(
        screen.getByRole("button", { name: "按原幂等请求核验确认结果" }),
      ),
    );
    await waitFor(() => expect(calls.confirm).toHaveBeenCalledTimes(3));
    expect(calls.confirm.mock.calls[2]).toEqual(original);
    expect(calls.quote).toHaveBeenCalledTimes(4);
    expect(
      calls.confirm.mock.calls.filter(
        (call) => call[1].batch_id === calls.confirm.mock.calls[0][1].batch_id,
      ),
    ).toHaveLength(1);
    await screen.findByRole("button", { name: "为未确认部分预览费用" });
  });
});
