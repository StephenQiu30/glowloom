import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { SkillWorkflowControl } from "@/components/canvas/skill-workflow/skill-workflow-control";
import type { SkillWorkflow } from "@/components/canvas/skill-workflow/skill-workflow-graph";
import {
  serviceDocumentFromEditor,
  serviceGraph,
} from "@/components/canvas/service-graph";
import type { YjsView } from "@/components/canvas/yjs-live-session";

vi.mock("next/dynamic", () => ({
  default: () =>
    function WorkflowConversation({ workflow }: { workflow: SkillWorkflow }) {
      return <output data-testid="workflow">{JSON.stringify(workflow)}</output>;
    },
}));

it("imports a starter into the draft and opens the saved graph for execution", async () => {
  const canvasId = "9f5cd35b-5041-4413-8755-b3f343d2c0b6";
  const projectId = "f083a17b-c878-4cbd-b0fc-d16a10f42dc3";
  const graphHash = "a".repeat(64);
  const document: API.DocumentDTO = {
    id: canvasId,
    project_id: projectId,
    name: "模板验收",
    revision: "4",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [],
    edges: [],
  };
  let draft = serviceGraph(document, canvasId, projectId);
  const prepare = vi.fn(async () => ({
    document: {
      ...serviceDocumentFromEditor(document, { ...draft, connections: [] }),
      revision: "5",
    },
    graphHash,
  }));
  const client = {
    getSnapshot: (): YjsView => ({
      document: draft,
      dto: document,
      connected: true,
      writable: true,
      dirty: false,
      migration: false,
      recovery: [],
      recoveryRemaining: 0,
      inputDrafts: {},
      unsafeToLeave: false,
      localPersistence: "MEMORY",
      message: "",
      phase: "已保存",
      canUndo: false,
      canRedo: false,
    }),
    canAcceptConnectionGraph: () => true,
    change: (graph: typeof draft) => {
      draft = graph;
    },
    waitForSavedSnapshot: prepare,
  };
  const view = () => (
    <SkillWorkflowControl
      client={client}
      graph={draft}
      canvasId={canvasId}
      projectId={projectId}
      identity="owner"
      enabled
      saved
    />
  );
  const { rerender } = render(view());
  fireEvent.click(screen.getByRole("button", { name: "技能流程" }));
  fireEvent.click(screen.getByRole("button", { name: "小说改编与分镜" }));
  expect(draft.nodes).toHaveLength(8);
  expect(draft.connections.length).toBeGreaterThan(0);
  rerender(view());
  fireEvent.click(screen.getByRole("button", { name: "技能流程" }));
  fireEvent.click(screen.getByRole("button", { name: "运行" }));
  const run = JSON.parse(
    (await screen.findByTestId("workflow")).textContent ?? "",
  ) as SkillWorkflow;
  expect(prepare).toHaveBeenCalledOnce();
  expect(run.nodes).toHaveLength(7);
  expect(run.nodes.every((node) => node.after.length === 0)).toBe(true);
  expect(run.source).toEqual({ canvasId, revision: "5", graphHash });
});
