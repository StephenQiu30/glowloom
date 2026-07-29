import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import Home from "@/app/page";

describe("S0 home", () => {
  it("starts a mock comic project from a story idea", async () => {
    const user = userEvent.setup();
    render(<Home />);

    expect(screen.getByRole("heading", { name: "今天，想把什么故事变成漫剧？" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "开始创作" })).toBeDisabled();
    expect(screen.getByRole("link", { name: "打开项目 她从画中来" })).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "漫剧创作描述" }), "一名画师发现自己画中的长安会在午夜苏醒");
    await user.click(screen.getByRole("button", { name: "开始创作" }));
    expect(screen.getByRole("status")).toHaveTextContent("项目草稿已生成");
  });
});
