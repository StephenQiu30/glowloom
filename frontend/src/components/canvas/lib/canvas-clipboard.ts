import type {
  CanvasNodeData,
  CanvasConnection,
  Position,
} from "../types/canvas";

export type CanvasClipboard = {
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
};

/** Capture immutable content, including descendants of selected groups. */
export function captureCanvasSelection(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
  selected: Set<string>,
): CanvasClipboard {
  const ids = new Set(selected);
  let changed = true;
  while (changed) {
    changed = false;
    for (const node of nodes) {
      if (
        node.metadata?.groupId &&
        ids.has(node.metadata.groupId) &&
        !ids.has(node.id)
      ) {
        ids.add(node.id);
        changed = true;
      }
    }
  }
  return structuredClone({
    nodes: nodes.filter((node) => ids.has(node.id)),
    connections: connections.filter(
      (edge) => ids.has(edge.fromNodeId) && ids.has(edge.toNodeId),
    ),
  });
}

/** IDs and parents are target-local; fixed media references retain their identity. */
export function instantiateCanvasClipboard(
  clipboard: CanvasClipboard,
  offset: Position,
  newID: () => string = () => crypto.randomUUID(),
): CanvasClipboard {
  const copy = structuredClone(clipboard);
  const ids = new Map(copy.nodes.map((node) => [node.id, newID()]));
  for (const node of copy.nodes) {
    node.id = ids.get(node.id)!;
    node.title = `${node.title} 副本`;
    node.position.x += offset.x;
    node.position.y += offset.y;
    if (node.metadata?.groupId)
      node.metadata.groupId = ids.get(node.metadata.groupId);
    if (node.documentData) {
      node.documentData.id = node.id;
      node.documentData.parentId = node.metadata?.groupId;
      node.documentData.title = node.title;
      node.documentData.position = { ...node.position };
    }
    if (node.serviceData) {
      node.serviceData.id = node.id;
      delete node.serviceData.input_revision;
      delete node.serviceData.content_version;
      delete node.serviceData.last_operation_id;
      node.serviceData.parent_id = node.metadata?.groupId;
      node.serviceData.title = node.title;
      node.serviceData.x = node.position.x;
      node.serviceData.y = node.position.y;
      for (const object of node.serviceData.config?.director?.objects ?? []) {
        if (object.source_node_id)
          object.source_node_id =
            ids.get(object.source_node_id) ?? object.source_node_id;
      }
      for (const clip of node.serviceData.config?.timeline?.clips ?? []) {
        if (clip.node_id) clip.node_id = ids.get(clip.node_id) ?? clip.node_id;
      }
      const subject = node.serviceData.config?.subject?.local;
      if (subject) {
        for (const media of [
          subject.primary,
          ...(subject.references ?? []),
          subject.voice?.media,
        ]) {
          if (!media?.source_node_id) continue;
          const source = ids.get(media.source_node_id);
          if (source) media.source_node_id = source;
          else delete media.source_node_id;
        }
      }
    }
    delete node.referenceProjection;
  }
  copy.connections = copy.connections.flatMap((edge) => {
    const from = ids.get(edge.fromNodeId),
      to = ids.get(edge.toNodeId);
    if (!from || !to) return [];
    edge.id = newID();
    edge.fromNodeId = from;
    edge.toNodeId = to;
    if (edge.documentData) {
      edge.documentData.id = edge.id;
      edge.documentData.fromNodeId = from;
      edge.documentData.toNodeId = to;
    }
    if (edge.serviceData) {
      edge.serviceData.id = edge.id;
      edge.serviceData.source_node_id = from;
      edge.serviceData.target_node_id = to;
    }
    return [edge];
  });
  return copy;
}
