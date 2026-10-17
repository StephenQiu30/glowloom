"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  createPlatformUserExport,
  createTeamMemberExport,
  getPlatformUserExport,
  getTeamMemberExport,
  downloadPlatformUserExport,
  downloadTeamMemberExport,
  getPlatformUserStatistics,
  getTeamMemberStatistics,
} from "@/api/identity";
import { getSessionGeneration, RequestError } from "@/lib/request";
import { authErrorMessage } from "@/components/auth/contracts";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { Button } from "@/components/ui/button";
const states: Record<string, string> = {
  pending: "排队中",
  running: "生成中",
  completed: "可下载",
  failed: "生成失败",
  expired: "已到期",
  revoked: "权限已失效",
};
export function filterDescription(filter: API.DirectoryExportFilter) {
  const parts: string[] = [];
  if (filter.search) parts.push(`搜索“${filter.search}”`);
  if (filter.status)
    parts.push(
      `状态 ${({ active: "启用", disabled: "禁用", deleted: "已注销", suspended: "暂停", removed: "已移除" } as Record<string, string>)[filter.status] ?? filter.status}`,
    );
  if (filter.role)
    parts.push(
      `团队角色 ${filter.role === "team_admin" ? "团队管理员" : "普通成员"}`,
    );
  if (filter.platform_role)
    parts.push(
      `平台角色 ${filter.platform_role === "platform_admin" ? "平台管理员" : "创作者"}`,
    );
  if (filter.verified !== undefined)
    parts.push(filter.verified ? "邮箱已验证" : "邮箱未验证");
  if (filter.in_team !== undefined)
    parts.push(filter.in_team ? "有有效团队关系" : "无有效团队关系");
  if (filter.from) parts.push(`起 ${new Date(filter.from).toLocaleString()}`);
  if (filter.to)
    parts.push(`止 ${new Date(filter.to).toLocaleString()}（不含）`);
  if (filter.order)
    parts.push(filter.order === "created_asc" ? "时间升序" : "时间降序");
  return parts.join("；") || "全部记录";
}
function count(value: unknown) {
  if (typeof value !== "number" || !Number.isInteger(value) || value < 0)
    throw new RequestError("invalid_receipt");
  return value;
}
function validateExport(job: API.DirectoryExportView) {
  if (
    !job ||
    !job.id ||
    !(job.status in states) ||
    !Number.isFinite(Date.parse(job.as_of))
  )
    throw new RequestError("invalid_receipt");
  count(job.row_count);
  if (job.row_count > 10000) throw new RequestError("invalid_receipt");
  if (
    job.status === "completed" &&
    (typeof job.expires_at !== "string" ||
      !Number.isFinite(Date.parse(job.expires_at)))
  )
    throw new RequestError("invalid_receipt");
  return job;
}
export function DirectoryTools({
  filter,
  teamID,
  reload = 0,
}: {
  filter: API.DirectoryExportFilter;
  teamID?: string;
  reload?: number;
}) {
  const [statistics, setStatistics] = useState<
    API.PlatformStatisticsView | API.TeamMemberStatisticsView | null
  >(null);
  const [statsError, setStatsError] = useState("");
  const [retry, setRetry] = useState(0);
  const [job, setJob] = useState<API.DirectoryExportView | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [forbidden, setForbidden] = useState(false);
  const [expired, setExpired] = useState(false);
  const [action, setAction] = useState<ConfirmAction | null>(null);
  const flight = useRef<AbortController | null>(null);
  const locked = useRef(false);
  const clearDenied = useCallback((error: unknown) => {
    if (error instanceof RequestError && [401, 403].includes(error.status)) {
      flight.current?.abort();
      setPending(false);
      setAction(null);
      setStatistics(null);
      setJob(null);
      setStatsError("当前权限已变化，请重新读取统计。");
      setForbidden(true);
    }
  }, []);
  useEffect(() => {
    const c = new AbortController();
    (teamID
      ? getTeamMemberStatistics({ id: teamID, ...filter }, { signal: c.signal })
      : getPlatformUserStatistics(filter, { signal: c.signal })
    )
      .then((value) => {
        if (c.signal.aborted) return;
        if (
          !value ||
          !Number.isFinite(Date.parse(value.as_of)) ||
          !value.filter
        )
          throw new RequestError("invalid_receipt");
        if ("total_users" in value) {
          [
            value.total_users,
            value.verified_users,
            value.active_team_users,
            value.membership_count,
            value.account_states?.active,
            value.account_states?.disabled,
            value.account_states?.deleted,
          ].forEach(count);
        } else {
          [
            value.total_members,
            value.member_states?.active,
            value.member_states?.suspended,
            value.member_states?.removed,
          ].forEach(count);
        }
        setForbidden(false);
        setStatistics(value);
        setStatsError("");
      })
      .catch((e) => {
        if (!c.signal.aborted) {
          setStatistics(null);
          setStatsError(authErrorMessage(e));
          clearDenied(e);
        }
      });
    return () => c.abort();
  }, [filter, teamID, retry, reload, clearDenied]);
  useEffect(() => () => flight.current?.abort(), []);
  useEffect(() => {
    if (job?.status !== "completed" || !job.expires_at) return;
    const remaining = Date.parse(job.expires_at) - Date.now();
    const timer = setTimeout(() => setExpired(true), Math.max(0, remaining));
    return () => clearTimeout(timer);
  }, [job]);
  async function inspect() {
    if (!job || forbidden || locked.current) return;
    locked.current = true;
    setPending(true);
    setError("");
    const c = new AbortController();
    flight.current = c;
    try {
      const next = validateExport(
        await (teamID
          ? getTeamMemberExport(
              { id: teamID, eid: job.id },
              { signal: c.signal },
            )
          : getPlatformUserExport({ id: job.id }, { signal: c.signal })),
      );
      if (!c.signal.aborted) {
        setJob(next);
        setExpired(
          next.status === "expired" ||
            Boolean(
              next.expires_at && Date.parse(next.expires_at) <= Date.now(),
            ),
        );
      }
    } catch (e) {
      if (!c.signal.aborted) {
        clearDenied(e);
        setError(
          e instanceof RequestError && e.status === 404
            ? "导出已到期或不再可访问，请重新创建。"
            : authErrorMessage(e),
        );
        if (e instanceof RequestError && [401, 403, 404].includes(e.status)) {
          setJob(null);
          setExpired(true);
        }
      }
    } finally {
      locked.current = false;
      if (!c.signal.aborted) setPending(false);
    }
  }
  async function download() {
    if (
      !job ||
      forbidden ||
      expired ||
      job.status !== "completed" ||
      locked.current
    )
      return;
    locked.current = true;
    setPending(true);
    setError("");
    const c = new AbortController();
    flight.current = c;
    const generation = getSessionGeneration();
    try {
      const options = { signal: c.signal, responseType: "blob" as const };
      const blob = await (teamID
        ? downloadTeamMemberExport({ id: teamID, eid: job.id }, options)
        : downloadPlatformUserExport({ id: job.id }, options));
      if (c.signal.aborted || generation !== getSessionGeneration()) return;
      if (!(blob instanceof Blob)) throw new RequestError("invalid_receipt");
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = teamID ? "team-members.csv" : "platform-users.csv";
      document.body.append(link);
      try {
        link.click();
      } finally {
        link.remove();
        setTimeout(() => URL.revokeObjectURL(url), 0);
      }
    } catch (e) {
      if (!c.signal.aborted) {
        clearDenied(e);
        setError(authErrorMessage(e));
        if (e instanceof RequestError && [401, 403, 404].includes(e.status))
          setJob(null);
      }
    } finally {
      locked.current = false;
      if (!c.signal.aborted) setPending(false);
    }
  }
  return (
    <section
      className="my-4 grid gap-3 rounded-xl border p-4"
      aria-label={teamID ? "成员统计与导出" : "用户统计与导出"}
    >
      <h2>{teamID ? "成员统计与导出" : "用户统计与导出"}</h2>
      <p className="text-sm break-words">
        目录筛选：{filterDescription(filter)}
      </p>
      {statistics ? (
        <>
          <p className="text-sm">
            统计时间：{new Date(statistics.as_of).toLocaleString()}
          </p>
          {"total_users" in statistics ? (
            <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3">
              {[
                ["账号总数", statistics.total_users],
                ["启用", statistics.account_states.active],
                ["禁用", statistics.account_states.disabled],
                ["已注销", statistics.account_states.deleted],
                ["已验证邮箱", statistics.verified_users],
                ["有效团队用户（去重）", statistics.active_team_users],
                ["成员关系数量", statistics.membership_count],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd className="text-xl">{value}</dd>
                </div>
              ))}
            </dl>
          ) : (
            <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              {[
                ["成员关系总数", statistics.total_members],
                ["有效成员", statistics.member_states.active],
                ["暂停成员", statistics.member_states.suspended],
                ["已移除", statistics.member_states.removed],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd className="text-xl">{value}</dd>
                </div>
              ))}
            </dl>
          )}
          <p className="text-sm text-muted-foreground">
            成员关系与可登录人数分别统计；账号数不按团队重复相加。
          </p>
        </>
      ) : !statsError ? (
        <p role="status">正在读取统计…</p>
      ) : null}
      {statsError ? (
        <FeedbackToast message={["统计暂不可用：", statsError].join(" ")} />
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => setRetry((n) => n + 1)}>
          刷新统计
        </Button>
        <Button
          disabled={pending || forbidden}
          onClick={() =>
            setAction({
              title: "创建目录导出",
              effect: `按当前筛选创建最多 10000 行的私有 CSV 快照。完成后 15 分钟到期，下载时重新检查权限。筛选：${filterDescription(filter)}`,
              run: async (_reason, signal, requestID) => {
                const options = {
                  signal,
                  headers: { "Idempotency-Key": requestID },
                };
                try {
                  const next = validateExport(
                    await (teamID
                      ? createTeamMemberExport(
                          { id: teamID },
                          { filter },
                          options,
                        )
                      : createPlatformUserExport({ filter }, options)),
                  );
                  if (!signal.aborted) {
                    setJob(next);
                    setExpired(
                      Boolean(
                        next.expires_at &&
                        Date.parse(next.expires_at) <= Date.now(),
                      ),
                    );
                    setError("");
                  }
                } catch (e) {
                  if (!signal.aborted) clearDenied(e);
                  throw e;
                }
              },
            })
          }
        >
          创建 CSV 导出
        </Button>
      </div>
      {job ? (
        <article
          className="grid gap-2 rounded-lg bg-muted p-3"
          aria-label="当前导出"
        >
          <p>导出状态：{expired ? "已到期" : states[job.status]}</p>
          <p>
            快照时间：{new Date(job.as_of).toLocaleString()} · {job.row_count}{" "}
            行
          </p>
          {job.status === "completed" ? (
            <p>下载有效期至：{new Date(job.expires_at).toLocaleString()}</p>
          ) : (
            <p>尚未生成可下载文件。</p>
          )}
          {job.status === "failed" ? (
            <FeedbackToast
              message={
                job.error_category === "authority_revoked"
                  ? "当前权限已变化，无法生成。"
                  : "导出生成失败，请重新读取状态或创建新导出。"
              }
            />
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={pending || expired}
              variant="outline"
              onClick={() => void inspect()}
            >
              检查导出状态
            </Button>
            <Button
              disabled={
                pending || forbidden || expired || job.status !== "completed"
              }
              onClick={() => void download()}
            >
              {pending ? "正在处理…" : "下载 CSV"}
            </Button>
          </div>
        </article>
      ) : null}
      {error && !(forbidden && statsError) ? (
        <FeedbackToast message={error} />
      ) : null}
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={() => {}}
      />
    </section>
  );
}
