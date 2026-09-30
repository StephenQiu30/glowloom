import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LiveEditor } from "./live-editor";
import { ApiError } from "@/lib/request";
import {
  applyCommands,
  type CanvasDocument,
  type CanvasCommand,
} from "./live-state";

vi.mock("next-themes", () => ({
  useTheme: () => ({ resolvedTheme: "light" }),
}));
vi.mock("@xyflow/react", () => ({
  ReactFlowProvider: ({ children }: { children: React.ReactNode }) => children,
  ReactFlow: (props: {
    nodes: { id: string; data: { text: string } }[];
    onNodeClick: (event: unknown, node: unknown) => void;
  }) => (
    <div>
      {props.nodes.map((node) => (
        <button key={node.id} onClick={() => props.onNodeClick(null, node)}>
          备注 {node.data.text}
        </button>
      ))}
    </div>
  ),
  Handle: () => null,
  Controls: () => null,
  Background: () => null,
  MiniMap: () => null,
  Position: { Left: "left", Right: "right" },
  applyNodeChanges: (_: unknown, nodes: unknown) => nodes,
  useReactFlow: () => ({
    setViewport: vi.fn(),
    getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
    screenToFlowPosition: () => ({ x: 0, y: 0 }),
  }),
}));
afterEach(cleanup);
const document: CanvasDocument = {
  id: "canvas",
  project_id: "project",
  name: "创作备注",
  scope: {},
  revision: 1,
  viewport: { x: 0, y: 0, zoom: 1 },
  nodes: [
    {
      id: "one",
      node_type: "text",
      node_action: "resource",
      config: { text: "原文" },
      x: 0,
      y: 0,
    },
  ],
  edges: [],
};
function service() {
  let stored = document;
  return vi.fn(
    async (_revision: number, commands: CanvasCommand[], _key: string) => {
      void _key;
      stored = {
        ...applyCommands(stored, commands),
        revision: stored.revision + 1,
      };
      return stored;
    },
  );
}
function mount(
  save = service(),
  reload = vi.fn(async () => document),
  readOnly = false,
) {
  render(
    <LiveEditor
      document={document}
      readOnly={readOnly}
      save={save}
      reload={reload}
      onAuthFailure={vi.fn()}
    />,
  );
  return { save, reload };
}
describe("备注服务交互", () => {
  it("服务端确认后才显示保存，撤销用新revision和新键并校准文本草稿", async () => {
    const { save } = mount();
    fireEvent.click(screen.getByRole("button", { name: "备注 原文" }));
    fireEvent.change(screen.getByLabelText("备注内容"), {
      target: { value: "已修改" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存备注" }));
    await screen.findByText("已保存至服务端 · 修订 2");
    fireEvent.click(screen.getByRole("button", { name: "撤销" }));
    await screen.findByText("已保存至服务端 · 修订 3");
    expect(
      (screen.getByLabelText("备注内容") as HTMLTextAreaElement).value,
    ).toBe("原文");
    expect(save.mock.calls[0][0]).toBe(1);
    expect(save.mock.calls[1][0]).toBe(2);
    expect(save.mock.calls[0][2]).not.toBe(save.mock.calls[1][2]);
  });
  it("网络失败保留草稿，重试同一请求使用原幂等键", async () => {
    const save = service().mockRejectedValueOnce(
      new ApiError(0, "dependency_unavailable"),
    );
    mount(save);
    fireEvent.click(screen.getByRole("button", { name: "备注 原文" }));
    fireEvent.change(screen.getByLabelText("备注内容"), {
      target: { value: "待保存内容" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存备注" }));
    await screen.findByRole("button", { name: "重试保存" });
    expect(
      (screen.getByLabelText("备注内容") as HTMLTextAreaElement).value,
    ).toBe("待保存内容");
    expect(screen.queryByText(/已保存至服务端/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "重试保存" }));
    await screen.findByText("已保存至服务端 · 修订 2");
    expect(save.mock.calls[1]).toEqual(save.mock.calls[0]);
  });
  it("版本冲突读取最新后清空撤销历史，不自动重放写入", async () => {
    const save = service().mockRejectedValueOnce(
      new ApiError(409, "revision_conflict", "test-request"),
    );
    const reload = vi.fn(async () => ({ ...document, revision: 4 }));
    mount(save, reload);
    fireEvent.click(screen.getByRole("button", { name: "新增备注" }));
    await screen.findByText(/画布已被其他页面修改/);
    expect(screen.queryByRole("button", { name: "重试保存" })).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "放弃本页失败修改并读取最新" }),
    );
    await screen.findByText(/已载入最新修订 4/);
    expect(
      (screen.getByRole("button", { name: "撤销" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(save).toHaveBeenCalledTimes(1);
  });
  it("归档只读不提交任何布局命令", async () => {
    const { save } = mount(undefined, undefined, true);
    expect(
      (screen.getByRole("button", { name: "新增备注" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "新增备注" }));
    await waitFor(() => expect(save).not.toHaveBeenCalled());
  });
});
