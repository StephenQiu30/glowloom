const compareCodePoints = (a, b) => {
  const left = Array.from(a, (c) => c.codePointAt(0)),
    right = Array.from(b, (c) => c.codePointAt(0));
  for (let i = 0; i < Math.min(left.length, right.length); i++)
    if (left[i] !== right[i]) return left[i] - right[i];
  return left.length - right.length;
};

/** Same canonical JSON contract as Java CanvasGraphCanonicalizer. */
export function canonicalJson(value) {
  if (JSON.isRawJSON(value)) return value.rawJSON;
  if (typeof value === "number") {
    if (!Number.isFinite(value)) throw new Error("INVALID_JSON_NUMBER");
    if (value === 0) return "0";
    const text = String(value);
    if (!/[eE]/.test(text)) return text;
    const negative = value < 0;
    const [mantissa, exponent] = text.replace(/^-/, "").split(/[eE]/);
    const [integer, fraction = ""] = mantissa.split(".");
    const digits = integer + fraction,
      point = integer.length + Number(exponent);
    const expanded =
      point <= 0
        ? "0." + "0".repeat(-point) + digits
        : point >= digits.length
          ? digits + "0".repeat(point - digits.length)
          : digits.slice(0, point) + "." + digits.slice(point);
    return (negative ? "-" : "") + expanded;
  }
  if (Array.isArray(value))
    return "[" + value.map(canonicalJson).join(",") + "]";
  if (value && typeof value === "object")
    return (
      "{" +
      Object.keys(value)
        .sort(compareCodePoints)
        .map((key) => JSON.stringify(key) + ":" + canonicalJson(value[key]))
        .join(",") +
      "}"
    );
  if (value === null || typeof value === "string" || typeof value === "boolean")
    return JSON.stringify(value);
  throw new Error("INVALID_JSON_VALUE");
}

export function modelIdentity(value) {
  if (typeof value === "string" && /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/.test(value) && value !== "00000000-0000-0000-0000-000000000000") return value;
  if (
    typeof value !== "string" ||
    !/^[1-9][0-9]{0,18}$/.test(value) ||
    BigInt(value) > 9223372036854775807n
  )
    throw new Error("INVALID_MODEL_IDENTITY");
  return value;
}

/** Typed Java Long identity is a string in the shared model, a numeric token in persisted v2 JSON. */
export function canonicalGraphJson(graph) {
  const copy = structuredClone(graph);
  for (const node of copy.nodes) {
    const selection = node.parameters?.modelSelection;
    if (selection) {
      const id = modelIdentity(selection.connectionId);
      selection.connectionId = /^\d+$/.test(id) ? JSON.rawJSON(id) : id;
    }
  }
  return canonicalJson(copy);
}

function decimalLiteral(text) {
  const [, sign, integer, fraction = "", exponent = "0"] =
    /^(-?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(text);
  const digits = integer + fraction,
    point = integer.length + Number(exponent);
  if (Math.abs(point) > 1024 * 1024) throw new Error("UNREPRESENTABLE_NUMBER");
  let expanded =
    point <= 0
      ? "0." + "0".repeat(-point) + digits
      : point >= digits.length
        ? digits + "0".repeat(point - digits.length)
        : digits.slice(0, point) + "." + digits.slice(point);
  expanded = expanded.replace(/^0+(?=\d)/, "");
  if (expanded.includes("."))
    expanded = expanded.replace(/0+$/, "").replace(/\.$/, "");
  return (sign && expanded !== "0" ? "-" : "") + expanded;
}

/** Parse authoritative stored JSON before IEEE-754 conversion; unknown precision loss blocks conversion. */
export function parseStoredGraph(input) {
  const token = Symbol("number");
  const parsed = JSON.parse(input, (_key, value, context) =>
    typeof value === "number" ? { [token]: context.source } : value,
  );
  for (const node of parsed.nodes ?? []) {
    const selection = node.parameters?.modelSelection;
    if (selection)
      selection.connectionId = modelIdentity(
        selection.connectionId?.[token] ?? selection.connectionId,
      );
  }
  const convert = (value) => {
    if (value && typeof value === "object" && value[token] !== undefined) {
      const number = Number(value[token]);
      if (
        !Number.isFinite(number) ||
        decimalLiteral(value[token]) !== canonicalJson(number)
      )
        throw new Error("UNREPRESENTABLE_NUMBER");
      return number;
    }
    if (Array.isArray(value)) return value.map(convert);
    if (value && typeof value === "object")
      return Object.fromEntries(
        Object.entries(value).map(([key, item]) => [key, convert(item)]),
      );
    return value;
  };
  return convert(parsed);
}
