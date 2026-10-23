import { describe, it, expect, vi, afterEach } from "vitest";
import {
  render as renderTesting,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from "@testing-library/react";
import {
  CanvasEditor,
  type ProductionCanvasService,
} from "@/components/canvas/editor";
import { serviceGraph } from "@/components/canvas/service-graph";
import type { CanvasDocumentIntegration } from "@/components/canvas/types/canvas-document-integration";
import { TooltipProvider } from "@/components/ui/tooltip";
const render = (ui: Parameters<typeof renderTesting>[0]) =>
  renderTesting(ui, { wrapper: TooltipProvider });
vi.mock("@/components/canvas/editor/canvas-editor", () => ({
  default: ({
    documentIntegration: integration,
  }: {
    documentIntegration: CanvasDocumentIntegration;
  }) => (
    <>
      {integration.toolbar({
        canUndo: false,
        canRedo: false,
        onHome: () => {},
        onProjects: () => {},
        onCreateProject: () => {},
        onExportProject: () => {},
        onImportImage: () => {},
        onUndo: () => {},
        onRedo: () => {},
      })}
      <button
        onClick={() =>
          integration.onChange(
            {
              ...integration.initialState,
              nodes: integration.initialState.nodes.map((node) => ({
                ...node,
                title: "修改后的标题",
              })),
            },
            true,
          )
        }
      >
        修改节点
      </button>
      <span>{integration.initialState.nodes[0]?.title}</span>
    </>
  ),
}));
const canvas = "11111111-1111-4111-8111-111111111111",
  project = "22222222-2222-4222-8222-222222222222",
  node = "33333333-3333-4333-8333-333333333333";
function service(): ProductionCanvasService {
  const graph = serviceGraph(
    {
      id: canvas,
      project_id: project,
      revision: "5",
      schema_version: 1,
      operation_version: 1,
      permissions: { read: true, write: true },
      nodes: [
        {
          id: node,
          node_type: "text",
          node_action: "resource",
          title: "原始标题",
          x: 0,
          y: 0,
          width: 320,
          height: 200,
          config: { format: "plain", format_version: 1, text: "原文" },
        },
      ],
      edges: [],
    },
    canvas,
    project,
  );
  return {
    document: graph,
    canvasID: canvas,
    projectID: project,
    revision: "5",
    name: "测试画布",
    status: "已保存",
    writable: true,
    viewStateKey: "canvas-test",
    commit: vi.fn(async () => ({
      document: {
        ...graph,
        nodes: graph.nodes.map((n) => ({ ...n, title: "修改后的标题" })),
      },
      revision: "6",
    })),
    undo: { canUndo: false, canRedo: false, undo: vi.fn(), redo: vi.fn() },
  };
}
afterEach(cleanup);
describe("正式画布保存", () => {
  it("keeps a dirty draft and its original baseline across expired-connection recovery", async () => {
    const value = service();
    value.canSubmit = false;
    const view = render(<CanvasEditor service={value} />);
    fireEvent.click(screen.getByText("修改节点"));
    view.rerender(
      <CanvasEditor
        service={{ ...value, writable: false, status: "正在续期" }}
      />,
    );
    expect(value.commit).not.toHaveBeenCalled();
    expect(screen.getByText("有未保存修改")).toBeTruthy();
    view.rerender(
      <CanvasEditor
        service={{
          ...value,
          revision: "6",
          canSubmit: true,
          document: structuredClone(value.document),
          status: "已保存",
        }}
      />,
    );
    await waitFor(() =>
      expect(value.commit).toHaveBeenCalledWith(
        expect.arrayContaining([
          {
            type: "set",
            node_id: node,
            path: ["title"],
            value: "修改后的标题",
          },
        ]),
        "6",
      ),
    );
    await waitFor(() => expect(screen.queryByText("有未保存修改")).toBeNull());
  });

  it("accepts the server snapshot only after a successful commit", async () => {
    const value = service();
    render(<CanvasEditor service={value} />);
    fireEvent.click(screen.getByText("修改节点"));
    await waitFor(() =>
      expect(value.commit).toHaveBeenCalledWith(
        [
          {
            type: "set",
            node_id: node,
            path: ["title"],
            value: "修改后的标题",
          },
        ],
        "5",
      ),
    );
    await waitFor(() => expect(screen.getByText("修改后的标题")).toBeTruthy());
    expect(screen.queryByRole("button", { name: /^保存$/ })).toBeNull();
  });
  it("keeps the original base revision while a remote snapshot arrives during a local edit", async () => {
    const value = service();
    vi.mocked(value.commit).mockImplementationOnce(() => new Promise(() => {}));
    const view = render(<CanvasEditor service={value} />);
    fireEvent.click(screen.getByText("修改节点"));
    await waitFor(() => expect(value.commit).toHaveBeenCalledOnce());
    view.rerender(
      <CanvasEditor
        service={{
          ...value,
          revision: "6",
          document: {
            ...value.document,
            nodes: value.document.nodes.map((n) => ({
              ...n,
              title: "远端标题",
            })),
          },
        }}
      />,
    );
    await waitFor(() =>
      expect(value.commit).toHaveBeenCalledWith(expect.any(Array), "5"),
    );
  });
  it("retains a rejected draft without an automatic overwrite retry", async () => {
    const value = service();
    vi.mocked(value.commit).mockRejectedValueOnce(new Error("保存冲突"));
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<CanvasEditor service={value} />);
    fireEvent.click(screen.getByText("修改节点"));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("保存冲突"),
    );
    expect(screen.getByText("输入已保留，需处理")).toBeTruthy();
    expect(value.commit).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: "采用服务器版本" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(value.commit).toHaveBeenCalledOnce();
    confirm.mockRestore();
  });
});
