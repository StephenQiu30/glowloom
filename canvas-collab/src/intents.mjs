import { canonicalJson } from "./canonical.mjs";

const copy = (value) => structuredClone(value);
const cell = (value, exists = value !== undefined) =>
  exists ? { exists: true, value: copy(value) } : { exists: false };
const equal = (left, right) => canonicalJson(left) === canonicalJson(right);
const key = (value) =>
  JSON.stringify([value.collection, value.id, value.field]);
const pairs = {
  placement: ["position", "parentId"],
  size: ["width", "height"],
};
const fields = [
  "title",
  "placement",
  "size",
  "zIndex",
  "collapsed",
  "parameters",
  "textDocument",
  "fileReferences",
  // The formal Go DTO stores fixed media/configuration in one shared native
  // value. Keep its semantic intent at the same atomic boundary for recovery.
  "native",
];
const nodeMap = (snapshot) =>
  new Map(snapshot.graph.nodes.map((node) => [node.id, node]));
const edgeMap = (snapshot) =>
  new Map(snapshot.graph.connections.map((edge) => [edge.id, edge]));

function nodeCell(node, field) {
  if (!node) return cell(undefined);
  if (field === "entity") return cell(node);
  if (field.startsWith("value:")) {
    const name = field.slice(6),
      values = node.parameters?.values ?? {};
    return cell(values[name], Object.hasOwn(values, name));
  }
  if (pairs[field])
    return cell(
      Object.fromEntries(
        pairs[field]
          .filter((name) => Object.hasOwn(node, name))
          .map((name) => [name, node[name]]),
      ),
    );
  if (!fields.includes(field)) throw new Error("INVALID_SEMANTIC_FIELD");
  return cell(node[field], Object.hasOwn(node, field));
}

export function readIntent(snapshot, intent) {
  if (intent.collection === "canvas" && intent.field === "title")
    return cell(snapshot.title);
  if (intent.collection === "nodes")
    return nodeCell(nodeMap(snapshot).get(intent.id), intent.field);
  if (intent.collection === "connections" && intent.field === "entity")
    return cell(edgeMap(snapshot).get(intent.id));
  throw new Error("INVALID_SEMANTIC_FIELD");
}

function setOwn(target, field, value) {
  if (value.exists)
    Object.defineProperty(target, field, {
      value: copy(value.value),
      configurable: true,
      enumerable: true,
      writable: true,
    });
  else delete target[field];
}
function writeNode(node, field, value) {
  if (pairs[field]) {
    for (const name of pairs[field])
      setOwn(
        node,
        name,
        value.exists
          ? cell(value.value[name], Object.hasOwn(value.value, name))
          : cell(undefined),
      );
  } else if (field.startsWith("value:")) {
    node.parameters ??= { values: {} };
    node.parameters.values ??= {};
    setOwn(node.parameters.values, field.slice(6), value);
  } else {
    if (!fields.includes(field)) throw new Error("INVALID_SEMANTIC_FIELD");
    setOwn(node, field, value);
  }
}

/** Readable private intent only; the live editor must separately validate and explicitly choose what to submit. */
export function applyReadableIntents(snapshot, intents) {
  const result = copy(snapshot);
  for (const intent of intents) {
    if (intent.collection === "canvas" && intent.field === "title") {
      if (intent.after.exists) result.title = intent.after.value;
      continue;
    }
    if (!["nodes", "connections"].includes(intent.collection))
      throw new Error("INVALID_SEMANTIC_FIELD");
    const values = result.graph[intent.collection],
      index = values.findIndex((value) => value.id === intent.id);
    if (intent.field === "entity") {
      if (!intent.after.exists) {
        if (index >= 0) values.splice(index, 1);
      } else if (index >= 0) values[index] = copy(intent.after.value);
      else values.push(copy(intent.after.value));
    } else if (intent.collection === "nodes" && index >= 0)
      writeNode(values[index], intent.field, intent.after);
    else if (intent.collection !== "nodes")
      throw new Error("INVALID_SEMANTIC_FIELD");
  }
  return result;
}

/** Coalesces semantic local changes independently of the immutable binary queue and remote rendering. */
export function trackIntents(previous, before, after, generation) {
  const pending = new Map(
    previous.map((intent) => [key(intent), copy(intent)]),
  );
  const track = (collection, id, field, prior, next) => {
    if (equal(prior, next)) return;
    const identity = { collection, id, field },
      old = pending.get(key(identity));
    const intent = {
      ...identity,
      before: old?.before ?? prior,
      after: next,
      generation,
    };
    if (equal(intent.before, intent.after)) pending.delete(key(identity));
    else pending.set(key(identity), intent);
  };
  track("canvas", "", "title", cell(before.title), cell(after.title));
  const oldNodes = nodeMap(before),
    newNodes = nodeMap(after);
  for (const id of new Set([...oldNodes.keys(), ...newNodes.keys()])) {
    const old = oldNodes.get(id),
      next = newNodes.get(id);
    const identity = { collection: "nodes", id, field: "entity" },
      entity = pending.get(key(identity));
    if (!old || !next || entity) {
      if (equal(cell(old), cell(next))) continue;
      let baseline = entity?.before ?? cell(old);
      if (!entity && old) {
        const image = copy(old);
        for (const intent of pending.values())
          if (intent.collection === "nodes" && intent.id === id)
            writeNode(image, intent.field, intent.before);
        baseline = cell(image);
      }
      for (const [entryKey, intent] of pending)
        if (intent.collection === "nodes" && intent.id === id)
          pending.delete(entryKey);
      if (!equal(baseline, cell(next)))
        pending.set(key(identity), {
          ...identity,
          before: baseline,
          after: cell(next),
          generation,
        });
      continue;
    }
    for (const field of fields) {
      if (
        field === "parameters" &&
        old.type === "config" &&
        next.type === "config"
      ) {
        for (const name of new Set([
          ...Object.keys(old.parameters?.values ?? {}),
          ...Object.keys(next.parameters?.values ?? {}),
        ]))
          track(
            "nodes",
            id,
            "value:" + name,
            nodeCell(old, "value:" + name),
            nodeCell(next, "value:" + name),
          );
      } else
        track("nodes", id, field, nodeCell(old, field), nodeCell(next, field));
    }
  }
  const oldEdges = edgeMap(before),
    newEdges = edgeMap(after);
  for (const id of new Set([...oldEdges.keys(), ...newEdges.keys()]))
    track(
      "connections",
      id,
      "entity",
      cell(oldEdges.get(id)),
      cell(newEdges.get(id)),
    );
  return [...pending.values()];
}

/** Exact ACK prefix bookkeeping. Acceptance of bytes is not proof a competing single-value intent prevailed. */
export function acknowledgeIntents(pending, frozen, confirmed) {
  const accepted = new Map(frozen.map((intent) => [key(intent), intent]));
  const latest = new Map(pending.map((intent) => [key(intent), intent]));
  const unapplied = frozen
    .filter((intent) => {
      const newer = latest.get(key(intent));
      const actual = readIntent(confirmed, intent);
      if (
        intent.collection === "nodes" &&
        intent.field === "textDocument" &&
        actual.exists
      )
        return false;
      if (
        intent.field === "entity" &&
        !intent.before.exists &&
        intent.after.exists &&
        actual.exists
      )
        return false;
      return (
        !(newer && newer.generation > intent.generation) &&
        !equal(actual, intent.after)
      );
    })
    .map((intent) => ({
      ...copy(intent),
      actual: readIntent(confirmed, intent),
    }));
  const remaining = pending
    .filter((intent) => {
      const prefix = accepted.get(key(intent));
      return !prefix || intent.generation > prefix.generation;
    })
    .map((intent) =>
      accepted.has(key(intent))
        ? { ...copy(intent), before: readIntent(confirmed, intent) }
        : copy(intent),
    );
  return { pending: remaining, unapplied };
}
