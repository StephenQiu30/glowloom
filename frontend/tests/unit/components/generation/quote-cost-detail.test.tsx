import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { QuoteCostDetail } from "@/components/generation/quote-cost-detail";
import { GenerationCredits } from "@/components/generation/generation-credits";
afterEach(cleanup);
it("shows the generation preview and confirmation maximum together", async () => {
  render(
    <GenerationCredits
      preview={{
        status: "ready",
        estimate: {
          credits: "25",
          maximum_credits: "75",
          kind: "estimate",
          basis: "video_tokens",
          quantity: "108000",
        },
      }}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "预计消耗 25 积分，最多冻结 75 积分",
    }),
  );
  expect(await screen.findByText("最多冻结 75 积分")).toBeTruthy();
  expect(screen.getByText("计量 108000 视频 Token")).toBeTruthy();
});

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
it("shows the supplier's 60 second unit without labeling it as a per-second price", () => {
  render(
    <QuoteCostDetail
      amount={2000000}
      detail={{
        unit: "per_second",
        unit_quantity: 60,
        unit_price_micros: 1000000,
        quantity: 120,
        exact_credits: "20",
        max_charge_credits: "20",
      }}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "费用说明" }));
  expect(screen.getByText("每60秒")).toBeTruthy();
  expect(screen.queryByText("每秒")).toBeNull();
});

it("shows only the spending cap when original audio duration is unknown", () => {
  render(
    <QuoteCostDetail
      amount={2000000}
      detail={{
        pricing_strategy: "original_duration_v1",
        exact_credits: "20",
        max_charge_credits: "20",
      }}
    />,
  );
  expect(screen.getByText("最多扣 20 积分")).toBeTruthy();
  expect(screen.queryByText(/预计消耗/)).toBeNull();
});

it("shows the video Token estimate and the independently confirmed cap", () => {
  render(
    <QuoteCostDetail
      amount={10_000_000}
      detail={{
        unit: "per_million_video_tokens",
        estimated_tokens: 86_400,
        actual_tokens: 93_600,
        estimated_credits: 87,
        exact_credits: "87",
        max_charge_credits: "100",
      }}
    />,
  );
  expect(screen.getByText("预计消耗 87 积分")).toBeTruthy();
  expect(screen.getByText("本次最多扣 100 积分")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "费用说明" }));
  expect(screen.getByText("每百万视频 Token")).toBeTruthy();
  expect(screen.getByText("93600")).toBeTruthy();
  expect(screen.queryByText(/不代表最终实扣或费用上限/)).toBeNull();
});
