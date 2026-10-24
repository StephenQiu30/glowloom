import { canonicalJson } from "./canonical.mjs";
import { normalizeRichText } from "./rich-text-schema.mjs";

// In-memory editor DTOs include absent optional properties as undefined. Match
// the wire JSON representation before entering the canonical shared graph.
const clone = (v) => {
  if (Array.isArray(v)) return v.map((item) => clone(item ?? null));
  if (v && typeof v === "object")
    return Object.fromEntries(
      Object.entries(v)
        .filter(([, item]) => item !== undefined)
        .map(([key, item]) => [key, clone(item)]),
    );
  return v;
};
const equal = (a, b) => canonicalJson(a) === canonicalJson(b);
const serverFields = ["content_version", "input_revision", "last_operation_id"];
const emptyDocument = () => ({ type: "doc", content: [{ type: "paragraph" }] });
function textDocument(node) {
  const stored = node.config?.rich_text ?? node.config?.editor?.document;
  if (stored)
    return { schemaVersion: 1, content: normalizeRichText(stored.content) };
  if (node.node_type !== "text") return undefined;
  const content = node.config?.composer?.document ?? {
    type: "doc",
    content: String(node.config?.text ?? "")
      .split("\n")
      .map((text) => ({
        type: "paragraph",
        ...(text ? { content: [{ type: "text", text }] } : {}),
      })),
  };
  return { schemaVersion: 1, content: normalizeRichText(content) };
}
/** Go controls authorization/revisions; only graph fields enter the shared document. */
export function serviceSnapshot(dto) {
  if (
    dto.schema_version !== 1 ||
    dto.operation_version !== 1 ||
    !Array.isArray(dto.nodes) ||
    !Array.isArray(dto.edges)
  )
    throw new Error("UNSUPPORTED_SERVICE_DOCUMENT");
  return {
    title: dto.name,
    graph: {
      schemaVersion: 2,
      serviceVersion: 1,
      documentId: dto.id,
      projectId: dto.project_id,
      nodes: dto.nodes.map((source) => {
        const native = clone(source),
          config = native.config ?? {},
          editor = config.editor;
        for (const key of [
          ...serverFields,
          "id",
          "title",
          "x",
          "y",
          "width",
          "height",
          "parent_id",
          "z_index",
        ])
          delete native[key];
        const body = textDocument(source);
        for (const key of ["rich_text", "text", "format", "format_version"])
          delete config[key];
        if (config.composer) {
          delete config.composer.document;
          delete config.composer.references;
          delete config.composer.labelState;
        }
        if (editor) {
          delete editor.document;
          delete editor.parameters;
          if (editor.metadata) delete editor.metadata.referenceLabels;
        }
        return {
          id: source.id,
          type: source.node_type === "generation" ? "config" : source.node_type,
          nodeTypeVersion: 1,
          title: source.title,
          position: { x: source.x, y: source.y },
          ...(source.width !== undefined ? { width: source.width } : {}),
          ...(source.height !== undefined ? { height: source.height } : {}),
          parentId: source.parent_id ?? null,
          zIndex: source.z_index,
          native,
          ...(source.config?.editor?.metadata?.referenceLabels
            ? {
                referenceLabels: clone(
                  source.config.editor.metadata.referenceLabels,
                ),
              }
            : {}),
          ...(source.config?.composer
            ? {
                references: clone(source.config.composer.references),
                labelState: clone(source.config.composer.labelState),
              }
            : {}),
          ...(source.config?.editor?.parameters
            ? { parameters: clone(source.config.editor.parameters) }
            : {}),
          ...(body ? { textDocument: body } : {}),
        };
      }),
      connections: dto.edges.map((edge) => ({
        id: edge.id,
        fromNodeId: edge.source_node_id,
        toNodeId: edge.target_node_id,
        native: clone(edge),
      })),
    },
  };
}
function composerProjection(content, original) {
  // The shared native config intentionally omits the derived document/registry.
  // A newly materialized shell must still project a complete service composer.
  const composer = {
    schemaVersion: 1,
    document: emptyDocument(),
    references: [],
    modelVersion: null,
    mode: null,
    params: {},
    labelState: {
      highWater: { image: 0, video: 0, audio: 0, subject: 0 },
      allocations: {},
    },
    ...clone(original ?? {}),
  };
  const paragraphs = [];
  let inline = [];
  const flush = () => {
    if (inline.length) {
      paragraphs.push({ type: "paragraph", content: inline });
      inline = [];
    }
  };
  const visit = (part) => {
    if (part.type === "text" && part.text)
      inline.push({ type: "text", text: part.text });
    else if (part.type === "hardBreak") inline.push({ type: "hardBreak" });
    else if (part.type === "reference")
      inline.push({
        type: "reference",
        attrs: { referenceId: part.attrs.referenceId },
      });
    else if (part.type === "mention") {
      const text = String(part.attrs?.token ?? part.attrs?.label ?? "");
      if (text) inline.push({ type: "text", text });
    } else {
      for (const child of part.content ?? []) visit(child);
      if (
        ["paragraph", "heading", "codeBlock", "horizontalRule"].includes(
          part.type,
        )
      )
        flush();
    }
  };
  visit(content);
  flush();
  composer.document = {
    type: "doc",
    content: paragraphs.length ? paragraphs : [{ type: "paragraph" }],
  };
  return composer;
}
/** Projection uses the current authorized DTO for server-owned versions, never client claims. */
export function serviceProjection(snapshot, base) {
  if (
    snapshot.graph.serviceVersion !== 1 ||
    snapshot.graph.documentId !== base.id ||
    snapshot.graph.projectId !== base.project_id
  )
    throw new Error("DOCUMENT_IDENTITY_MISMATCH");
  const previous = new Map(base.nodes.map((n) => [n.id, n]));
  const nodes = snapshot.graph.nodes.map((node) => {
    const original = previous.get(node.id),
      native = clone(node.native),
      config = native.config ?? {};
    const result = {
      ...native,
      id: node.id,
      title: node.title,
      x: node.position.x,
      y: node.position.y,
      ...(node.width !== undefined ? { width: node.width } : {}),
      ...(node.height !== undefined ? { height: node.height } : {}),
      parent_id: node.parentId ?? null,
      z_index: node.zIndex,
      config,
    };
    for (const key of serverFields) {
      delete result[key];
      if (original && Object.hasOwn(original, key)) result[key] = original[key];
    }
    if (node.referenceLabels) {
      config.editor ??= { schemaVersion: 1 };
      config.editor.metadata ??= {};
      config.editor.metadata.referenceLabels = clone(node.referenceLabels);
    }
    if (node.references) {
      config.composer ??= composerProjection(emptyDocument());
      config.composer.references = clone(node.references);
      config.composer.labelState = clone(node.labelState);
    } else if (config.composer && original?.config?.composer) {
      // Plain-text promotion can derive a composer before its unchanged
      // registry enters Yjs. A later native-field edit carries only its shell.
      // Explicit shared registries (including []) always take precedence.
      config.composer.references = clone(original.config.composer.references);
      config.composer.labelState = clone(original.config.composer.labelState);
    }
    if (node.parameters) {
      config.editor ??= { schemaVersion: 1 };
      config.editor.parameters = clone(node.parameters);
    }
    if (node.textDocument) {
      const old = original && textDocument(original);
      if (
        old &&
        equal(old, node.textDocument) &&
        (!node.references || original.config?.composer)
      ) {
        for (const key of ["text", "format", "format_version"])
          if (Object.hasOwn(original.config, key))
            config[key] = clone(original.config[key]);
        if (!config.composer && original.config.composer)
          config.composer = clone(original.config.composer);
        if (config.composer)
          config.composer.document = clone(original.config.composer.document);
        if (original.config.rich_text)
          config.rich_text = clone(original.config.rich_text);
        if (original.config.editor?.document) {
          config.editor ??= { schemaVersion: 1 };
          config.editor.document = clone(original.config.editor.document);
        }
      } else if (result.node_type === "text") {
        delete config.text;
        config.format = "tiptap";
        config.format_version = 1;
        config.rich_text = clone(node.textDocument);
        config.composer = composerProjection(
          node.textDocument.content,
          config.composer,
        );
      } else {
        config.editor ??= { schemaVersion: 1 };
        config.editor.document = clone(node.textDocument);
      }
    }
    return result;
  });
  return {
    ...clone(base),
    name: snapshot.title,
    nodes,
    edges: snapshot.graph.connections.map((edge) => ({
      ...clone(edge.native),
      id: edge.id,
      source_node_id: edge.fromNodeId,
      target_node_id: edge.toNodeId,
    })),
  };
}
