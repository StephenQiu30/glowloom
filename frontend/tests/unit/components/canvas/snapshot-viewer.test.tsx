import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ServiceCanvasWorkspace } from "@/components/canvas/service-workspace";

const fixture = vi.hoisted(() => ({
  scope: "10000000-0000-4000-8000-000000000006",
  canvas: "10000000-0000-4000-8000-000000000001",
  project: "10000000-0000-4000-8000-000000000002",
  get: vi.fn(),
  start: vi.fn(),
  submit: vi.fn(),
  configure: vi.fn(),
  renew: vi.fn(),
}));
vi.mock("@/components/auth/session-renewal", () => ({
  ensureSessionFresh: fixture.renew,
}));
vi.mock("@/api/canvases", () => ({
  getCanvas: fixture.get,
  locateCanvas: async () => ({
    canvas_id: fixture.canvas,
    project_id: fixture.project,
    scope_id: fixture.scope,
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  useSearchParams: () =>
    new URLSearchParams({
      canvas_id: fixture.canvas,
      project_id: fixture.project,
    }),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: "actor" } }),
  SessionGate: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: { id: fixture.scope },
    spaces: [{ id: fixture.scope }],
    pending: false,
    error: "",
  }),
  SpaceGate: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/components/canvas/live-session", () => ({
  CanvasLiveSession: class {
    constructor(config: unknown) {
      fixture.configure(config);
    }
    start = fixture.start;
    submit = fixture.submit;
    stop() {}
  },
}));
vi.mock("@/components/canvas/editor", () => ({
  CanvasEditor: ({
    service,
  }: {
    service: import("@/components/canvas/editor").ProductionCanvasService;
  }) => (
    <div>
      {service.document.nodes.map((node) => (
        <div data-node-id={node.id} key={node.id}>
          {node.metadata?.prompt ?? node.metadata?.content}
        </div>
      ))}
      {service.document.connections.map((edge) => (
        <div data-connection-id={edge.id} key={edge.id} />
      ))}
    </div>
  ),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const document: API.DocumentDTO = {
  id: fixture.canvas,
  project_id: fixture.project,
  name: "已有画布",
  revision: "33",
  schema_version: 1,
  operation_version: 1,
  permissions: { read: true, write: true },
  nodes: [
    {
      id: "10000000-0000-4000-8000-000000000003",
      node_type: "image_generation",
      node_action: "generate",
      title: "历史生成",
      x: 0,
      y: 0,
      width: 271,
      height: 253,
      config: { draft: { prompt: "保留历史提示词" } },
    },
    {
      id: "10000000-0000-4000-8000-000000000004",
      node_type: "text",
      node_action: "resource",
      title: "已有文字",
      x: 350,
      y: 0,
      width: 300,
      height: 200,
      config: { text: "保留正文", format: "plain", format_version: 1 },
    },
  ],
  edges: [
    {
      id: "10000000-0000-4000-8000-000000000005",
      source_node_id: "10000000-0000-4000-8000-000000000003",
      target_node_id: "10000000-0000-4000-8000-000000000004",
      edge_type: "reference",
    },
  ],
};
it("opens mixed historical nodes in the formal editor without submitting writes", async () => {
  const original = JSON.stringify(document);
  fixture.get.mockResolvedValue(document);
  const { container } = render(<ServiceCanvasWorkspace />);
  expect(await screen.findByText("保留历史提示词")).not.toBeNull();
  expect(screen.getByText("保留正文")).not.toBeNull();
  expect(container.querySelectorAll("[data-node-id]")).toHaveLength(2);
  expect(container.querySelectorAll("[data-connection-id]")).toHaveLength(1);
  expect(fixture.start).toHaveBeenCalledOnce();
  expect(fixture.submit).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "添加文本" })).toBeNull();
  expect(JSON.stringify(document)).toBe(original);
});
it("does not use historical preview to bypass canvas identity", async () => {
  fixture.get.mockResolvedValue({ ...document, project_id: fixture.canvas });
  render(<ServiceCanvasWorkspace />);
  await waitFor(() =>
    expect(screen.getByText(/暂时无法读取内容/)).not.toBeNull(),
  );
  expect(screen.queryByText("保留历史提示词")).toBeNull();
  expect(fixture.start).not.toHaveBeenCalled();
});

it("uses the shared renewal coordinator and refuses a changed account", async () => {
  fixture.get.mockResolvedValue(document);
  render(<ServiceCanvasWorkspace />);
  await screen.findByText("保留正文");
  const config = fixture.configure.mock.calls[0][0] as {
    renewSession: (signal: AbortSignal) => Promise<void>;
  };
  const signal = new AbortController().signal;
  fixture.renew.mockResolvedValueOnce({ actor: { id: "actor" } });
  await expect(config.renewSession(signal)).resolves.toBeUndefined();
  expect(fixture.renew).toHaveBeenCalledWith(signal, true);
  fixture.renew.mockResolvedValueOnce({ actor: { id: "different-actor" } });
  await expect(config.renewSession(signal)).rejects.toThrow("账号已变更");
});
