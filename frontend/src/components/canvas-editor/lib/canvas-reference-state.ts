import { detachSubjectSources } from "./canvas-subject-connections.ts";
import { detachTimelineSources } from "./canvas-timeline-connections.ts";
import {
  documentText,
  parseRichTextDocument,
  reconcileDocumentReferences,
} from "../runtime/components/editor/lib/rich-text-document.ts";
import { buildNodeMentionReferences } from "./canvas-resource-references.ts";
import type { CanvasNodeData, CanvasConnection } from "../types/canvas.ts";

export function reconcileCanvasReferences(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  return nodes.map((node) => {
    const document =
      node.metadata?.richText ??
      (node.documentData?.textDocument
        ? parseRichTextDocument(node.documentData.textDocument)
        : undefined);
    if (!document) return node;
    const richText = reconcileDocumentReferences(
      document,
      buildNodeMentionReferences(node, nodes, connections),
    );
    if (richText === document) return node;
    return {
      ...node,
      ...(node.documentData
        ? { documentData: { ...node.documentData, textDocument: richText } }
        : {}),
      metadata: {
        ...node.metadata,
        richText,
        ...(node.type === "text"
          ? { content: documentText(richText.content) }
          : { prompt: documentText(richText.content) }),
      },
    };
  });
}

/** Apply graph removals and inline reference cleanup as one undoable edit. */
export function removeCanvasReferences(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
  removal: {
    nodeIds?: Set<string>;
    connectionIds?: Set<string>;
    source?: { fromNodeId: string; toNodeId: string };
  },
) {
  const nextConnections = connections.filter(
    (edge) =>
      !removal.nodeIds?.has(edge.fromNodeId) &&
      !removal.nodeIds?.has(edge.toNodeId) &&
      !removal.connectionIds?.has(edge.id) &&
      !(
        removal.source?.fromNodeId === edge.fromNodeId &&
        removal.source?.toNodeId === edge.toNodeId
      ),
  );
  const nextNodes = nodes
    .filter((node) => !removal.nodeIds?.has(node.id))
    .map((node) =>
      node.metadata?.groupId && removal.nodeIds?.has(node.metadata.groupId)
        ? { ...node, metadata: { ...node.metadata, groupId: undefined } }
        : node,
    );
  return {
    nodes: reconcileCanvasReferences(
      detachSubjectSources(
        detachTimelineSources(
          nextNodes,
          connections,
          nextConnections,
          removal.nodeIds,
          nodes,
        ),
        nextConnections,
      ),
      nextConnections,
    ),
    connections: nextConnections,
  };
}
