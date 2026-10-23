export function isCurrentCanvasVersion(
  version: Pick<
    CanvasContract.CanvasSnapshotSummaryVO,
    "sourceRevision" | "projectBindingRevision"
  >,
  document: Pick<
    CanvasContract.CanvasVO,
    "revision" | "projectBindingRevision"
  >,
) {
  return (
    String(version.sourceRevision) === String(document.revision) &&
    String(version.projectBindingRevision) ===
      String(document.projectBindingRevision)
  );
}

/** A restore receipt confirms the command; only a fresh HEAD may become editable. */
export function validateRestoredCanvas(
  document: CanvasContract.CanvasVO,
  receipt: CanvasContract.CanvasCommandResultVO,
) {
  if (
    String(document.canvasId) !== String(receipt.canvasId) ||
    BigInt(document.revision) < BigInt(receipt.revision) ||
    String(document.projectBindingRevision) !==
      String(receipt.projectBindingRevision) ||
    (String(document.revision) === String(receipt.revision) &&
      document.graphHash !== receipt.graphHash)
  ) {
    throw new Error("恢复后的画布尚未确认，请查询原命令后重试读取");
  }
  return BigInt(document.revision) > BigInt(receipt.revision);
}

export const canvasVersionReasons: Record<
  CanvasContract.CanvasSnapshotSummaryVO["reason"],
  string
> = {
  INITIAL: "创建画布",
  AUTO_SAVE: "自动保存",
  BEFORE_CHANGE: "修改前保留",
  CHECKPOINT: "手动保留",
  BEFORE_RESTORE: "恢复前保留",
  RESTORE_RESULT: "恢复历史版本",
  ATTACH_BASELINE: "归入项目",
  MIGRATION_BASELINE: "迁移保留的当前版本",
};
