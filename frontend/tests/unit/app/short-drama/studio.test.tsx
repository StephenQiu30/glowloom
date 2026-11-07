import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ShortDramaStudio } from "@/app/short-drama/studio";

const context = vi.hoisted(() => ({
  actor: "actor-a",
  space: "space-a",
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: context.actor } }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: { id: context.space, writable: true },
    pending: false,
  }),
}));
vi.mock("@/components/layout/basic-layout", () => ({
  useLoginDialog: () => vi.fn(),
}));
vi.mock("@/components/projects/asset-create-project", () => ({
  AssetCreateProject: () => <div role="dialog" aria-label="新建短剧项目" />,
}));
vi.mock("@/app/assets/_components/asset-workspace", () => ({
  AssetWorkspace: () => null,
}));

beforeEach(() => {
  context.actor = "actor-a";
  context.space = "space-a";
});
afterEach(cleanup);

it.each(["actor", "space"] as const)(
  "切换%s后清空短剧创作输入并关闭旧身份的创建弹窗",
  (field) => {
    const view = render(<ShortDramaStudio />);
    const editor = screen.getByLabelText("剧本创作要求");
    fireEvent.change(editor, { target: { value: "私人创作要求" } });
    fireEvent.click(screen.getByRole("button", { name: "立即生成" }));
    expect(screen.getByRole("dialog", { name: "新建短剧项目" })).toBeTruthy();

    context[field] = `${field}-b`;
    view.rerender(<ShortDramaStudio />);
    expect(screen.queryByRole("dialog", { name: "新建短剧项目" })).toBeNull();
    expect(
      (screen.getByLabelText("剧本创作要求") as HTMLTextAreaElement).value,
    ).toBe("");
    expect(
      screen.getByRole("button", { name: "立即生成" }).hasAttribute("disabled"),
    ).toBe(true);
  },
);
