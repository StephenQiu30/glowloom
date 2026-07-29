import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ComicProductionStudio } from "@/app/studio/comic-production-studio";

describe("AI 漫剧资产工作台", () => {
  it("管理角色资产并切换其他资产类型", async () => {
    const user = userEvent.setup();
    render(<ComicProductionStudio />);

    expect(screen.getByRole("heading", { name: "资产库" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "顾清禾角色设定图" })).toBeInTheDocument();
    expect(screen.getByText("18 个分镜引用此版本")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /场景\s*6/ }));
    expect(screen.getByRole("heading", { name: "顾府画阁" })).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /角色\s*4/ }));
    await user.click(screen.getByRole("button", { name: /陆沉舟头像/ }));
    expect(screen.getByRole("heading", { name: "陆沉舟" })).toBeInTheDocument();
    expect(screen.getByText("没有待处理的镜头")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "保存新版本" }));
    expect(screen.getByText("陆沉舟 v3 已保存，旧版本仍可追溯")).toBeInTheDocument();
  });
});
