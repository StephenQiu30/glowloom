import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { CandidatePreview } from "@/features/studio/candidate-preview";

describe("CandidatePreview", () => {
  it("reauthorizes an expired preview in place without replacing the candidate", async () => {
    const authorize = vi
      .fn<() => Promise<string>>()
      .mockRejectedValueOnce(new Error("expired"))
      .mockResolvedValueOnce("http://127.0.0.1:9000/private/preview?signature=fresh");
    const user = userEvent.setup();
    render(<CandidatePreview authorize={authorize} candidateId="candidate-1" kind="image" />);

    await user.click(screen.getByRole("button", { name: "预览候选" }));
    expect(screen.getByRole("button", { name: "重新授权预览" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "重新授权预览" }));
    expect(screen.getByRole("img", { name: "候选 candidate-1 预览" })).toHaveAttribute(
      "src",
      "http://127.0.0.1:9000/private/preview?signature=fresh",
    );
    expect(authorize).toHaveBeenCalledTimes(2);
  });
});
