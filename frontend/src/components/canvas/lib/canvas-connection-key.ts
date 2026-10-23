/** Match the backend's NON_NULL canonical binding, including nested selections. */
function canonicalBinding(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalBinding);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value)
      .filter(([, item]) => item != null)
      .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
      .map(([key, item]) => [key, canonicalBinding(item)]),
  );
}

export function canvasConnectionKey(
  edge: CanvasContract.CanvasDocumentConnectionDTO,
) {
  return JSON.stringify([
    edge.fromNodeId,
    edge.fromPortId,
    edge.toNodeId,
    edge.toPortId,
    canonicalBinding(edge.binding),
  ]);
}

/** Keep the first edge's identity and order; distinct bindings remain untouched. */
export function deduplicateCanvasConnections(
  edges: CanvasContract.CanvasDocumentConnectionDTO[],
) {
  const seen = new Set<string>();
  const unique = edges.filter((edge) => {
    const key = canvasConnectionKey(edge);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  return unique.length === edges.length ? edges : unique;
}
