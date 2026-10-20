import { canonical } from "@/components/generation/draft-save";
import { fieldPolicies, type FieldPolicy } from "./operation-fields";
import {
  applyOperations,
  fieldContext,
  fieldPolicy,
  fieldValue,
  object,
  operationError,
  type Operation,
} from "./operations";
import { isDraft, type Canvas, type Node } from "./document";
import type { SequenceEdit } from "./sequence";

export type TextTrace = {
  node_id: string;
  path: string[];
  edits: SequenceEdit[];
};
export type EditTrace = { text?: TextTrace[]; markdownSource?: Set<string> };
const same = (a: unknown, b: unknown) => canonical(a) === canonical(b);
const identity = (value: unknown) => {
  const item = object(value);
  return String(item.id ?? item.bone ?? "");
};
function pathsFor(
  before: unknown,
  after: unknown,
  pattern: string[],
): string[][] {
  const result: string[][] = [];
  if (pattern.join(".") === "config.batch_table.rows.@.input_node_ids.@") {
    const table = (node: unknown) => {
      try {
        return object(object(object(node).config).batch_table);
      } catch {
        return {};
      }
    };
    const a = table(before),
      b = table(after);
    const rows = [
      ...new Set(
        [
          ...((a.rows as unknown[]) ?? []),
          ...((b.rows as unknown[]) ?? []),
        ].map(identity),
      ),
    ];
    const columns = [
      ...new Set(
        [
          ...((a.reference_columns as unknown[]) ?? []),
          ...((b.reference_columns as unknown[]) ?? []),
        ].map(identity),
      ),
    ];
    return rows.flatMap((row) =>
      columns.map((column) => [
        "config",
        "batch_table",
        "rows",
        "@" + row,
        "input_node_ids",
        "@" + column,
      ]),
    );
  }
  const visit = (
    left: unknown,
    right: unknown,
    index: number,
    path: string[],
  ) => {
    if (index === pattern.length) {
      result.push(path);
      return;
    }
    const part = pattern[index];
    if (part === "@") {
      const a = Array.isArray(left) ? left : [],
        b = Array.isArray(right) ? right : [];
      for (const id of new Set([...a, ...b].map(identity)))
        if (id)
          visit(
            a.find((v) => identity(v) === id),
            b.find((v) => identity(v) === id),
            index + 1,
            [...path, "@" + id],
          );
      return;
    }
    if (part === "*") {
      const a =
          left && typeof left === "object" && !Array.isArray(left)
            ? object(left)
            : {},
        b =
          right && typeof right === "object" && !Array.isArray(right)
            ? object(right)
            : {};
      for (const key of new Set([...Object.keys(a), ...Object.keys(b)]))
        visit(a[key], b[key], index + 1, [...path, key]);
      return;
    }
    if (index === pattern.length - 1) {
      if (left !== undefined || right !== undefined)
        result.push([...path, part]);
      return;
    }
    const read = (value: unknown) =>
      value && typeof value === "object" && !Array.isArray(value)
        ? object(value)[part]
        : undefined;
    visit(read(left), read(right), index + 1, [...path, part]);
  };
  visit(before, after, 0, []);
  return result;
}
function maybeValue(node: Node, path: string[], rule: FieldPolicy) {
  try {
    return fieldValue(node, path, rule);
  } catch {
    return undefined;
  }
}
function groupMembers(path: string[], rule: FieldPolicy) {
  return (rule.group ?? []).map((key) => [...path.slice(0, -1), key]);
}
function under(path: string[], prefix: string[]) {
  return path.length >= prefix.length && prefix.every((p, i) => p === path[i]);
}
function sourceIdentity(node: Node, path: string[]) {
  return fieldContext(node, path, { kind: "set", context: path.join(".") });
}
function projection(node: Node) {
  const value = structuredClone(node);
  delete value.input_revision;
  const composer = value.config.draft?.composer;
  if (composer) {
    delete (composer as Partial<typeof composer>).labelState;
    for (const ref of composer.references) {
      delete (ref as Partial<typeof ref>).bindingGeneration;
      delete (ref as Partial<typeof ref>).order;
    }
    if (composer.params.negative_prompt === "")
      delete composer.params.negative_prompt;
  }
  return value;
}

/** Snapshots identify changed scalar/group/list controls. T fields must supply
 * real editor/native-input ranges; final strings never determine character edits. */
export function canvasOperations(
  before: Canvas,
  desired: Canvas,
  trace: EditTrace = {},
): Operation[] {
  const operations: Operation[] = [];
  let candidate = before;
  const append = (op: Operation) => {
    if (operations.length >= 100) operationError("operation_limit");
    candidate = applyOperations(candidate, [op]);
    operations.push(op);
  };
  const find = (id: string) =>
    candidate.nodes.find((n) => n.id === id) ??
    operationError("target_not_found");
  const set = (id: string, path: string[], next: Node) => {
    const node = find(id),
      rule = fieldPolicy(path),
      old = fieldValue(node, path, rule),
      value = fieldValue(next, path, rule);
    const op: Operation = {
      type: "set",
      node_id: id,
      path,
      value: value.value,
    };
    if (!value.exists) {
      delete op.value;
      op.unset = true;
    }
    if (
      rule.cas ||
      (rule.optional && (value.exists !== old.exists || !value.exists)) ||
      (!rule.group &&
        rule.context &&
        [old.value, value.value].some(
          (v) => v !== null && typeof v === "object",
        ))
    ) {
      op.expected_exists = old.exists;
      if (old.exists) op.expected = old.value;
    }
    if (rule.context) op.context = fieldContext(node, path, rule);
    append(op);
  };
  for (const edge of before.edges)
    if (!desired.edges.some((e) => e.id === edge.id))
      append({ type: "disconnect", node_id: edge.id });
  const added = new Set<string>();
  for (const node of desired.nodes)
    if (!before.nodes.some((n) => n.id === node.id)) {
      const clean = structuredClone(node);
      delete clean.input_revision;
      append({ type: "create_node", node: clean });
      added.add(node.id);
    }
  for (const node of before.nodes)
    if (!desired.nodes.some((n) => n.id === node.id))
      append({ type: "delete_node", node_id: node.id });
  const covered = new Map<string, string[][]>();
  for (const next of desired.nodes) {
    if (added.has(next.id)) continue;
    let node = find(next.id);
    if (
      isDraft(node) &&
      isDraft(next) &&
      !node.config.draft.composer &&
      next.config.draft.composer
    )
      append({
        type: "migrate_composer",
        node_id: node.id,
        expected: structuredClone(node.config.draft),
      });
    node = find(next.id);
    const paths = new Map<string, string[]>();
    for (const pattern of Object.keys(fieldPolicies))
      for (const path of pathsFor(node, next, pattern.split(".")))
        paths.set(path.join("\0"), path);
    const sorted = [...paths.values()].sort((a, b) => a.length - b.length);
    const skip: string[][] = [];
    covered.set(next.id, skip);
    // Switches and optional presence changes run before their nested controls.
    for (const path of sorted) {
      if (skip.some((prefix) => under(path, prefix))) continue;
      const rule = fieldPolicy(path);
      if (!rule.group || path.at(-1) === "text_source") continue;
      const old = maybeValue(find(next.id), path, rule),
        value = maybeValue(next, path, rule);
      if (!old || !value || same(old, value)) continue;
      const model = path.at(-1) === "model",
        source = path.at(-1) === "source";
      const hasChildren = sorted.some(
        (other) => other.length > path.length && under(other, path),
      );
      const presence = rule.optional && old.exists !== value.exists;
      const switchGroup =
        model || source
          ? same(
              sourceIdentity(find(next.id), path),
              sourceIdentity(next, path),
            ) === false
          : path.at(-1) === "binding";
      if (
        presence ||
        switchGroup ||
        (rule.optional && hasChildren === false) ||
        (rule.cas && !model && !source && !hasChildren)
      ) {
        set(next.id, path, next);
        skip.push(...groupMembers(path, rule));
      }
    }
  }
  // Exact causal editor steps run before reference removals, whose implicit
  // cleanup would otherwise consume these same mention positions twice.
  for (const entry of trace.text ?? []) {
    if (
      added.has(entry.node_id) ||
      !desired.nodes.some((n) => n.id === entry.node_id)
    )
      continue;
    if (covered.get(entry.node_id)?.some((prefix) => under(entry.path, prefix)))
      continue;
    const rule = fieldPolicy(entry.path);
    if (rule.kind !== "text") operationError("unsupported_field");
    for (const edit of entry.edits) {
      if (!edit.length) continue;
      const node = find(entry.node_id);
      append({
        type: "text",
        node_id: entry.node_id,
        path: entry.path,
        text: edit,
        ...(rule.context
          ? { context: fieldContext(node, entry.path, rule) }
          : {}),
      });
    }
  }
  for (const next of desired.nodes) {
    if (added.has(next.id)) continue;
    const paths = new Map<string, string[]>();
    for (const pattern of Object.keys(fieldPolicies))
      for (const path of pathsFor(find(next.id), next, pattern.split(".")))
        paths.set(path.join("\0"), path);
    const sorted = [...paths.values()].sort((a, b) => a.length - b.length);
    const skip = covered.get(next.id) ?? [];
    for (const path of sorted) {
      if (skip.some((prefix) => under(path, prefix))) continue;
      const rule = fieldPolicy(path);
      if (rule.kind !== "list") continue;
      const old = maybeValue(find(next.id), path, rule),
        value = maybeValue(next, path, rule);
      if (!old || !value) continue;
      const actual = Array.isArray(old.value) ? old.value : [],
        wanted = Array.isArray(value.value) ? value.value : [];
      const context = () =>
        rule.context
          ? { context: fieldContext(find(next.id), path, rule) }
          : {};
      for (const item of actual)
        if (!wanted.some((v) => identity(v) === identity(item)))
          append({
            type: "remove",
            node_id: next.id,
            path,
            item_id: identity(item),
            ...context(),
          });
      for (let i = wanted.length - 1; i >= 0; i--) {
        const items =
          (fieldValue(find(next.id), path, rule).value as unknown[]) ?? [];
        if (!items.some((v) => identity(v) === identity(wanted[i])))
          append({
            type: "insert",
            node_id: next.id,
            path,
            item_id: identity(wanted[i]),
            before_id: i + 1 < wanted.length ? identity(wanted[i + 1]) : "",
            value: wanted[i],
            ...context(),
          });
      }
      for (let i = 0; i < wanted.length; i++) {
        const items =
          (fieldValue(find(next.id), path, rule).value as unknown[]) ?? [];
        if (identity(items[i]) !== identity(wanted[i]))
          append({
            type: "move",
            node_id: next.id,
            path,
            item_id: identity(wanted[i]),
            before_id: identity(items[i]),
            ...context(),
          });
      }
    }
    for (const path of sorted) {
      if (skip.some((prefix) => under(path, prefix))) continue;
      const rule = fieldPolicy(path);
      if (rule.kind === "list" || path.at(-1) === "text_source") continue;
      const old = maybeValue(find(next.id), path, rule),
        value = maybeValue(next, path, rule);
      if (!old || !value || same(old, value)) continue;
      if (
        rule.group &&
        (path.at(-1) === "model" ||
          path.at(-1) === "source" ||
          (rule.optional &&
            sorted.some(
              (other) => other.length > path.length && under(other, path),
            )))
      )
        continue;
      if (rule.kind === "text") {
        if (
          path.join(".") === "config.text" &&
          trace.markdownSource?.has(next.id)
        ) {
          set(next.id, ["config", "text_source"], next);
          continue;
        }
        operationError("unmapped_text_edit");
      }
      set(next.id, path, next);
    }
  }
  for (const edge of desired.edges)
    if (!candidate.edges.some((e) => e.id === edge.id))
      append({ type: "connect", edge });
  if (
    candidate.nodes.length !== desired.nodes.length ||
    candidate.nodes.some((node) => {
      const target = desired.nodes.find((n) => n.id === node.id);
      return !target || !same(projection(node), projection(target));
    }) ||
    !same(
      [...candidate.edges].sort((a, b) =>
        String(a.id).localeCompare(String(b.id)),
      ),
      [...desired.edges].sort((a, b) =>
        String(a.id).localeCompare(String(b.id)),
      ),
    )
  )
    operationError("unmapped_canvas_field");
  return operations;
}
