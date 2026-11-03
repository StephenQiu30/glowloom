import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BillingOutcomeView } from "@/components/billing/billing-outcome";
afterEach(cleanup);
it("separates paid charge from pending supplier evidence and restores only original task", () => {
  const recover = vi.fn();
  render(
    <BillingOutcomeView
      outcome={{
        charge_state: "settled",
        charged_micros: 19,
        provider_settlement_state: "pending",
        delivery_state: "recoverable",
        regenerate: "blocked",
        reconciliation_required: true,
      }}
      onRecover={recover}
    />,
  );
  expect(screen.getByText(/实扣：¥0.000019/)).toBeTruthy();
  expect(screen.getByText(/费用仍需核查/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "继续取回原任务" }));
  expect(recover).toHaveBeenCalledOnce();
  expect(screen.queryByText(/退款完成/)).toBeNull();
});
it("never infers refund from a failed or unreadable settlement", () => {
  render(<BillingOutcomeView />);
  expect(screen.getByText(/费用状态待核验/)).toBeTruthy();
  expect(screen.queryByText(/¥0/)).toBeNull();
});
