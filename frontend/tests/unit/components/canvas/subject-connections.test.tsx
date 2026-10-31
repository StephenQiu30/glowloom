import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  connectCanvasDocument,
  supportsCanvasConnection,
} from "@/components/canvas/lib/canvas-document-connections";
import {
  buildConnectedNodeMap,
  buildNodeMentionReferences,
} from "@/components/canvas/lib/canvas-resource-references";
import {
  serviceGraph,
  serviceDocumentFromEditor,
} from "@/components/canvas/service-graph";
import {
  CanvasSubjectConnectionBar,
  connectedCanvasSubjects,
} from "@/components/canvas/subject/canvas-subject-connection-bar";
import { removeCanvasReferences } from "@/components/canvas/lib/canvas-reference-state";
import { setSubjectConfig } from "@/components/canvas/subject/subject-node-state";

afterEach(cleanup);
const canvasID = "11111111-1111-4111-8111-111111111111";
const projectID = "22222222-2222-4222-8222-222222222222";
function fixture(kind = "image") {
  const dto: API.DocumentDTO = {
    id: canvasID,
    project_id: projectID,
    schema_version: 1,
    operation_version: 1,
    revision: "1",
    nodes: [],
    edges: [],
    permissions: { read: true, write: true },
  };
  const state = serviceGraph(dto, canvasID, projectID);
  const source = createCanvasNode("subject", { x: 0, y: 0 });
  source.title = "主体甲";
  const target = createCanvasNode(kind, { x: 500, y: 0 });
  const director = createCanvasNode("director", { x: 0, y: 500 });
  state.nodes = [target, director, source];
  return { dto, state, source, target, director };
}

it.each(["image", "video", "audio"])(
  "connects empty subject to %s atomically and saves the formal edge",
  (kind) => {
    const { dto, state, source, target, director } = fixture(kind);
    const linked = connectCanvasDocument(state, source.id, target.id);
    expect(linked.nodes.map((n) => n.id)).toEqual(state.nodes.map((n) => n.id));
    expect(linked.nodes.find((n) => n.id === director.id)).toBe(director);
    expect(linked.connections).toHaveLength(1);
    expect(linked.connections[0].documentData).toMatchObject({
      fromPortId: "subject",
      toPortId: "reference_subjects",
      purpose: "REFERENCE_SUBJECT",
      binding: { kind: "NODE_VALUE" },
    });
    expect(connectCanvasDocument(linked, source.id, target.id)).toBe(linked);
    const saved = serviceDocumentFromEditor(dto, linked);
    const reopened = serviceGraph(saved, canvasID, projectID);
    expect(reopened.connections[0].documentData).toEqual(
      linked.connections[0].documentData,
    );
    expect(
      reopened.nodes.find((n) => n.id === source.id)?.serviceData?.config
        ?.subject,
    ).toEqual({ version: 1 });
    expect(
      buildNodeMentionReferences(
        reopened.nodes.find((n) => n.id === target.id)!,
        reopened.nodes,
        reopened.connections,
      ),
    ).toEqual([]);
    expect(
      buildConnectedNodeMap(reopened.nodes, reopened.connections).get(
        target.id,
      )?.[0].referenceProjection,
    ).toBeUndefined();
    expect(connectedCanvasSubjects(reopened, target.id)).toEqual([
      { edgeID: linked.connections[0].id, title: "主体甲", hasContent: false },
    ]);
  },
);

it("rejects unsupported subject targets without changing the graph", () => {
  for (const kind of ["text", "subject", "timeline", "director"]) {
    const { state, source, target } = fixture(kind);
    expect(supportsCanvasConnection("subject", kind)).toBe(false);
    expect(() => connectCanvasDocument(state, source.id, target.id)).toThrow(
      "没有可用",
    );
    expect(state.connections).toEqual([]);
  }
});

it("shows empty state honestly, follows pin fill, removal and graph restoration", () => {
  const { state, source, target } = fixture();
  const linked = connectCanvasDocument(state, source.id, target.id);
  const view = render(
    <CanvasSubjectConnectionBar graph={linked} nodeID={target.id} />,
  );
  expect(
    screen.getByRole("region", { name: "连接的主体" }).textContent,
  ).toContain("待添加主媒体，不用于生成");
  const filled = {
    ...linked,
    nodes: linked.nodes.map((n) =>
      n.id === source.id
        ? setSubjectConfig(n, {
            version: 1,
            subject_id: canvasID,
            version_id: projectID,
          })
        : n,
    ),
  };
  view.rerender(
    <CanvasSubjectConnectionBar graph={filled} nodeID={target.id} />,
  );
  expect(screen.getByRole("region").textContent).toContain(
    "生成时核验固定内容",
  );
  const removed = {
    ...filled,
    ...removeCanvasReferences(filled.nodes, filled.connections, {
      nodeIds: new Set([source.id]),
    }),
  };
  view.rerender(
    <CanvasSubjectConnectionBar graph={removed} nodeID={target.id} />,
  );
  expect(screen.queryByRole("region")).toBeNull();
  view.rerender(
    <CanvasSubjectConnectionBar graph={filled} nodeID={target.id} />,
  );
  expect(screen.getAllByRole("listitem")).toHaveLength(1);
  view.rerender(<CanvasSubjectConnectionBar graph={null} nodeID={target.id} />);
  expect(screen.queryByRole("region")).toBeNull();
});
