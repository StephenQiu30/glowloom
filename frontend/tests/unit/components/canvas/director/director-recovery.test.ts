import { describe, expect, it } from "vitest";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
import { parseDirectorRecoveryFile } from "@/components/canvas/director/director-recovery";

describe("director recovery files", () => {
  it("restores only a validated scene for its exact canvas and node", () => {
    const director = emptyDirectorConfig();
    const value = JSON.stringify({
      version: 1,
      canvas_id: "canvas",
      node_id: "node",
      director,
    });
    expect(parseDirectorRecoveryFile(value, "canvas", "node").id).toBe(
      director.id,
    );
    expect(() => parseDirectorRecoveryFile(value, "other", "node")).toThrow(
      "不属于",
    );
    expect(() => parseDirectorRecoveryFile(value, "canvas", "other")).toThrow(
      "不属于",
    );
  });
  it("rejects malformed and oversized recovery payloads", () => {
    expect(() =>
      parseDirectorRecoveryFile(
        JSON.stringify({
          version: 1,
          canvas_id: "c",
          node_id: "n",
          director: { version: 2 },
        }),
        "c",
        "n",
      ),
    ).toThrow();
    expect(() =>
      parseDirectorRecoveryFile(" ".repeat(600_001), "c", "n"),
    ).toThrow("过大");
  });
});
