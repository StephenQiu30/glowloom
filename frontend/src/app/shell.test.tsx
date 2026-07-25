import { render, screen } from "@testing-library/react";

import Home from "@/app/page";

describe("application shell", () => {
  it("presents the AI short-drama production purpose", () => {
    render(<Home />);

    expect(
      screen.getByRole("heading", { name: "把故事变成可交付的 AI 短剧" }),
    ).toBeInTheDocument();
    expect(screen.getByText("剧本 → 分镜 → 媒体 → 成片")).toBeInTheDocument();
  });
});
