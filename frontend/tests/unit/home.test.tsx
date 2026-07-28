import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import Home from "@/app/page";

describe("S0 home", () => {
  it("explains the current platform state", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ status: "ok" }) }),
    );
    render(<Home />);
    expect(
      screen.getByRole("heading", { name: "从剧本到成片，保持每一步可控" }),
    ).toBeInTheDocument();
    expect(screen.getByText("服务状态")).toBeInTheDocument();
  });
});
