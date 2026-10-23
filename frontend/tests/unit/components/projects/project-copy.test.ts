import { expect, it } from "vitest";
import {
  copyIntent,
  requireCopy,
  restoreCopyIntent,
} from "@/components/projects/project-copy";
const source = "11111111-1111-4111-8111-111111111111";
const job = "22222222-2222-4222-8222-222222222222";
const target = "33333333-3333-4333-8333-333333333333";
it("freezes the source revision and preserves the original request on recovery", () => {
  const intent = copyIntent(source, 7, "副本");
  expect(restoreCopyIntent(JSON.stringify(intent), source)).toEqual(intent);
  expect(intent.body).toEqual({ expected_revision: 7, target_name: "副本" });
  expect(() => restoreCopyIntent(JSON.stringify(intent), target)).toThrow();
  expect(() => copyIntent(source, 7, " ")).toThrow();
});
it("rejects mismatched copies and never treats queued as completion", () => {
  const value = {
    id: job,
    source_project_id: source,
    target_project_id: target,
    revision: 1,
    status: "queued",
    source_revision: 7,
    target_name: "副本",
  };
  expect(requireCopy(value, source).status).toBe("queued");
  expect(() => requireCopy(value, target)).toThrow();
  expect(() =>
    requireCopy({ ...value, target_project_id: source }, source),
  ).toThrow();
  expect(() => requireCopy({ ...value, revision: NaN }, source)).toThrow();
});
