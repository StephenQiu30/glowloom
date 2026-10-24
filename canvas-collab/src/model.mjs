import * as Y from "yjs";
import {
  modelIdentity,
  canonicalGraphJson,
  canonicalJson,
} from "./canonical.mjs";
import { sha256 } from "./binary.mjs";

import {
  prosemirrorJSONToYXmlFragment,
  yXmlFragmentToProsemirrorJSON,
} from "@tiptap/y-tiptap";
import { normalizeRichText, richTextSchema } from "./rich-text-schema.mjs";

export function canvasGraphHash(graph) {
  return sha256(new TextEncoder().encode(canonicalGraphJson(graph)));
}

const copy = (value) => structuredClone(value);
const fail = (code) => Object.assign(new Error(code), { code });
const emptyText = { type: "doc", content: [{ type: "paragraph" }] };
const rootFields = new Set([
  "modelVersion",
  "title",
  "properties",
  "nodes",
  "connections",
  "restoration",
]);
const nodeFields = new Set([
  "data",
  "placement",
  "size",
  "order",
  "body",
  "documentPresent",
  "parameters",
]);

function map(value) {
  return new Y.Map(Object.entries(copy(value)));
}
function root(doc) {
  const value = doc.getMap("canvas");
  for (const key of value.keys())
    if (!rootFields.has(key)) throw fail("UNREGISTERED_FIELD");
  if (value.get("modelVersion") !== 1) throw fail("INVALID_CANVAS_MODEL");
  if (!value.has("restoration")) throw fail("INVALID_CANVAS_MODEL");
  validateRestoration(value.get("restoration"));
  for (const name of ["nodes", "connections", "properties"])
    if (!(value.get(name) instanceof Y.Map)) throw fail("INVALID_CANVAS_MODEL");
  return value;
}

function validateRestoration(value) {
  if (value === null) return;
  if (
    !value ||
    Object.getPrototypeOf(value) !== Object.prototype ||
    Object.keys(value).sort().join(",") !==
      "connections,epoch,id,nodes,sourceCommitSeq" ||
    !/^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/.test(value.id) ||
    !/^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/.test(value.epoch) ||
    !/^(0|[1-9][0-9]{0,18})$/.test(value.sourceCommitSeq)
  )
    throw fail("INVALID_RESTORATION");
  for (const [name, limit] of [
    ["nodes", 500],
    ["connections", 1000],
  ]) {
    const ids = value[name];
    if (
      !Array.isArray(ids) ||
      ids.length > limit ||
      new Set(ids).size !== ids.length ||
      ids.some(
        (id) => typeof id !== "string" || !/^[A-Za-z0-9_-]{1,64}$/.test(id),
      )
    )
      throw fail("INVALID_RESTORATION");
  }
  if (!value.nodes.length && !value.connections.length)
    throw fail("INVALID_RESTORATION");
}

function anchor(type) {
  const position = Y.createRelativePositionFromTypeIndex(type, 0);
  return JSON.stringify(position.type ?? position.tname);
}

/** Stable container identity through the public relative-position API. */
export function inspectCanvasDocument(doc) {
  const canvas = root(doc);
  const inspect = (collection, nodes) =>
    Object.fromEntries(
      [...collection.entries()].map(([id, value]) => {
        if (!(value instanceof Y.Map)) throw fail("INVALID_CANVAS_MODEL");
        const entry = { self: anchor(value) };
        if (nodes) {
          for (const field of value.keys())
            if (!nodeFields.has(field)) throw fail("UNREGISTERED_FIELD");
          const data = value.get("data");
          if (!(data instanceof Y.Map) || data.get("id") !== id)
            throw fail("IMMUTABLE_FIELD");
          entry.data = anchor(data);
          entry.kind = data.get("type");
          entry.version = data.get("nodeTypeVersion");
          entry.present = value.get("documentPresent");
          if (value.get("body")) entry.body = anchor(value.get("body"));
          if (value.get("parameters")) {
            entry.parameters = anchor(value.get("parameters"));
            entry.parameterValues = anchor(
              value.get("parameters").get("values"),
            );
          }
        } else if (value.get("data")?.id !== id) throw fail("IMMUTABLE_FIELD");
        return [id, entry];
      }),
    );
  return {
    containers: Object.fromEntries(
      ["nodes", "connections", "properties"].map((name) => [
        name,
        anchor(canvas.get(name)),
      ]),
    ),
    properties: copy(canvas.get("properties").toJSON()),
    restoration: copy(canvas.get("restoration")),
    nodes: inspect(canvas.get("nodes"), true),
    connections: inspect(canvas.get("connections"), false),
  };
}

export function validateCanvasTransition(before, doc) {
  const after = inspectCanvasDocument(doc);
  const restoration = equal(before.restoration, after.restoration)
    ? null
    : after.restoration;
  if (!equal(before.properties, after.properties))
    throw fail("READ_ONLY_PROPERTY");
  if (!equal(before.containers, after.containers))
    throw fail("REPLACED_SHARED_TYPE");
  for (const name of ["nodes", "connections"])
    for (const [id, previous] of Object.entries(before[name])) {
      const next = after[name][id];
      if (!next) continue;
      for (const field of [
        "self",
        "data",
        "body",
        "parameters",
        "parameterValues",
      ])
        if (
          previous[field] !== undefined &&
          previous[field] !== next[field] &&
          !restoration?.[name].includes(id)
        )
          throw fail("REPLACED_SHARED_TYPE");
      if (
        previous.kind !== next.kind ||
        previous.version !== next.version ||
        (previous.present === true && next.present !== true)
      )
        throw fail("IMMUTABLE_FIELD");
    }
  return restoration;
}

// DTOs may omit optional fields with undefined; compare their JSON value, independent of key order.
const jsonValue = (value) => {
  if (!value || typeof value !== "object" || JSON.isRawJSON(value))
    return value;
  if (Array.isArray(value))
    return value.map((item) => (item === undefined ? null : jsonValue(item)));
  return Object.fromEntries(
    Object.entries(value)
      .filter(([, item]) => item !== undefined)
      .map(([key, item]) => [key, jsonValue(item)]),
  );
};
const equal = (a, b) =>
  a === b ||
  (a !== undefined &&
    b !== undefined &&
    canonicalJson(jsonValue(a)) === canonicalJson(jsonValue(b)));

function updateFields(target, before, after, immutable = []) {
  for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
    if (equal(before[key], after[key])) continue;
    if (immutable.includes(key)) throw fail("IMMUTABLE_FIELD");
    if (Object.hasOwn(after, key)) target.set(key, copy(after[key]));
    else target.delete(key);
  }
}

function referenceData(data, references) {
  if (!references) return data;
  if (
    !Array.isArray(references) ||
    references.length > 255 ||
    new Set(references.map((r) => r.id)).size !== references.length
  )
    throw fail("INVALID_REFERENCES");
  return {
    ...data,
    referencesPresent: true,
    ...Object.fromEntries(
      references.map((r) => ["reference:" + r.id, copy(r)]),
    ),
  };
}

function nodeParts(source) {
  const {
    position,
    parentId,
    width,
    height,
    textDocument,
    parameters,
    references,
    ...data
  } = source;
  return {
    data: referenceData(data, references),
    placement: {
      position,
      ...(Object.hasOwn(source, "parentId") ? { parentId } : {}),
    },
    size: {
      ...(width !== undefined ? { width } : {}),
      ...(height !== undefined ? { height } : {}),
    },
    textDocument,
    parameters,
  };
}

function addParameters(node, source, present = true) {
  if (source.modelSelection) modelIdentity(source.modelSelection.connectionId);
  const { values, ...config } = copy(source);
  const parameters = new Y.Map();
  node.set("parameters", parameters);
  parameters.set("present", present);
  parameters.set("config", config);
  parameters.set("valuesPresent", Object.hasOwn(source, "values"));
  parameters.set("values", map(values ?? {}));
}

function patchParameters(node, before, after) {
  if (equal(before, after)) return;
  if (!before) {
    if (!node.get("parameters")) {
      addParameters(node, after);
      return;
    }
    before = {};
  }
  if (!after) throw fail("INVALID_PARAMETERS");
  const parameters = node.get("parameters");
  parameters.set("present", true);
  const { values: oldValues, ...oldConfig } = before;
  const { values: newValues, ...newConfig } = after;
  if (!equal(oldConfig, newConfig)) parameters.set("config", copy(newConfig));
  updateFields(parameters.get("values"), oldValues ?? {}, newValues ?? {});
  if (Object.hasOwn(before, "values") !== Object.hasOwn(after, "values"))
    parameters.set("valuesPresent", Object.hasOwn(after, "values"));
}

/** Apply only explicit local differences against the last rendered projection. */
export function applyCanvasChanges(
  doc,
  before,
  after,
  origin = "local-canvas",
) {
  after = copy(after);
  const canvas = root(doc);
  const {
    nodes: previousNodes,
    connections: previousEdges,
    ...previousProperties
  } = before.graph;
  const {
    nodes: nextNodes,
    connections: nextEdges,
    ...nextProperties
  } = after.graph;
  if (!equal(previousProperties, nextProperties))
    throw fail("READ_ONLY_PROPERTY");
  const prepare = (previous, next, name) => {
    if (new Set(next.map((n) => n.id)).size !== next.length)
      throw fail("DUPLICATE_IDENTITY");
    const collection = canvas.get(name),
      old = new Map(previous.map((item) => [item.id, item]));
    const ids = new Set(next.map((item) => item.id));
    const actions = [];
    for (const item of previous)
      if (!ids.has(item.id)) actions.push(() => collection.delete(item.id));
    next.forEach((item, order) => {
      if (item.parameters?.modelSelection)
        modelIdentity(item.parameters.modelSelection.connectionId);
      const baseline = old.get(item.id),
        current = collection.get(item.id);
      if (!baseline) {
        if (current) throw fail("IDENTITY_COLLISION");
        if (item.textDocument) normalizeRichText(item.textDocument.content);
        actions.push(() =>
          name === "nodes"
            ? addNode(collection, item, order)
            : collection.set(item.id, map({ data: item, order })),
        );
        return;
      }
      if (
        equal(baseline, item) &&
        previous.findIndex((p) => p.id === item.id) === order
      )
        return;
      if (!current) throw fail("TARGET_DELETED");
      if (name === "nodes") {
        const a = nodeParts(baseline),
          b = nodeParts(item);
        if (a.parameters && !b.parameters) throw fail("INVALID_PARAMETERS");
        for (const key of ["id", "type", "nodeTypeVersion"])
          if (!equal(a.data[key], b.data[key])) throw fail("IMMUTABLE_FIELD");
        if (!equal(a.textDocument, b.textDocument)) {
          const actual = readCanvasDocument(doc).graph.nodes.find(
            (n) => n.id === item.id,
          )?.textDocument;
          if (!equal(actual, b.textDocument)) throw fail("USE_TEXT_BINDING");
        }
        actions.push(() => {
          updateFields(current.get("data"), a.data, b.data, [
            "id",
            "type",
            "nodeTypeVersion",
          ]);
          patchParameters(current, a.parameters, b.parameters);
          if (!equal(a.placement, b.placement))
            current.set("placement", copy(b.placement));
          if (!equal(a.size, b.size)) current.set("size", copy(b.size));
        });
      } else if (!equal(baseline, item))
        actions.push(() => current.set("data", copy(item)));
      if (previous.findIndex((p) => p.id === item.id) !== order)
        actions.push(() => current.set("order", order));
    });
    return actions;
  };
  // Plan and validate both collections before mutating the shared document.
  const actions = [
    ...prepare(previousNodes, nextNodes, "nodes"),
    ...prepare(previousEdges, nextEdges, "connections"),
  ];
  doc.transact(() => {
    if (before.title !== after.title) canvas.set("title", after.title);
    actions.forEach((run) => run());
  }, origin);
}

export function nodeTextBinding(doc, nodeId) {
  const node = root(doc).get("nodes").get(nodeId);
  if (!node) throw fail("TARGET_DELETED");
  const fragment = node.get("body");
  if (!(fragment instanceof Y.XmlFragment)) throw fail("NO_TEXT_FIELD");
  return {
    fragment,
    markChanged: () => {
      if (root(doc).get("nodes").get(nodeId) !== node)
        throw fail("TARGET_DELETED");
      if (!node.get("documentPresent"))
        doc.transact(() => node.set("documentPresent", true), "local-text");
    },
  };
}

/** Explicitly chosen private recovery only. Ordinary graph rendering must use the text binding. */
export function applyRecoveredCanvasChanges(doc, before, after) {
  const probe = createCanvasDocument(after);
  probe.destroy();
  const graphOnly = copy(after),
    texts = [];
  for (const node of graphOnly.graph.nodes) {
    const old = before.graph.nodes.find((value) => value.id === node.id);
    if (!old || equal(old.textDocument, node.textDocument)) continue;
    const content = normalizeRichText(node.textDocument?.content ?? emptyText);
    const shared = root(doc).get("nodes").get(node.id);
    if (!(shared?.get("body") instanceof Y.XmlFragment))
      throw fail("NO_TEXT_FIELD");
    texts.push({ shared, content, present: !!node.textDocument });
    if (old.textDocument) node.textDocument = copy(old.textDocument);
    else delete node.textDocument;
  }
  // Plan the graph changes against a disposable clone before the live transaction.
  const validation = new Y.Doc();
  try {
    Y.applyUpdate(validation, Y.encodeStateAsUpdate(doc));
    applyCanvasChanges(validation, before, graphOnly);
  } finally {
    validation.destroy();
  }
  doc.transact(() => {
    applyCanvasChanges(doc, before, graphOnly);
    for (const { shared, content, present } of texts) {
      const fragment = shared.get("body");
      fragment.delete(0, fragment.length);
      prosemirrorJSONToYXmlFragment(richTextSchema, content, fragment);
      shared.set("documentPresent", present);
    }
  }, "local-canvas");
}

/** Graph commands have a separate history from Tiptap's ySyncPluginKey transactions. */
export function canvasUndoManager(doc) {
  return new Y.UndoManager(root(doc), {
    trackedOrigins: new Set(["local-canvas"]),
    captureTimeout: 500,
  });
}

/** Explicit entity restoration is a protocol fact inside the same frozen binary batch. */
export function applyCanvasHistory(doc, manager, direction, scope) {
  if (!["undo", "redo"].includes(direction))
    throw fail("INVALID_HISTORY_ACTION");
  const before = readCanvasDocument(doc);
  manager.stopCapturing();
  manager[direction]();
  const after = readCanvasDocument(doc),
    restored = {};
  for (const name of ["nodes", "connections"]) {
    const previous = new Set(before.graph[name].map((value) => value.id));
    restored[name] = after.graph[name]
      .filter((value) => !previous.has(value.id))
      .map((value) => value.id)
      .sort();
  }
  if (restored.nodes.length || restored.connections.length) {
    const intent = {
      id: crypto.randomUUID(),
      epoch: scope.epoch,
      sourceCommitSeq: String(scope.commitSeq),
      ...restored,
    };
    validateRestoration(intent);
    doc.transact(
      () => root(doc).set("restoration", intent),
      "local-restoration",
    );
  }
  return restored;
}

function addNode(collection, source, order) {
  const {
    position,
    parentId,
    width,
    height,
    textDocument,
    parameters,
    references,
    ...data
  } = copy(source);
  const node = new Y.Map();
  collection.set(source.id, node);
  node.set("data", map(referenceData(data, references)));
  node.set("placement", {
    position,
    ...(Object.hasOwn(source, "parentId") ? { parentId } : {}),
  });
  node.set("size", {
    ...(width !== undefined ? { width } : {}),
    ...(height !== undefined ? { height } : {}),
  });
  node.set("order", order);
  // Allocate the shared container before peers can concurrently add a first
  // reference/model setting. Concurrently replacing an absent map loses a branch.
  addParameters(node, parameters ?? {}, !!parameters);
  if (!["group", "config"].includes(source.type)) {
    if (textDocument && textDocument.schemaVersion !== 1)
      throw fail("INVALID_RICH_TEXT");
    const body = new Y.XmlFragment();
    node.set("body", body);
    node.set("documentPresent", !!textDocument);
    prosemirrorJSONToYXmlFragment(
      richTextSchema,
      normalizeRichText(textDocument?.content ?? emptyText),
      body,
    );
  } else if (textDocument) throw fail("INVALID_RICH_TEXT");
}

/** One-time creation only. A browser must hydrate committed binary state instead. */
export function createCanvasDocument({ title, graph }) {
  if (
    graph.schemaVersion !== 2 ||
    !Array.isArray(graph.nodes) ||
    !Array.isArray(graph.connections)
  )
    throw fail("INVALID_GRAPH");
  if (
    new Set(graph.nodes.map((n) => n.id)).size !== graph.nodes.length ||
    new Set(graph.connections.map((e) => e.id)).size !==
      graph.connections.length
  )
    throw fail("DUPLICATE_IDENTITY");
  const doc = new Y.Doc();
  try {
    doc.transact(() => {
      const canvas = doc.getMap("canvas");
      const { nodes, connections, ...properties } = copy(graph);
      canvas.set("modelVersion", 1);
      canvas.set("restoration", null);
      canvas.set("title", title);
      canvas.set("properties", map(properties));
      const nodeMap = new Y.Map(),
        edgeMap = new Y.Map();
      canvas.set("nodes", nodeMap);
      canvas.set("connections", edgeMap);
      nodes.forEach((node, index) => addNode(nodeMap, node, index));
      connections.forEach((edge, order) =>
        edgeMap.set(edge.id, map({ data: edge, order })),
      );
    }, "initialize");
    return doc;
  } catch (error) {
    doc.destroy();
    throw error;
  }
}

function ordered(collection) {
  return [...collection.entries()]
    .sort(
      ([aid, a], [bid, b]) =>
        a.get("order") - b.get("order") || (aid < bid ? -1 : aid > bid ? 1 : 0),
    )
    .map(([, item]) => item);
}

export function readCanvasDocument(doc) {
  const canvas = root(doc);
  return {
    title: canvas.get("title"),
    graph: {
      ...copy(canvas.get("properties").toJSON()),
      nodes: ordered(canvas.get("nodes")).map((node) => {
        const data = copy(node.get("data").toJSON());
        const references = Object.entries(data).filter(([key]) =>
          key.startsWith("reference:"),
        );
        if (data.referencesPresent) {
          data.references = references
            .map(([, value]) => value)
            .sort((a, b) => a.order - b.order || a.id.localeCompare(b.id))
            .map((value, order) => ({ ...value, order }));
        }
        delete data.referencesPresent;
        for (const [key] of references) delete data[key];
        const body = node.get("body");
        let text = {};
        let parameters = {};
        if (
          node.get("parameters")?.get("present") !== false &&
          node.get("parameters")
        ) {
          const stored = node.get("parameters");
          parameters = {
            parameters: {
              ...copy(stored.get("config")),
              ...(stored.get("valuesPresent") || stored.get("values").size > 0
                ? { values: copy(stored.get("values").toJSON()) }
                : {}),
            },
          };
        }
        if (body) {
          const content = normalizeRichText(
            yXmlFragmentToProsemirrorJSON(body),
          );
          if (node.get("documentPresent") || !equal(content, emptyText))
            text = { textDocument: { schemaVersion: 1, content } };
        }
        return {
          ...data,
          ...copy(node.get("placement")),
          ...copy(node.get("size")),
          ...parameters,
          ...text,
        };
      }),
      connections: ordered(canvas.get("connections")).map((edge) =>
        copy(edge.get("data")),
      ),
    },
  };
}
