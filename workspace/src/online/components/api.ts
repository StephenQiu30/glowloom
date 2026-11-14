const base = process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? "";
export async function api<T>(path: string, body?: unknown): Promise<T> {
  let csrf = "";
  if (body !== undefined && path !== "/session") {
    const session = await fetch(base + "/api/session/", { cache: "no-store" });
    if (!session.ok) {
      if (session.status === 401) location.assign(base + "/login/");
      throw new Error("会话已过期，请重新登录。");
    }
    csrf = (await session.json()).csrf;
  }
  const response = await fetch(
    base + "/api" + path + (path.includes("?") ? "" : "/"),
    {
      method: body === undefined ? "GET" : "POST",
      headers:
        body === undefined
          ? undefined
          : { "content-type": "application/json", "x-workspace-csrf": csrf },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
    },
  );
  const data = await response.json();
  if (!response.ok) {
    const messages: Record<string, string> = {
      revision_conflict: "原文已被修改，提案保留。请重新读取当前原文后审阅。",
      version_conflict: "运行版本已变化，请刷新后重试。",
      inputs_stale: "输入已变化，请复核并更新上下文。",
      workflow_snapshot_stale: "流程定义已变化，请重建在线知识快照。",
      database_unconfigured: "尚未配置在线运行存储。",
      bridge_unconfigured: "尚未连接受控执行桥。",
      bridge_unknown: "执行桥回执未知，请查询原运行，不要重新派发。",
      tts_disabled_by_owner: "真实TTS生成暂未启用。",
      invalid_token: "访问令牌不正确。",
      verification_unknown: "检查仍在进行或回执未知，请查询原检查。",
    };
    throw new Error(
      messages[data.error] ?? data.error ?? "请求失败，请保留当前内容后重试。",
    );
  }
  return data as T;
}
