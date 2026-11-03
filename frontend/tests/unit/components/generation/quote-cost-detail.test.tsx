import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { QuoteCostDetail } from "@/components/generation/quote-cost-detail";
afterEach(cleanup);
it("shows micros and separately named integer credit estimate without a ceiling promise", () => {
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
  expect(screen.getByText(/¥0.000001/)).toBeTruthy();
  expect(screen.getByText(/预估积分：1/)).toBeTruthy();
  expect(screen.getByText(/预留金额不代表最终实扣或费用上限/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "费用明细" })).toBeTruthy();
});
it("keeps missing and unit-only amounts unknown rather than free", () => {
  const { rerender } = render(
    <QuoteCostDetail
      preview={{ amount_kind: "unknown", confirmable: false }}
    />,
  );
  expect(screen.getByText(/费用待核验/)).toBeTruthy();
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
  expect(screen.getByText(/仅有单价/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: /确认/ })).toBeNull();
});
