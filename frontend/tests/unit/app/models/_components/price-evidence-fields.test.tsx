import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { PriceEvidenceFields } from "@/app/models/_components/price-evidence-fields";

afterEach(cleanup);

it("只展示可编辑的官方价格来源", () => {
  render(<PriceEvidenceFields sourceURL="https://example.invalid/pricing" />);
  const source = screen.getByLabelText<HTMLInputElement>("官方价格来源");
  expect(source.required).toBe(true);
  expect(source.value).toBe("https://example.invalid/pricing");
  expect(screen.queryByLabelText("核验时间")).toBeNull();
});
