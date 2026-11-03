import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { PriceEvidenceFields } from "@/app/models/_components/price-evidence-fields";
afterEach(cleanup);
it("requires explicit reviewed source without inventing provenance or a review timestamp", () => {
  render(
    <PriceEvidenceFields
      price={{
        source_url: "https://example.invalid/pricing",
        reviewed_at: "2026-11-01T01:00:00Z",
      }}
    />,
  );
  const url = screen.getByLabelText("价格来源链接") as HTMLInputElement;
  const time = screen.getByLabelText("核验时间") as HTMLInputElement;
  expect(url.required).toBe(true);
  expect(time.required).toBe(true);
  expect(url.value).toBe("");
  expect(time.value).toBe("");
  expect(screen.queryByText(/https:\/\/example.invalid\/pricing/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "上次核验信息" }));
  expect(screen.getByText(/https:\/\/example.invalid\/pricing/)).toBeTruthy();
  expect(screen.getByText(/2026-11-01T01:00:00Z/)).toBeTruthy();
});
