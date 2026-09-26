import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { QuoteConfirmDialog, type QuoteResponse } from "./quote-confirm-dialog";

afterEach(cleanup);

function quote(overrides: Partial<QuoteResponse> = {}): QuoteResponse {
  return {
    batch_id: "batch-1",
    expires_at: new Date(Date.now() + 60_000).toISOString(),
    items: [
      {
        operation_id: "op-1",
        target_label: "镜头 3-12",
        model_key: "ark.seedance-2-pro",
        mode: "omni_reference",
        quote_micros: 20_000_000,
        reused: false,
        region: "overseas",
        errors: [],
        quote_detail: {
          unit: "per_second",
          quantity: 10,
          unit_price_micros: 1_000_000,
          outputs: 2,
        },
      },
    ],
    total_micros: 20_000_000,
    available_micros: 30_000_000,
    confirmable: true,
    ...overrides,
  };
}

it("shows the quote, budget, region and cost detail; only an explicit click confirms", async () => {
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  render(
    <QuoteConfirmDialog
      open
      onOpenChange={vi.fn()}
      quote={quote()}
      onConfirm={onConfirm}
      onRequote={vi.fn()}
    />,
  );

  expect(screen.getByRole("dialog", { name: "确认生成报价" })).toBeTruthy();
  expect(screen.getByText("ark.seedance-2-pro")).toBeTruthy();
  expect(screen.getByText(/数据出境/)).toBeTruthy();
  expect(screen.getByText(/剩余预算/).parentElement?.textContent).toContain(
    "¥30.00",
  );
  fireEvent.click(screen.getByText("费用明细"));
  expect(screen.getByText(/10 × ¥1.00 × 2/)).toBeTruthy();

  const confirm = screen.getByRole("button", { name: "生成（约 ¥20.00）" });
  expect(fireEvent.keyDown(confirm, { key: "Enter", code: "Enter" })).toBe(
    false,
  );
  expect(onConfirm).not.toHaveBeenCalled();
  fireEvent.click(confirm);
  await waitFor(() => expect(onConfirm).toHaveBeenCalledExactlyOnceWith([]));
});

it("blocks an over-budget quote until an item is excluded", async () => {
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  render(
    <QuoteConfirmDialog
      open
      onOpenChange={vi.fn()}
      quote={quote({
        items: [
          quote().items[0],
          {
            operation_id: "op-2",
            target_label: "镜头 3-13",
            quote_micros: 20_000_000,
            region: "domestic",
            reused: false,
            errors: [],
          },
        ],
        total_micros: 40_000_000,
        confirmable: false,
      })}
      onConfirm={onConfirm}
      onRequote={vi.fn()}
    />,
  );

  expect(screen.getByText(/预算不足，还差 ¥10.00/)).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "批量生成 2 项（约 ¥40.00）" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "剔除 镜头 3-13" }));
  expect(
    screen
      .getByRole("button", { name: "生成（约 ¥20.00）" })
      .hasAttribute("disabled"),
  ).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "生成（约 ¥20.00）" }));
  await waitFor(() =>
    expect(onConfirm).toHaveBeenCalledExactlyOnceWith(["op-2"]),
  );
});

it("shows invalid and reused items, and requires a fresh quote after expiry", () => {
  const onConfirm = vi.fn();
  const onRequote = vi.fn();
  const onForceRegenerate = vi.fn();
  render(
    <QuoteConfirmDialog
      open
      onOpenChange={vi.fn()}
      quote={quote({
        expires_at: new Date(Date.now() - 1_000).toISOString(),
        items: [
          {
            operation_id: "op-reused",
            target_label: "镜头 3-12",
            quote_micros: 0,
            reused: true,
            region: "domestic",
            errors: [],
          },
          {
            operation_id: null,
            target_label: "镜头 3-13",
            errors: [{ code: "input_not_ready", message: "素材尚未就绪" }],
          },
        ],
        total_micros: 0,
      })}
      onConfirm={onConfirm}
      onRequote={onRequote}
      onForceRegenerate={onForceRegenerate}
    />,
  );

  expect(screen.getByText("复用已有结果，¥0")).toBeTruthy();
  expect(screen.getByText("素材尚未就绪")).toBeTruthy();
  fireEvent.click(
    screen.getByRole("button", { name: "强制重新生成 镜头 3-12" }),
  );
  expect(onForceRegenerate).toHaveBeenCalledExactlyOnceWith("op-reused");
  expect(
    screen
      .getByRole("button", { name: "批量生成 2 项（约 ¥0.00）" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重新报价" }));
  expect(onRequote).toHaveBeenCalledOnce();
  expect(onConfirm).not.toHaveBeenCalled();
});

it("keeps a server rejected confirmation visible without exposing internal errors", async () => {
  const onConfirm = vi.fn().mockRejectedValue({
    code: "quote_expired",
    message: "internal database detail",
  });
  render(
    <QuoteConfirmDialog
      open
      onOpenChange={vi.fn()}
      quote={quote()}
      onConfirm={onConfirm}
      onRequote={vi.fn()}
    />,
  );

  fireEvent.click(screen.getByRole("button", { name: "生成（约 ¥20.00）" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    "报价已失效，请重新报价",
  );
  expect(screen.queryByText(/internal database detail/)).toBeNull();
});

it("blocks confirmation when the server total differs from the visible item costs", () => {
  const onConfirm = vi.fn();
  render(
    <QuoteConfirmDialog
      open
      onOpenChange={vi.fn()}
      quote={quote({ total_micros: 19_000_000 })}
      onConfirm={onConfirm}
      onRequote={vi.fn()}
    />,
  );

  expect(screen.getByText("报价金额不一致，请重新报价。")).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "生成（约 ¥20.00）" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(onConfirm).not.toHaveBeenCalled();
});
