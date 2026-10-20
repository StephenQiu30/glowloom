import { expect, it } from "vitest";
import { ProjectPresentationStore } from "@/components/projects/project-presentation-store";
import type { Project } from "@/components/projects/project";
const id = "00000000-0000-4000-8000-000000000001";
const project = (name: string, revision: number): Project => ({
  id,
  canvas_id: "00000000-0000-4000-8000-000000000002",
  name,
  revision,
  aspect_ratio: "16:9",
  style_key: null,
  update_time: "2026-10-19T00:00:00Z",
  status: "active",
  is_delete: false,
});
it("a late lower project revision cannot replace the current name", () => {
  const store = new ProjectPresentationStore();
  store.accept("actor/scope", project("新名", 2));
  store.accept("actor/scope", project("旧名", 1));
  expect(store.read("actor/scope", id)?.name).toBe("新名");
});
it("private presentation and unknown rename are cleared together and never cross namespaces", () => {
  const store = new ProjectPresentationStore();
  store.accept("A/scope", project("私有项目", 1));
  store.setRename("A/scope", id, {
    key: "original-key",
    body: { name: "待确认名称", expected_revision: 1 },
    phase: "unknown",
  });
  expect(store.read("B/scope", id)).toBeNull();
  expect(store.rename("B/scope", id)).toBeUndefined();
  store.clear();
  expect(store.read("A/scope", id)).toBeNull();
  expect(store.rename("A/scope", id)).toBeUndefined();
});
it("same-revision metadata cannot regress newer independently committed content activity", () => {
  const store = new ProjectPresentationStore();
  store.accept("A/scope", {
    ...project("名称", 1),
    last_modified_at: "2026-10-19T01:00:00Z",
    last_modified_basis: "content",
  });
  store.accept("A/scope", {
    ...project("名称", 1),
    last_modified_at: "2026-10-19T00:00:00Z",
    last_modified_basis: "created",
  });
  expect(store.read("A/scope", id)?.last_modified_at).toBe(
    "2026-10-19T01:00:00Z",
  );
  expect(store.read("A/scope", id)?.last_modified_basis).toBe("content");
});
