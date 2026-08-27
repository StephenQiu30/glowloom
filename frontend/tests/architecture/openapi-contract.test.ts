import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const frontendRoot = resolve(import.meta.dirname, "../..");
const repositoryRoot = resolve(frontendRoot, "..");

describe("Go Backend OpenAPI type ownership", () => {
  it("keeps generated frontend types scoped to the current MVP contract", () => {
    const openapi = JSON.parse(
      readFileSync(
        resolve(repositoryRoot, "backend/api/openapi/lanverse-v1.json"),
        "utf8",
      ),
    ) as { paths: Record<string, unknown>; components: { schemas: Record<string, unknown> } };
    const generated = readFileSync(
      resolve(frontendRoot, "src/api/schema.d.ts"),
      "utf8",
    );

    expect(openapi.paths).toHaveProperty(
      "/api/v1/production-bibles/{bible_id}/review-decisions",
    );
    expect(openapi.components.schemas).toHaveProperty("ProductionBibleResponse");
    expect(openapi.paths).toHaveProperty(
      "/api/v1/projects/{project_id}/human-tasks",
    );
    expect(openapi.paths).toHaveProperty(
      "/api/v1/review-decisions/{review_decision_id}/resume",
    );
    expect(openapi.components.schemas).toHaveProperty(
      "HumanGateCoordinationResponse",
    );
    expect(generated).toContain("review_decisions");
    expect(generated).toContain("HumanGateCoordinationResponse");
    expect(generated).not.toMatch(/migration|kafka|redis|sqlalchemy/i);
  });
});
