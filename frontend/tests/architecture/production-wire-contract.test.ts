import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const frontendRoot = resolve(import.meta.dirname, "../..");
const repositoryRoot = resolve(frontendRoot, "..");
const fixture = JSON.parse(
  readFileSync(
    resolve(
      repositoryRoot,
      "backend/tests/fixtures/agent/storygraph-scene-analysis-wire.json",
    ),
    "utf8",
  ),
) as Record<string, unknown>;

function compareUtf8(left: string, right: string): number {
  return Buffer.from(left, "utf8").compare(Buffer.from(right, "utf8"));
}

function productionCanonicalJson(value: unknown): string {
  if (value === null || typeof value === "boolean") {
    return JSON.stringify(value);
  }
  if (typeof value === "number") {
    if (!Number.isSafeInteger(value)) {
      throw new Error("production canonical JSON only permits safe integers");
    }
    return String(value);
  }
  if (typeof value === "string") {
    return JSON.stringify(value.normalize("NFC"));
  }
  if (Array.isArray(value)) {
    return `[${value.map(productionCanonicalJson).join(",")}]`;
  }
  if (typeof value === "object") {
    const normalized = new Map<string, unknown>();
    for (const [key, item] of Object.entries(value)) {
      const normalizedKey = key.normalize("NFC");
      if (normalized.has(normalizedKey)) {
        throw new Error("production canonical JSON contains duplicate normalized keys");
      }
      normalized.set(normalizedKey, item);
    }
    return `{${[...normalized.entries()]
      .sort(([left], [right]) => compareUtf8(left, right))
      .map(
        ([key, item]) =>
          `${JSON.stringify(key)}:${productionCanonicalJson(item)}`,
      )
      .join(",")}}`;
  }
  throw new Error("production canonical JSON contains an unsupported value");
}

function productionCanonicalHash(value: unknown): string {
  return createHash("sha256").update(productionCanonicalJson(value)).digest("hex");
}

describe("production StoryGraph wire", () => {
  it("matches the shared Go and Python Unicode and identity golden", () => {
    expect(productionCanonicalJson(fixture.canonical_unicode_root)).toBe(
      fixture.canonical_unicode_json,
    );
    expect(productionCanonicalHash(fixture.canonical_unicode_root)).toBe(
      fixture.canonical_unicode_hash,
    );

    const invocation = fixture.valid_invocation as Record<string, unknown>;
    const payload = invocation.payload as Record<string, unknown>;
    const material = {
      wire_schema_version: invocation.wire_schema_version,
      stage_release: invocation.stage_release,
      control: invocation.control,
      budget: invocation.budget,
      payload: {
        ...payload,
        source_refs: [...(payload.source_refs as Record<string, unknown>[])],
        upstream_candidates: [
          ...(payload.upstream_candidates as Record<string, unknown>[]),
        ],
      },
    };
    const inputHash = productionCanonicalHash(material);
    expect(inputHash).toBe(fixture.expected_input_hash);

    const shard = payload.shard as Record<string, string>;
    const stageIdentity = {
      identity_contract_id: "storygraph-stage-instance-production",
      variant_key: payload.variant,
      scope: payload.scope,
      shard_manifest_hash: shard.manifest_hash,
      shard_key: shard.shard_key,
      input_hash: inputHash,
    };
    expect(productionCanonicalHash(stageIdentity)).toBe(
      fixture.expected_stage_instance_key,
    );
  });
});
