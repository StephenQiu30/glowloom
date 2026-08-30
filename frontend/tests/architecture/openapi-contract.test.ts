import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const frontendRoot = resolve(import.meta.dirname, "../..");
const repositoryRoot = resolve(frontendRoot, "..");

describe("Go Backend OpenAPI type ownership", () => {
  it("keeps generated frontend types scoped to the current MVP contract", () => {
    const openapi = JSON.parse(
      readFileSync(
        resolve(repositoryRoot, "backend/api/openapi/lanverse-public-api.json"),
        "utf8",
      ),
    ) as { paths: Record<string, unknown>; components: { schemas: Record<string, unknown> } };
    const generated = readFileSync(
      resolve(frontendRoot, "src/api/schema.d.ts"),
      "utf8",
    );

    expect(openapi.paths).toHaveProperty(
      "/api/production-bibles/{bible_id}/review-decisions",
    );
    expect(openapi.components.schemas).toHaveProperty("ProductionBibleResponse");
    expect(openapi.paths).toHaveProperty(
      "/api/projects/{project_id}/human-tasks",
    );
    expect(openapi.paths).toHaveProperty(
      "/api/review-decisions/{review_decision_id}/resume",
    );
    expect(openapi.components.schemas).toHaveProperty(
      "HumanGateCoordinationResponse",
    );
    expect(openapi.paths).toHaveProperty(
      "/api/workflow-runs/{workflow_run_id}/story-analysis-recoveries",
    );
    expect(openapi.components.schemas).toHaveProperty(
      "StoryAnalysisRecoveryResponse",
    );
    expect(generated).toContain("review_decisions");
    expect(generated).toContain("HumanGateCoordinationResponse");
    expect(generated).toContain("StoryAnalysisRecoveryResponse");
    expect(generated).not.toMatch(/migration|kafka|redis|sqlalchemy/i);
  });
});
