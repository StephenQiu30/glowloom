import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { QuoteCostDetail } from "@/components/generation/quote-cost-detail";
afterEach(cleanup);
it("shows the server credit estimate and keeps billing rules collapsed", () => {
  render(
    <QuoteCostDetail
      amount={1}
      detail={{
        amount_kind: "reservation",
        price_version_no: 3,
        unit: "per_image",
        unit_price_micros: 1,
        quantity: 1,
        outputs: 1,
        currency: "CNY",
        estimated_credits: 1,
      }}
    />,
  );
  expect(screen.getByText("预计消耗 1 积分")).toBeTruthy();
  expect(screen.queryByText(/¥0.000001/)).toBeNull();
  const detail = screen.getByRole("button", { name: "费用说明" });
  expect(detail.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByText(/预留金额不代表最终实扣或费用上限/)).toBeNull();
  fireEvent.click(detail);
  expect(screen.getByText(/¥0.000001/)).toBeTruthy();
  expect(screen.getByText(/预留金额不代表最终实扣或费用上限/)).toBeTruthy();
  expect(screen.getByText("每张图片")).toBeTruthy();
});
it("keeps missing and unit-only amounts unknown rather than free", () => {
  const { rerender } = render(
    <QuoteCostDetail
      preview={{ amount_kind: "unknown", confirmable: false }}
    />,
  );
  expect(screen.getByText("费用待确认")).toBeTruthy();
  expect(screen.queryByText(/¥0/)).toBeNull();
  rerender(
    <QuoteCostDetail
      preview={{
        amount_kind: "unit_only",
        confirmable: false,
        unit: "per_image",
        currency: "CNY",
      }}
    />,
  );
  expect(screen.getByText("费用待确认")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /确认/ })).toBeNull();
});

it("shows a monetary reservation when the quote has no valid credit estimate", () => {
  render(
    <QuoteCostDetail amount={250_000} detail={{ estimated_credits: 1.5 }} />,
  );
  expect(screen.getByText("预计预留 ¥0.25")).toBeTruthy();
  expect(screen.queryByText(/积分/)).toBeNull();
  expect(screen.queryByText(/价格版本|预留金额不代表/)).toBeNull();
});

it("distinguishes an explicit zero quote from an unknown amount", () => {
  render(<QuoteCostDetail amount={0} detail={{ estimated_credits: 0 }} />);
  expect(screen.getByText("预计消耗 0 积分")).toBeTruthy();
  expect(screen.queryByText("费用待确认")).toBeNull();
});

it("shows exact fractional credits and the approved cap", () => {
  render(
    <QuoteCostDetail
      amount={120000}
      detail={{
        estimated_credits: 2,
        exact_credits: "1.2",
        max_charge_credits: "1.2",
      }}
    />,
  );
  expect(screen.getByText("预计消耗 1.2 积分")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "费用说明" }));
  expect(screen.getByText(/最多扣 1.2 积分/)).toBeTruthy();
});
