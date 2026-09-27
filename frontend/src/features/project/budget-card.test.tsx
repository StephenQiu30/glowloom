import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { BudgetCard, type BudgetSnapshot } from "./budget-card";

afterEach(cleanup);

const budget: BudgetSnapshot = {
  limit_micros: 500_000_000,
  settled_micros: 180_000_000,
  reserved_micros: 12_000_000,
  available_micros: 308_000_000,
  is_overrun: false,
};

it("shows the supplied budget amounts and occupancy in a labelled card", () => {
  render(<BudgetCard budget={budget} />);

  const card = screen.getByRole("region", { name: "项目预算" });
  expect(within(card).getByText("¥500.00")).toBeTruthy();
  expect(within(card).getByText("¥180.00")).toBeTruthy();
  expect(within(card).getByText("¥12.00")).toBeTruthy();
  expect(within(card).getByText("¥308.00")).toBeTruthy();
  const progress = within(card).getByRole("progressbar", {
    name: "预算使用率",
  });
  expect(progress.getAttribute("aria-valuenow")).toBe("38");
  expect(progress.getAttribute("aria-valuetext")).toBe("已占用 38.4%");
});

it("treats a new project's zero limit as unset instead of low or divided by zero", () => {
  render(
    <BudgetCard
      budget={{
        limit_micros: 0,
        settled_micros: 0,
        reserved_micros: 0,
        available_micros: 0,
        is_overrun: false,
      }}
    />,
  );

  expect(screen.getByText("未设置预算")).toBeTruthy();
  expect(screen.getByText(/付费报价暂不可确认/)).toBeTruthy();
  expect(screen.queryByText("预算余额不足")).toBeNull();
  expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe(
    "0",
  );
});

it("shows low balance only below 20 percent and clears it when the snapshot recovers", () => {
  const view = render(
    <BudgetCard
      budget={{
        ...budget,
        settled_micros: 400_000_000,
        reserved_micros: 0,
        available_micros: 100_000_000,
      }}
    />,
  );
  expect(screen.queryByText("预算余额不足")).toBeNull();

  view.rerender(
    <BudgetCard
      budget={{
        ...budget,
        settled_micros: 401_000_000,
        reserved_micros: 0,
        available_micros: 99_000_000,
      }}
    />,
  );
  expect(screen.getByText("预算余额不足")).toBeTruthy();

  view.rerender(<BudgetCard budget={budget} />);
  expect(screen.queryByText("预算余额不足")).toBeNull();
});

it("prioritizes overrun, exposes the negative available amount and caps the visual bar", () => {
  render(
    <BudgetCard
      budget={{
        limit_micros: 100_000_000,
        settled_micros: 110_000_000,
        reserved_micros: 5_000_000,
        available_micros: -15_000_000,
        is_overrun: true,
      }}
    />,
  );

  expect(screen.getByText("已超支")).toBeTruthy();
  expect(screen.queryByText("预算余额不足")).toBeNull();
  expect(screen.getByText("-¥15.00")).toBeTruthy();
  const progress = screen.getByRole("progressbar");
  expect(progress.getAttribute("aria-valuenow")).toBe("100");
  expect(progress.getAttribute("aria-valuetext")).toBe("已占用 115%");
});
