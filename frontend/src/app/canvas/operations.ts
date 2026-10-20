import { canonical } from "@/components/generation/draft-save";
import { isDraft, isEditableNode, type Canvas, type Node } from "./document";
import { promptSequence, sequencePrompt } from "./editor-ot";
import { nodePromptDraft } from "./composer/references";
import { fieldPolicies, type FieldPolicy } from "./operation-fields";
import {
  applySequence,
  sequenceLength,
  transformSequencePair,
  type Atom,
  type SequenceEdit,
} from "./sequence";

export type Operation = {
  type: string;
  node_id?: string;
  path?: string[];
  value?: unknown;
  expected?: unknown;
  expected_exists?: boolean;
  context?: unknown;
  unset?: boolean;
  text?: SequenceEdit;
  item_id?: string;
  before_id?: string;
  node?: Node;
  edge?: API.Edge;
  [labelPrediction]?: { base: unknown; order: string[] };
};
const labelPrediction = Symbol("local reference label prediction");
type ObjectValue = Record<string, unknown>;
export function operationError(code: string): never {
  throw new Error(code);
}
export function object(value: unknown): ObjectValue {
  if (!value || typeof value !== "object" || Array.isArray(value))
    return operationError("target_not_found");
  return value as ObjectValue;
}
function identity(value: unknown): string {
  const item = object(value);
  return typeof item.id === "string"
    ? item.id
    : typeof item.bone === "string"
      ? item.bone
      : "";
}
function same(a: unknown, b: unknown) {
  return canonical(a) === canonical(b);
}
function pathEqual(a: string[] | undefined, b: string[] | undefined) {
  return same(a ?? [], b ?? []);
}
export function fieldPolicy(path: string[]): FieldPolicy {
  if (
    path.some(
      (part) =>
        !part || ["__proto__", "constructor", "prototype"].includes(part),
    )
  )
    return operationError("unsupported_field");
  let best: FieldPolicy | undefined,
    score = -1;
  for (const [pattern, rule] of Object.entries(fieldPolicies)) {
    const parts = pattern.split(".");
    if (parts.length !== path.length) continue;
    if (
      parts.every(
        (p, i) =>
          p === "*" ||
          (p === "@" && path[i].startsWith("@") && path[i].length > 1) ||
          p === path[i],
      )
    ) {
      const candidate = parts.filter((p) => p !== "*" && p !== "@").length;
      if (candidate > score) {
        best = rule;
        score = candidate;
      }
    }
  }
  return best ?? operationError("unsupported_field");
}
export function fieldContainer(
  value: unknown,
  path: string[],
): [ObjectValue, string] {
  if (
    !path.length ||
    path.length > 16 ||
    path.some((p) => ["__proto__", "constructor", "prototype"].includes(p))
  )
    return operationError("unsupported_field");
  let current = value;
  for (const key of path.slice(0, -1)) {
    if (Array.isArray(current)) {
      if (!key.startsWith("@")) return operationError("unsupported_field");
      current = current.find((item) => identity(item) === key.slice(1));
    } else current = object(current)[key];
  }
  return [object(current), path.at(-1)!];
}
function negativePromptPath(path: string[]) {
  return pathEqual(path, [
    "config",
    "draft",
    "composer",
    "params",
    "negative_prompt",
  ]);
}
export function fieldValue(
  node: unknown,
  path: string[],
  rule = fieldPolicy(path),
): { value: unknown; exists: boolean } {
  if (
    path.length === 6 &&
    path[1] === "batch_table" &&
    path[2] === "rows" &&
    path[4] === "input_node_ids"
  ) {
    const config = object(object(node).config),
      table = object(config.batch_table);
    const columns = (table.reference_columns as unknown[]) ?? [],
      rows = (table.rows as unknown[]) ?? [];
    const column = columns.findIndex((c) => identity(c) === path[5].slice(1)),
      row = rows.find((r) => identity(r) === path[3].slice(1));
    if (column < 0 || !row) return operationError("target_not_found");
    const slots = object(row).input_node_ids as unknown[];
    if (!slots || column >= slots.length)
      return operationError("target_not_found");
    return { value: slots[column], exists: true };
  }
  const [parent, key] = fieldContainer(node, path);
  if (rule.group) {
    const value: ObjectValue = {};
    for (const member of rule.group) {
      if (
        path[1] === "timeline" &&
        path[2] === "clips" &&
        ["node_id", "asset_id", "source_duration_ms"].includes(member)
      )
        value[member] = parent[member] ?? null;
      else if (
        path.at(-1) === "model" &&
        ["model_profile_id", "modelVersion"].includes(member)
      )
        value[member] = parent[member] ?? null;
      else if (
        path.at(-1) === "model" &&
        member === "mode" &&
        path[1] === "draft"
      )
        value[member] = parent[member] ?? null;
      else if (member in parent) value[member] = parent[member];
    }
    return { value, exists: !rule.optional || Object.keys(value).length > 0 };
  }
  return {
    value: negativePromptPath(path) ? (parent[key] ?? "") : parent[key],
    exists: negativePromptPath(path) || key in parent,
  };
}
export function fieldContext(
  node: unknown,
  path: string[],
  rule = fieldPolicy(path),
): unknown {
  if (!rule.context) return undefined;
  const parts = rule.context
    .split(".")
    .map((p, i) => (p === "@" ? path[i] : p));
  const [parent, key] = fieldContainer(node, parts);
  const keys =
    key === "model"
      ? parts[1] === "generation"
        ? ["capability", "model_profile_id", "mode"]
        : parts[1] === "batch_table"
          ? ["operation", "model_profile_id", "mode"]
          : ["modelVersion", "mode"]
      : parts[1] === "timeline"
        ? ["node_id", "asset_id"]
        : parts.at(-2) === "panorama"
          ? ["asset_id", "name"]
          : ["primitive", "source_node_id", "asset_id", "builtin_actor"];
  return Object.fromEntries(keys.map((k) => [k, parent[k] ?? null]));
}
function sequence(value: unknown, path: string[]): Atom[] {
  if (path.at(-1) === "document") return promptSequence(value);
  if (typeof value !== "string") return operationError("invalid_sequence");
  return value ? [{ type: "text", text: value }] : [];
}
function referenceCollection(path: string[]) {
  return pathEqual(path, ["config", "draft", "composer", "references"]);
}
function referenceText(op: Operation) {
  return (
    op.type === "text" &&
    pathEqual(op.path, ["config", "draft", "composer", "document"])
  );
}
function removal(op: Operation) {
  return op.type === "remove" && referenceCollection(op.path ?? []);
}
function cleanup(node: Node, id: string): SequenceEdit {
  return promptSequence(node.config.draft?.composer?.document).map((atom) =>
    atom.type === "reference" && atom.reference_id === id
      ? { delete: 1 }
      : { retain: sequenceLength([atom]) },
  );
}
function insertMentions(edit: SequenceEdit, id: string) {
  return edit.some((part) =>
    part.insert?.some((a) => a.type === "reference" && a.reference_id === id),
  );
}
function timelineGate(node: Node, op: Operation) {
  const path = op.path ?? [];
  if (path[0] !== "config" || path[1] !== "timeline") return;
  const timeline = node.config.timeline;
  const locked = (id: unknown) =>
    timeline?.tracks?.find((t) => t.id === id)?.locked === true;
  if (
    path[2] === "tracks" &&
    op.type === "remove" &&
    locked(op.item_id) &&
    timeline?.clips?.some((c) => c.track_id === op.item_id)
  )
    operationError("track_locked");
  if (path[2] !== "clips") return;
  const clip = timeline?.clips?.find(
    (c) => c.id === (path[3]?.slice(1) ?? op.item_id),
  );
  if (locked(clip?.track_id) || (op.value && locked(object(op.value).track_id)))
    operationError("track_locked");
}
function normalizeReferences(node: Node, op: Operation) {
  const composer = node.config.draft?.composer;
  if (!composer) return operationError("invalid_labels");
  if (
    op.type === "insert" &&
    !op[labelPrediction] &&
    composer.labelState.allocations[op.item_id!]
  )
    operationError("reference_id_reused");
  if (op[labelPrediction])
    composer.labelState = structuredClone(
      op[labelPrediction]!.base,
    ) as typeof composer.labelState;
  const order = [...(op[labelPrediction]?.order ?? [])];
  composer.references.forEach((r, i) => {
    r.order = i;
    if (!order.includes(r.id)) order.push(r.id);
  });
  for (const id of order) {
    const ref = composer.references.find((r) => r.id === id);
    if (!ref) continue;
    const kind = ref.source.kind === "subject" ? "subject" : ref.mediaKind;
    const old = composer.labelState.allocations[id];
    if (old) {
      if (old.labelKind !== kind) operationError("label_kind_changed");
      continue;
    }
    if (
      Object.keys(composer.labelState.allocations).length >= 10000 ||
      composer.labelState.highWater[kind] >= Number.MAX_SAFE_INTEGER
    )
      operationError("label_limit");
    composer.labelState.allocations[id] = {
      labelKind: kind,
      ordinal: ++composer.labelState.highWater[kind],
    };
  }
  if (op.type === "insert")
    composer.references.find((r) => r.id === op.item_id)!.bindingGeneration = 1;
  if (op.type === "remove")
    for (const p of composer.document.content) {
      p.content = p.content?.filter(
        (n) => n.type !== "reference" || n.attrs.referenceId !== op.item_id,
      );
      if (!p.content?.length) delete p.content;
    }
}
function applyOne(doc: Canvas, op: Operation) {
  if (op.type === "noop") return;
  if (op.type === "create_node" || op.type === "restore_node") {
    if (
      !op.node ||
      doc.nodes.some((n) => n.id === op.node!.id) ||
      !isEditableNode(op.node)
    )
      return operationError("id_conflict");
    doc.nodes.push(structuredClone(op.node));
    return;
  }
  if (op.type === "connect" || op.type === "restore_edge") {
    if (
      !op.edge?.id ||
      op.edge.edge_type !== "annotation" ||
      doc.edges.some((e) => e.id === op.edge!.id)
    )
      return operationError("invalid_operation");
    doc.edges.push(structuredClone(op.edge));
    return;
  }
  if (op.type === "disconnect") {
    const index = doc.edges.findIndex((e) => e.id === op.node_id);
    if (index < 0) return operationError("target_not_found");
    doc.edges.splice(index, 1);
    return;
  }
  const node = doc.nodes.find((n) => n.id === op.node_id);
  if (!node) return operationError("target_not_found");
  if (!isEditableNode(node)) return operationError("protected_target");
  if (op.type === "delete_node") {
    for (const n of doc.nodes) {
      if (n.parent_id === node.id) delete n.parent_id;
      for (const row of n.config.batch_table?.rows ?? [])
        row.input_node_ids = row.input_node_ids?.map((id) =>
          id === node.id ? null : id,
        ) as typeof row.input_node_ids;
      for (const clip of n.config.timeline?.clips ?? [])
        if (clip.node_id === node.id) {
          delete clip.node_id;
          clip.asset_id = node.ref_id;
        }
      for (const item of n.config.director?.objects ?? [])
        if (item.source_node_id === node.id) {
          delete item.source_node_id;
          item.asset_id = node.ref_id;
        }
    }
    doc.nodes = doc.nodes.filter((n) => n.id !== node.id);
    doc.edges = doc.edges.filter(
      (e) => e.source_node_id !== node.id && e.target_node_id !== node.id,
    );
    return;
  }
  if (op.type === "migrate_composer") {
    if (
      !isDraft(node) ||
      node.config.draft.composer ||
      !same(node.config.draft, op.expected)
    )
      return operationError("field_conflict");
    node.config.draft = {
      composer: nodePromptDraft(node),
      prompt: "",
      negative_prompt: "",
      aspect_ratio: "",
      candidate_count: 0,
    };
    return;
  }
  const path = op.path ?? [],
    rule = fieldPolicy(path);
  timelineGate(node, op);
  if (rule.context && !same(fieldContext(node, path, rule), op.context))
    operationError("source_changed");
  if (
    path.length === 6 &&
    path[1] === "batch_table" &&
    path[2] === "rows" &&
    path[4] === "input_node_ids"
  ) {
    if (
      op.type !== "set" ||
      op.unset ||
      (op.value !== null && typeof op.value !== "string")
    )
      return operationError("invalid_operation");
    const column =
      node.config.batch_table?.reference_columns?.findIndex(
        (c) => c.id === path[5].slice(1),
      ) ?? -1;
    const row = node.config.batch_table?.rows?.find(
      (r) => r.id === path[3].slice(1),
    );
    if (
      !row?.input_node_ids ||
      column < 0 ||
      column >= row.input_node_ids.length
    )
      return operationError("target_not_found");
    row.input_node_ids[column] = op.value as never;
    return;
  }
  const [parent, key] = fieldContainer(node, path),
    before = fieldValue(node, path, rule);
  if (["insert", "restore_item", "remove", "move"].includes(op.type)) {
    if (rule.kind !== "list" || !op.item_id)
      return operationError("unsupported_field");
    const items = (parent[key] ?? []) as unknown[];
    if (!Array.isArray(items)) return operationError("target_not_found");
    const original = [...items];
    let index = items.findIndex((item) => identity(item) === op.item_id);
    if (op.type === "insert" || op.type === "restore_item") {
      if (index >= 0) return operationError("id_conflict");
      if (identity(op.value) !== op.item_id)
        return operationError("invalid_operation");
      const value = structuredClone(op.value);
      if (op.type === "restore_item" && referenceCollection(path)) {
        const ref = object(value);
        ref.bindingGeneration = Number(ref.bindingGeneration) + 1;
      }
      items.push(value);
      index = items.length - 1;
    } else if (index < 0) return operationError("target_not_found");
    if (op.type === "remove") items.splice(index, 1);
    else {
      if (op.before_id === op.item_id) return operationError("order_conflict");
      const [item] = items.splice(index, 1),
        anchor = op.before_id
          ? items.findIndex((item) => identity(item) === op.before_id)
          : items.length;
      if (anchor < 0) return operationError("anchor_deleted");
      items.splice(anchor, 0, item);
    }
    parent[key] = items;
    if (pathEqual(path, ["config", "batch_table", "reference_columns"]))
      for (const row of node.config.batch_table?.rows ?? []) {
        const slots = new Map(
          original.map((c, i) => [
            identity(c),
            row.input_node_ids?.[i] ?? null,
          ]),
        );
        row.input_node_ids = items.map(
          (c) => slots.get(identity(c)) ?? null,
        ) as typeof row.input_node_ids;
      }
    if (referenceCollection(path)) normalizeReferences(node, op);
    return;
  }
  if (op.type !== rule.kind) return operationError("unsupported_field");
  if (
    rule.cas ||
    (rule.optional && (op.unset || !before.exists)) ||
    (!rule.group &&
      rule.context &&
      [before.value, op.value].some((v) => v !== null && typeof v === "object"))
  ) {
    if (
      op.expected_exists !== before.exists ||
      (before.exists && !same(before.value, op.expected))
    )
      operationError("field_conflict");
  }
  if (op.type === "text") {
    if (!before.exists || op.value !== undefined || op.unset)
      return operationError("invalid_operation");
    const atoms = applySequence(sequence(before.value, path), op.text ?? []);
    if (key === "document") parent[key] = sequencePrompt(atoms);
    else {
      if (atoms.some((a) => a.type !== "text"))
        return operationError("invalid_sequence");
      const value = atoms
        .map((a) => (a.type === "text" ? a.text : ""))
        .join("");
      if (negativePromptPath(path) && !value) delete parent[key];
      else parent[key] = value;
    }
  } else if (rule.group) {
    const value = op.unset && rule.optional ? {} : object(op.value);
    if (
      (op.unset && !rule.optional) ||
      Object.keys(value).some((k) => !rule.group!.includes(k))
    )
      return operationError("invalid_operation");
    for (const member of rule.group) {
      if (member in value) parent[member] = structuredClone(value[member]);
      else {
        if (key === "position" || key === "size")
          return operationError("invalid_operation");
        delete parent[member];
      }
    }
  } else if (op.unset) delete parent[key];
  else parent[key] = structuredClone(op.value);
  if (
    key === "binding" &&
    path.length === 6 &&
    referenceCollection(path.slice(0, 4))
  ) {
    const expected = object(op.expected);
    parent.bindingGeneration = Number(expected.bindingGeneration) + 1;
  }
}

/** Prediction only. The server additionally validates graph, source access and history. */
export function applyOperations(document: Canvas, ops: Operation[]): Canvas {
  if (ops.length > 100) operationError("operation_limit");
  const doc = structuredClone(document);
  const created = new Set<string>(),
    removed = new Set<string>();
  for (const op of ops) {
    const id =
      op.type === "create_node"
        ? op.node?.id
        : op.type === "connect"
          ? op.edge?.id
          : undefined;
    if (id) {
      if (created.has(id)) operationError("id_conflict");
      created.add(id);
    }
    const key = [op.node_id, ...(op.path ?? []), op.item_id].join("/");
    if (op.type === "insert" && removed.has(key)) operationError("id_conflict");
    if (op.type === "remove") removed.add(key);
    applyOne(doc, op);
  }
  for (const n of doc.nodes)
    if (isDraft(n)) {
      const old = document.nodes.find((p) => p.id === n.id);
      if (!old)
        n.input_revision = ops.some(
          (o) => o.type === "restore_node" && o.node?.id === n.id,
        )
          ? (n.input_revision ?? 0) + 1
          : 1;
      else if (!same(old.config.draft, n.config.draft))
        n.input_revision = (old.input_revision ?? 0) + 1;
    }
  return doc;
}
function targetsItem(op: Operation, removed: Operation) {
  const p = removed.path ?? [],
    q = op.path ?? [];
  return (
    q.length > p.length &&
    pathEqual(q.slice(0, p.length), p) &&
    q[p.length] === "@" + removed.item_id
  );
}
export function usesNode(op: Operation, id: string | undefined) {
  if (!id) return false;
  if (
    op.node_id === id ||
    op.node?.id === id ||
    op.node?.parent_id === id ||
    op.edge?.source_node_id === id ||
    op.edge?.target_node_id === id
  )
    return true;
  const n = op.node;
  if (
    n &&
    (n.config.batch_table?.rows?.some((r) => r.input_node_ids?.includes(id)) ||
      n.config.timeline?.clips?.some((c) => c.node_id === id) ||
      n.config.director?.objects?.some((o) => o.source_node_id === id))
  )
    return true;
  const path = op.path ?? [];
  if (
    op.type === "set" &&
    (path.join(".") === "parent_id" ||
      (path[1] === "batch_table" && path[4] === "input_node_ids"))
  )
    return op.value === id;
  if (
    ["insert", "restore_item"].includes(op.type) &&
    path.join(".") === "config.batch_table.rows" &&
    op.value &&
    typeof op.value === "object"
  ) {
    const slots = (op.value as Record<string, unknown>).input_node_ids;
    return Array.isArray(slots) && slots.includes(id);
  }
  if (
    path[0] === "config" &&
    ((path[1] === "timeline" && path[2] === "clips") ||
      (path[1] === "director" && path[2] === "objects")) &&
    op.value &&
    typeof op.value === "object"
  ) {
    const v = op.value as Record<string, unknown>;
    return v.node_id === id || v.source_node_id === id;
  }
  return false;
}
function structural(op: Operation) {
  return ["insert", "restore_item", "remove", "move"].includes(op.type);
}
function overlaps(a: Operation, b: Operation) {
  const paths = (op: Operation) => {
    const p = op.path ?? [],
      r = fieldPolicy(p);
    return r.group ? r.group.map((k) => [...p.slice(0, -1), k]) : [p];
  };
  return paths(a).some((x) =>
    paths(b).some((y) =>
      pathEqual(
        x.slice(0, Math.min(x.length, y.length)),
        y.slice(0, Math.min(x.length, y.length)),
      ),
    ),
  );
}
function pair(
  base: Canvas,
  earlier: Operation,
  later: Operation,
): [Operation, Operation] {
  let a = { ...earlier },
    b = { ...later };
  if (a.type === "delete_node" && usesNode(b, a.node_id)) {
    if (b.type === "delete_node" && a.node_id === b.node_id)
      return [a, { type: "noop" }];
    return operationError("target_deleted");
  }
  if (b.type === "delete_node" && usesNode(a, b.node_id))
    return [{ type: "noop" }, b];
  const draftOperation = (o: Operation) =>
    o.type === "migrate_composer" ||
    (o.path?.[0] === "config" && o.path?.[1] === "draft");
  if (
    a.node_id === b.node_id &&
    ((a.type === "migrate_composer" && draftOperation(b)) ||
      (b.type === "migrate_composer" && draftOperation(a)))
  )
    return operationError("field_conflict");
  if (a.type === "migrate_composer" || b.type === "migrate_composer")
    return [a, b];
  if (
    ["create_node", "restore_node"].includes(a.type) &&
    ["create_node", "restore_node"].includes(b.type) &&
    a.node?.id === b.node?.id
  )
    return operationError("id_conflict");
  if (a.type === "connect" && b.type === "connect" && a.edge?.id === b.edge?.id)
    return operationError("id_conflict");
  if (
    a.type === "disconnect" &&
    b.type === "disconnect" &&
    a.node_id === b.node_id
  )
    return [a, { type: "noop" }];
  if (
    [a.type, b.type].some((t) =>
      [
        "create_node",
        "restore_node",
        "connect",
        "restore_edge",
        "disconnect",
      ].includes(t),
    )
  )
    return [a, b];
  if (a.type === "noop" || b.type === "noop" || a.node_id !== b.node_id)
    return [a, b];
  if ((removal(a) && referenceText(b)) || (referenceText(a) && removal(b))) {
    const node = base.nodes.find((n) => n.id === a.node_id)!;
    if (removal(a)) {
      if (insertMentions(b.text ?? [], a.item_id!))
        return operationError("target_deleted");
      [, b.text] = transformSequencePair(
        cleanup(node, a.item_id!),
        b.text ?? [],
        sequenceLength(promptSequence(node.config.draft?.composer?.document)),
      );
    } else {
      [a.text] = transformSequencePair(
        a.text ?? [],
        cleanup(node, b.item_id!),
        sequenceLength(promptSequence(node.config.draft?.composer?.document)),
      );
      a.text = a.text.flatMap((p) =>
        p.insert
          ? (() => {
              const insert = p.insert!.filter(
                (x) => x.type !== "reference" || x.reference_id !== b.item_id,
              );
              return insert.length ? [{ insert }] : [];
            })()
          : [p],
      );
    }
    return [a, b];
  }
  const restoredRow = (columns: Operation, row: Operation) => {
    if (
      row.type !== "restore_item" ||
      row.node_id !== columns.node_id ||
      row.path?.join(".") !== "config.batch_table.rows" ||
      columns.path?.join(".") !== "config.batch_table.reference_columns"
    )
      return row;
    const old =
      base.nodes.find((n) => n.id === row.node_id)?.config.batch_table
        ?.reference_columns ?? [];
    const after =
      applyOperations(base, [columns]).nodes.find((n) => n.id === row.node_id)
        ?.config.batch_table?.reference_columns ?? [];
    const value = structuredClone(row.value) as API.BatchTableRow;
    const slots = new Map(
      old.map((c, i) => [c.id, value.input_node_ids?.[i] ?? null]),
    );
    value.input_node_ids = after.map(
      (c) => slots.get(c.id) ?? null,
    ) as API.BatchTableRow["input_node_ids"];
    return { ...row, value };
  };
  if (structural(a)) b = restoredRow(a, b);
  if (structural(b)) a = restoredRow(b, a);
  if (structural(a) && structural(b) && pathEqual(a.path, b.path)) {
    if (a.type === "move" || b.type === "move")
      return operationError("order_conflict");
    if (
      ["insert", "restore_item"].includes(a.type) &&
      ["insert", "restore_item"].includes(b.type)
    ) {
      if (a.item_id === b.item_id) return operationError("id_conflict");
      if (referenceCollection(a.path ?? [])) {
        const old = a[labelPrediction];
        a[labelPrediction] = {
          base:
            old?.base ??
            structuredClone(
              base.nodes.find((n) => n.id === a.node_id)!.config.draft!
                .composer!.labelState,
            ),
          order: [...new Set([...(old?.order ?? [a.item_id!]), b.item_id!])],
        };
      }
      if (a.before_id === b.before_id) a.before_id = b.item_id;
      return [a, b];
    }
    if (a.type === "remove" && b.type === "remove" && a.item_id === b.item_id)
      return [a, { type: "noop" }];
    if (a.type === "remove" && (b.before_id === a.item_id || targetsItem(b, a)))
      return operationError("target_deleted");
    return [a, b];
  }
  if (a.type === "remove" && targetsItem(b, a))
    return operationError("target_deleted");
  if (b.type === "remove" && targetsItem(a, b)) return [{ type: "noop" }, b];
  if (!pathEqual(a.path, b.path)) {
    const ar = fieldPolicy(a.path ?? []),
      br = fieldPolicy(b.path ?? []);
    if ((ar.cas || br.cas) && overlaps(a, b))
      return operationError("field_conflict");
    return [a, b];
  }
  if (a.type === "text" && b.type === "text") {
    const n = base.nodes.find((n) => n.id === a.node_id)!;
    const value = fieldValue(n, a.path!).value;
    [a.text, b.text] = transformSequencePair(
      a.text ?? [],
      b.text ?? [],
      sequenceLength(sequence(value, a.path!)),
    );
    return [a, b];
  }
  if (a.type === "set" && b.type === "set") {
    const r = fieldPolicy(a.path ?? []);
    if (
      r.cas ||
      (r.optional &&
        (a.unset ||
          b.unset ||
          a.expected_exists === false ||
          b.expected_exists === false)) ||
      (!r.group &&
        [a.value, b.value].some((v) => v !== null && typeof v === "object"))
    )
      return operationError("field_conflict");
    return [{ type: "noop" }, b];
  }
  return operationError("field_conflict");
}
export function transformOperationPair(
  base: Canvas,
  a: Operation[],
  b: Operation[],
): [Operation[], Operation[]] {
  applyOperations(base, a);
  applyOperations(base, b);
  let ap = [...a],
    prefix = base;
  const bp: Operation[] = [];
  for (const late of b) {
    let state = prefix,
      transformed = late;
    const next: Operation[] = [];
    for (const early of ap) {
      const [x, y] = pair(state, early, transformed);
      next.push(x);
      transformed = y;
      const step = structuredClone(state);
      applyOne(step, early);
      state = step;
    }
    ap = next;
    bp.push(transformed);
    const step = structuredClone(prefix);
    applyOne(step, late);
    prefix = step;
  }
  return [ap, bp];
}

export type Effect = {
  entity: string;
  id: string;
  path: string[];
  index: number;
  after?: unknown;
  after_exists: boolean;
};
export function applyEffects(document: Canvas, effects: Effect[]): Canvas {
  const doc = structuredClone(document);
  for (const e of effects) {
    if (
      !["node", "edge"].includes(e.entity) ||
      !e.id ||
      !Array.isArray(e.path) ||
      !Number.isSafeInteger(e.index) ||
      e.index < 0 ||
      e.path.some(
        (p) =>
          typeof p !== "string" ||
          ["__proto__", "constructor", "prototype"].includes(p),
      )
    )
      return operationError("history_unavailable");
    if (!e.path.length && e.after_exists && object(e.after).id !== e.id)
      return operationError("history_unavailable");
    const items = e.entity === "node" ? doc.nodes : doc.edges;
    const index = items.findIndex((n) => n.id === e.id);
    if (!e.path.length) {
      if (e.after_exists) {
        if (index < 0)
          items.splice(
            Math.min(e.index, items.length),
            0,
            structuredClone(e.after) as never,
          );
        else items[index] = structuredClone(e.after) as never;
      } else if (index >= 0) items.splice(index, 1);
      continue;
    }
    if (index < 0) return operationError("history_unavailable");
    const [parent, key] = fieldContainer(items[index], e.path);
    if (e.after_exists) parent[key] = structuredClone(e.after);
    else delete parent[key];
  }
  return doc;
}
