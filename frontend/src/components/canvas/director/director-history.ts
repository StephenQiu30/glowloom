import { directorFingerprint } from "./director-model";
import {
  type DirectorDocument,
  type DirectorMode,
  type DirectorSceneData,
} from "./director-workspaces";
type Change = {
  before: DirectorSceneData;
  after: DirectorSceneData;
  epoch: string;
};
/** Workspaces own separate undo stacks. Remote/epoch changes cannot be silently overwritten. */
export class DirectorHistory {
  private undo: Record<DirectorMode, Change[]> = { static: [], dynamic: [] };
  private redo: Record<DirectorMode, Change[]> = { static: [], dynamic: [] };
  record(before: DirectorDocument, after: DirectorDocument, epoch: string) {
    for (const mode of ["static", "dynamic"] as const) {
      if (
        directorFingerprint(before.workspaces[mode]) ===
        directorFingerprint(after.workspaces[mode])
      )
        continue;
      this.undo[mode].push({
        before: structuredClone(before.workspaces[mode]),
        after: structuredClone(after.workspaces[mode]),
        epoch,
      });
      if (this.undo[mode].length > 30) this.undo[mode].shift();
      this.redo[mode] = [];
    }
  }
  available(mode: DirectorMode, action: "undo" | "redo") {
    return this[action][mode].length > 0;
  }
  prepare(document: DirectorDocument, epoch: string, action: "undo" | "redo") {
    const mode = document.mode,
      entry = this[action][mode].at(-1);
    if (!entry)
      throw new Error(
        action === "undo"
          ? "当前工作区没有可撤销的编辑"
          : "当前工作区没有可重做的编辑",
      );
    const expected = action === "undo" ? entry.after : entry.before;
    if (
      entry.epoch !== epoch ||
      directorFingerprint(document.workspaces[mode]) !==
        directorFingerprint(expected)
    )
      throw new Error("工作区已由协作者或历史恢复改变，无法撤销旧编辑");
    const next = structuredClone(document);
    next.workspaces[mode] = structuredClone(
      action === "undo" ? entry.before : entry.after,
    );
    return {
      next,
      accept: () => {
        if (this[action][mode].at(-1) !== entry)
          throw new Error("历史记录已变化");
        this[action][mode].pop();
        this[action === "undo" ? "redo" : "undo"][mode].push(entry);
      },
    };
  }
}
