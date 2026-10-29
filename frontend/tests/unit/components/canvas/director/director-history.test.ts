import { expect, it } from "vitest";
import { DirectorHistory } from "@/components/canvas/director/director-history";
import { migrateDirectorDocument } from "@/components/canvas/director/director-workspaces";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
it("undoes only the active workspace and rejects stale collaboration baselines", () => {
  const history = new DirectorHistory(),
    a = migrateDirectorDocument(emptyDirectorConfig()),
    b = structuredClone(a);
  b.workspaces.static.background = "#ffffff";
  history.record(a, b, "epoch");
  const c = structuredClone(b);
  c.mode = "dynamic";
  c.workspaces.dynamic.background = "#ff0000";
  history.record(b, c, "epoch");
  const undo = history.prepare(c, "epoch", "undo");
  expect(undo.next.workspaces.static.background).toBe("#ffffff");
  expect(undo.next.workspaces.dynamic.background).toBe(
    a.workspaces.dynamic.background,
  );
  undo.accept();
  expect(history.available("static", "undo")).toBe(true);
  expect(history.available("dynamic", "redo")).toBe(true);
  const changed = structuredClone(b);
  changed.workspaces.static.background = "#00ff00";
  expect(() => history.prepare(changed, "epoch", "undo")).toThrow();
  expect(() => history.prepare(b, "new-epoch", "undo")).toThrow();
});
