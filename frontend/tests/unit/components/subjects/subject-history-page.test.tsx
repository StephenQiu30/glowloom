import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SubjectHistoryPage } from "@/app/subjects/history/_components/subject-history-page";

const state = vi.hoisted(() => ({ scope: "team-one", props: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: "actor" } }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ selected: { id: state.scope } }),
}));
vi.mock("@/components/projects/project-picker", () => ({
  ProjectPicker: ({ onChange }: { onChange: (project: unknown) => void }) => (
    <button
      onClick={() => onChange({ id: "legacy-project", name: "历史项目" })}
    >
      选择历史项目
    </button>
  ),
}));
vi.mock("next/dynamic", () => ({
  default: () => (props: unknown) => {
    state.props(props);
    return <p>原项目主体列表</p>;
  },
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  state.scope = "team-one";
});

it("明确保留项目历史入口，以原项目与三种kind读取旧列表", () => {
  render(<SubjectHistoryPage />);
  expect(
    screen.getByRole("link", { name: "返回主体" }).getAttribute("href"),
  ).toBe("/subjects");
  expect(screen.getByText("选择项目，查看历史主体")).toBeTruthy();
  expect(state.props).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "选择历史项目" }));
  expect(state.props).toHaveBeenLastCalledWith({
    projectID: "legacy-project",
    kind: "character",
  });
  fireEvent.mouseDown(screen.getByRole("tab", { name: "场景" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(state.props).toHaveBeenLastCalledWith({
    projectID: "legacy-project",
    kind: "location",
  });
  fireEvent.mouseDown(screen.getByRole("tab", { name: "物品" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(state.props).toHaveBeenLastCalledWith({
    projectID: "legacy-project",
    kind: "prop",
  });
});

it("切换空间后移除原项目选择，避免沿用上一个空间历史", () => {
  const view = render(<SubjectHistoryPage />);
  fireEvent.click(screen.getByRole("button", { name: "选择历史项目" }));
  expect(screen.getByText("原项目主体列表")).toBeTruthy();
  state.scope = "team-two";
  view.rerender(<SubjectHistoryPage />);
  expect(screen.queryByText("原项目主体列表")).toBeNull();
  expect(screen.getByText("选择项目，查看历史主体")).toBeTruthy();
});
