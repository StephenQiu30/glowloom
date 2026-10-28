/** Stable command codes become actionable copy, without exposing transport internals. */
export function assetActionFeedback(code: string) {
  if (code === "revision_conflict" || code === "project_revision_conflict")
    return "内容已更新，请重试此项。";
  if (code === "deleted" || code === "not_found")
    return "内容已移除或不可访问，请刷新列表。";
  if (code === "forbidden" || code === "read_only")
    return "当前账号无法修改此项，请检查空间权限。";
  if (code === "no_results") return "暂无可收藏的结果，请在生成完成后重试。";
  return "未能完成此项，请刷新列表后重试。";
}
