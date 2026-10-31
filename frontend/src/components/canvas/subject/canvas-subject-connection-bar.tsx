import type { CanvasEditorState } from "../lib/canvas-document-adapter";
import { canvasSubjectSources } from "../lib/canvas-subject-source";

/** Presence is graph state; source authorization remains the generation owner's job. */
export function connectedCanvasSubjects(
  graph: CanvasEditorState,
  nodeID: string,
) {
  const sources = canvasSubjectSources(graph.nodes, "");
  return graph.connections
    .filter(
      (edge) =>
        edge.toNodeId === nodeID &&
        edge.documentData?.toPortId === "reference_subjects",
    )
    .toSorted(
      (a, b) =>
        a.documentData!.order - b.documentData!.order ||
        a.id.localeCompare(b.id),
    )
    .flatMap((edge) => {
      const node = graph.nodes.find(
        (item) => item.id === edge.fromNodeId && item.type === "subject",
      );
      return node
        ? [
            {
              edgeID: edge.id,
              title: node.title,
              hasContent: sources.some((source) => source.nodeID === node.id),
            },
          ]
        : [];
    });
}

export function CanvasSubjectConnectionBar({
  graph,
  nodeID,
}: {
  graph: CanvasEditorState | null;
  nodeID: string;
}) {
  const subjects = graph ? connectedCanvasSubjects(graph, nodeID) : [];
  if (!subjects.length) return null;
  return (
    <section
      aria-label="连接的主体"
      className="space-y-2 rounded-xl border p-3 text-sm"
    >
      <p>连接主体 · {subjects.length}</p>
      <ul className="flex flex-wrap gap-2">
        {subjects.map((subject) => (
          <li key={subject.edgeID} className="rounded-md bg-muted px-2 py-1">
            {subject.title} ·{" "}
            {subject.hasContent
              ? "生成时核验固定内容"
              : "待添加主媒体，不用于生成"}
          </li>
        ))}
      </ul>
    </section>
  );
}
