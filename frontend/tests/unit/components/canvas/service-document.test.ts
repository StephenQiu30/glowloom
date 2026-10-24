import { expect, it } from "vitest";
import {
  revision,
  fromServiceDocument,
  validateHistoricalSnapshot,
} from "@/components/canvas/service-document";
const dto: API.DocumentDTO = {
  id: "10000000-0000-4000-8000-000000000001",
  project_id: "10000000-0000-4000-8000-000000000002",
  revision: "9007199254740993",
  schema_version: 1,
  operation_version: 1,
  permissions: { read: true, write: true },
  nodes: [],
  edges: [],
};
it("preserves canonical 64-bit revisions", () => {
  expect(revision(dto.revision)).toBe(dto.revision);
  for (const bad of [1, "01", "-1", "9223372036854775808"])
    expect(() => revision(bad)).toThrow();
});
it("checks document/project identity and readable schema", () => {
  expect(fromServiceDocument(dto, dto.id!, dto.project_id!).nodes).toEqual([]);
  expect(() => fromServiceDocument(dto, "wrong", dto.project_id!)).toThrow();
  expect(() =>
    fromServiceDocument(
      { ...dto, schema_version: 99 },
      dto.id!,
      dto.project_id!,
    ),
  ).toThrow();
});
it("accepts an empty historical graph without mutating the input", () => {
  const before = JSON.stringify(dto);
  validateHistoricalSnapshot(dto);
  expect(JSON.stringify(dto)).toBe(before);
});
