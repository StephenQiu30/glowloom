"use client";

import {
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ApiError } from "@/lib/request";
import type { MediaAsset } from "./queries";
import {
  depthActions,
  depthStatusLabels,
  depthStageLabels,
  depthUUID,
  newerDepth,
  type DepthJob,
  type DepthPreview,
  type DepthSource,
} from "./media-depth-model";
import {
  clearDepthIntent,
  loadDepthIntent,
  sameDepthScope,
  saveDepthIntent,
  unknownDepthWrite,
  type DepthIntent,
  type DepthScope,
} from "./media-depth-intent";
import {
  MEDIA_DEPTHS_KEY,
  downloadDepth,
  getDepth,
  listDepths,
  runDepthIntent,
} from "./media-depth-queries";
import { MediaDepthPreviewDialog } from "./media-depth-preview-dialog";
import { DepthFailure } from "./media-depth-failure";

export type MediaDepthPanelProps = {
  projectId: string;
  canvasId: string;
  nodeId: string;
  jobId?: string;
  onPrepareSource?: () => Promise<DepthSource>;
  onAdopt?: (job: DepthJob, asset: MediaAsset) => Promise<boolean>;
  onJobSelected?: (id: string) => void;
  onLockChange?: (locked: boolean) => void;
};
const subscribeOrigin = () => () => {};
const browserOrigin = () => window.location.origin;
const serverOrigin = () => "";
export function MediaDepthPanel(props: MediaDepthPanelProps) {
  const origin = useSyncExternalStore(
      subscribeOrigin,
      browserOrigin,
      serverOrigin,
    ),
    session = useId();
  const valid = [
    props.projectId,
    props.canvasId,
    props.nodeId,
    ...(props.jobId ? [props.jobId] : []),
  ].every((id) => depthUUID.safeParse(id).success);
  const first = useQuery({
    queryKey: [
      ...MEDIA_DEPTHS_KEY,
      props.projectId,
      props.canvasId,
      props.nodeId,
      "scope",
      session,
    ],
    queryFn: ({ signal }) =>
      listDepths(
        props.projectId,
        props.canvasId,
        props.nodeId,
        undefined,
        signal,
      ),
    enabled: valid,
    retry: false,
    staleTime: 0,
    refetchOnMount: "always",
    refetchOnWindowFocus: false,
    refetchInterval: 5000,
  });
  if (!valid)
    return (
      <DepthFailure
        title="深度任务链接无效"
        error={new Error("项目、画布、视频节点和任务必须是有效 UUID。")}
      />
    );
  if (first.error && !first.data)
    return (
      <DepthFailure
        title="深度任务读取失败"
        error={first.error}
        retry={() => void first.refetch()}
      />
    );
  if (!first.data || !origin)
    return <p role="status">正在读取深度任务与当前身份…</p>;
  const scope: DepthScope = {
    origin,
    projectId: props.projectId,
    canvasId: props.canvasId,
    nodeId: props.nodeId,
    actorId: first.data.current_actor_id,
    orgId: first.data.current_org_id,
  };
  return (
    <DepthSession
      key={[
        scope.actorId,
        scope.orgId,
        scope.projectId,
        scope.canvasId,
        scope.nodeId,
      ].join(":")}
      {...props}
      scope={scope}
      first={first.data}
      scopeError={first.error}
    />
  );
}
function DepthSession({
  scope,
  first,
  scopeError,
  jobId,
  onPrepareSource,
  onAdopt,
  onJobSelected,
  onLockChange,
}: MediaDepthPanelProps & {
  scope: DepthScope;
  first: Awaited<ReturnType<typeof listDepths>>;
  scopeError: unknown;
}) {
  const cache = useQueryClient(),
    inFlight = useRef(false),
    seenSuccess = useRef("");
  const [recovery, setRecovery] = useState<{
    intent: DepthIntent | null;
    error?: unknown;
  }>(() => {
    try {
      return { intent: loadDepthIntent(window.sessionStorage, scope) };
    } catch (error) {
      return { intent: null, error };
    }
  });
  const [pending, setPending] = useState(false),
    [error, setError] = useState<unknown>(null),
    [conflict, setConflict] = useState(false),
    [notice, setNotice] = useState("");
  const [selected, setSelected] = useState(
      recovery.intent && recovery.intent.action !== "create"
        ? recovery.intent.jobId
        : jobId,
    ),
    [preview, setPreview] = useState(false);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]),
    [pageIndex, setPageIndex] = useState(0);
  const scopeKey = [
    ...MEDIA_DEPTHS_KEY,
    scope.projectId,
    scope.actorId,
    scope.orgId,
    scope.canvasId,
    scope.nodeId,
  ] as const;
  const page = useQuery({
    queryKey: [...scopeKey, "list", cursors[pageIndex]],
    queryFn: ({ signal }) =>
      listDepths(
        scope.projectId,
        scope.canvasId,
        scope.nodeId,
        cursors[pageIndex],
        signal,
      ),
    enabled: pageIndex > 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: pageIndex > 0 ? 5000 : false,
  });
  const detail = useQuery({
    queryKey: [...scopeKey, "detail", selected],
    queryFn: async ({ signal }) => {
      const value = await getDepth(scope.projectId, selected!, signal);
      if (
        value.source.canvas_id !== scope.canvasId ||
        value.source.node_id !== scope.nodeId
      )
        throw new ApiError(502, "invalid_response");
      return value;
    },
    enabled: Boolean(selected),
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
    structuralSharing: (current, incoming) =>
      newerDepth(current as DepthJob | undefined, incoming as DepthJob),
    refetchInterval: (query) =>
      query.state.data &&
      !["succeeded", "cancelled"].includes(query.state.data.status)
        ? 2000
        : false,
  });
  const job = detail.data,
    listed = pageIndex === 0 ? first : page.data;
  const locked = pending || Boolean(recovery.intent),
    disabled = locked || Boolean(recovery.error) || conflict;
  useEffect(() => {
    onLockChange?.(locked);
    return () => onLockChange?.(false);
  }, [locked, onLockChange]);
  useEffect(() => {
    if (!locked) return;
    const href = window.location.href;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    const back = (event: PopStateEvent) => {
      event.stopImmediatePropagation();
      window.history.pushState(window.history.state, "", href);
    };
    const navigate = (event: MouseEvent) => {
      if (event.target instanceof Element && event.target.closest("a[href]")) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    window.addEventListener("beforeunload", warn);
    window.addEventListener("popstate", back, true);
    document.addEventListener("click", navigate, true);
    return () => {
      window.removeEventListener("beforeunload", warn);
      window.removeEventListener("popstate", back, true);
      document.removeEventListener("click", navigate, true);
    };
  }, [locked]);
  useEffect(() => {
    if (
      !job ||
      job.status !== "succeeded" ||
      seenSuccess.current === `${job.id}:${job.attempt}`
    )
      return;
    seenSuccess.current = `${job.id}:${job.attempt}`;
    void cache.invalidateQueries({
      queryKey: ["canvas", "media", scope.projectId],
    });
  }, [job, scope.projectId, cache]);
  async function freshScope() {
    const value = await listDepths(
      scope.projectId,
      scope.canvasId,
      scope.nodeId,
    );
    if (
      !sameDepthScope(scope, {
        ...scope,
        actorId: value.current_actor_id,
        orgId: value.current_org_id,
      })
    )
      throw new Error(
        "当前工作区身份已变化。原请求已保留，不会在其他身份下重放。请重新读取深度任务。",
      );
    return value;
  }
  async function send(intent: DepthIntent, replay: boolean) {
    if (inFlight.current || recovery.error) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    setNotice("");
    let posted = false,
      persisted = Boolean(recovery.intent);
    try {
      const [, facts] = await Promise.all([
        freshScope(),
        intent.action === "create"
          ? Promise.resolve(undefined)
          : getDepth(scope.projectId, intent.jobId),
      ]);
      if (
        facts &&
        (facts.source.canvas_id !== scope.canvasId ||
          facts.source.node_id !== scope.nodeId)
      )
        throw new ApiError(502, "invalid_response");
      if (
        !replay &&
        intent.action !== "create" &&
        (!facts ||
          facts.revision !== intent.body.revision ||
          !depthActions(facts)[intent.action])
      )
        throw new ApiError(409, "media_depth_conflict");
      saveDepthIntent(window.sessionStorage, intent);
      persisted = true;
      setRecovery({ intent });
      posted = true;
      const accepted = await runDepthIntent(intent);
      clearDepthIntent(window.sessionStorage, scope);
      setRecovery({ intent: null });
      setConflict(false);
      setNotice("操作已受理，请以最新任务状态确认结果。");
      setSelected(accepted.id);
      onJobSelected?.(accepted.id);
      await cache.invalidateQueries({
        queryKey: [...MEDIA_DEPTHS_KEY, scope.projectId],
      });
    } catch (failure) {
      setError(failure);
      if (posted && !unknownDepthWrite(failure)) {
        try {
          clearDepthIntent(window.sessionStorage, scope);
          setRecovery({ intent: null });
        } catch (storageError) {
          setRecovery({ intent, error: storageError });
          setError(storageError);
        }
      } else if (persisted) setRecovery({ intent });
      if (failure instanceof ApiError && failure.status === 409)
        setConflict(true);
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }
  async function create() {
    if (disabled || !onPrepareSource || inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    let source: DepthSource;
    try {
      source = await onPrepareSource();
      if (
        source.canvas_id !== scope.canvasId ||
        source.node_id !== scope.nodeId
      )
        throw new Error("来源视频已变化，请重新选择。");
    } catch (failure) {
      setError(failure);
      if (failure instanceof ApiError && failure.status === 409)
        setConflict(true);
      return;
    } finally {
      inFlight.current = false;
      setPending(false);
    }
    await send(
      {
        ...scope,
        version: 1,
        key: crypto.randomUUID(),
        action: "create",
        body: source,
      },
      false,
    );
  }
  function control(action: "cancel" | "retry" | "reconcile") {
    if (disabled || !job || !depthActions(job)[action]) return;
    void send(
      {
        ...scope,
        version: 1,
        key: crypto.randomUUID(),
        action,
        jobId: job.id,
        body: { project_id: scope.projectId, revision: job.revision },
      },
      false,
    );
  }
  async function review(value: DepthPreview) {
    if (
      disabled ||
      !job ||
      !depthActions(job).review ||
      value.revision !== job.revision ||
      value.sha256 !== job.sha256
    )
      return;
    await send(
      {
        ...scope,
        version: 1,
        key: crypto.randomUUID(),
        action: "review",
        jobId: job.id,
        body: {
          project_id: scope.projectId,
          revision: value.revision,
          sha256: value.sha256,
          local_review_confirmed: true,
        },
      },
      false,
    );
  }
  async function reloadLatest() {
    if (inFlight.current || recovery.intent) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    try {
      await Promise.all([
        freshScope(),
        selected ? detail.refetch({ throwOnError: true }) : Promise.resolve(),
        pageIndex > 0
          ? page.refetch({ throwOnError: true })
          : Promise.resolve(),
      ]);
      setConflict(false);
      setNotice("已读取最新状态，请重新确认此次操作。");
    } catch (failure) {
      setError(failure);
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }
  async function download() {
    if (disabled || !job) return;
    setPending(true);
    setError(null);
    let url: string | undefined;
    try {
      await freshScope();
      const current = await getDepth(scope.projectId, job.id);
      if (current.revision !== job.revision)
        throw new ApiError(409, "media_depth_conflict");
      const blob = await downloadDepth(current);
      url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `depth-${job.id}.mp4`;
      link.click();
    } catch (failure) {
      setError(failure);
    } finally {
      if (url) URL.revokeObjectURL(url);
      setPending(false);
    }
  }
  const actions = job ? depthActions(job) : undefined;
  return (
    <section className="min-w-0 space-y-5" aria-label="视频深度任务">
      <p className="text-sm text-muted-foreground">
        使用固定 Small 模型生成相对深度。原视频保持不变；最长 15.1 秒、500
        MiB，审核后输出 1920 × 1080 MP4。后台运行条件未就绪时会明确失败。
      </p>
      <Button
        disabled={disabled || !onPrepareSource}
        onClick={() => void create()}
      >
        创建视频深度任务
      </Button>
      {notice ? <p role="status">{notice}</p> : null}
      {scopeError ? (
        <DepthFailure title="当前身份重新读取失败" error={scopeError} />
      ) : null}
      {recovery.error ? (
        <DepthFailure
          title="深度请求恢复受阻"
          error={recovery.error}
          retry={() => {
            try {
              setRecovery({
                intent: loadDepthIntent(window.sessionStorage, scope),
              });
            } catch (failure) {
              setRecovery({ intent: recovery.intent, error: failure });
            }
          }}
        />
      ) : null}
      {error ? (
        <DepthFailure
          title={recovery.intent ? "原请求效果尚未确认" : "深度操作未完成"}
          error={error}
        />
      ) : null}
      {recovery.intent ? (
        <Alert>
          <AlertTitle>原请求效果尚未确认</AlertTitle>
          <AlertDescription>
            保留原请求键和修订 {recovery.intent.body.revision}
            ，请人工核验。核验完成前不能提交新操作或关闭任务。
          </AlertDescription>
          <Button
            disabled={pending || Boolean(recovery.error)}
            onClick={() => void send(recovery.intent!, true)}
          >
            核验原请求
          </Button>
        </Alert>
      ) : null}
      {conflict && !recovery.intent ? (
        <Button disabled={pending} onClick={() => void reloadLatest()}>
          读取最新状态
        </Button>
      ) : null}
      <div className="space-y-3">
        <h3 className="font-medium">此视频的深度任务</h3>
        {page.error ? (
          <DepthFailure
            title="深度任务列表读取失败"
            error={page.error}
            retry={() => void page.refetch()}
          />
        ) : null}
        {!listed ? (
          <p role="status">正在读取任务列表…</p>
        ) : listed.items.length === 0 ? (
          <p className="text-sm text-muted-foreground">尚无深度任务。</p>
        ) : (
          <ul className="space-y-2">
            {listed.items.map((item) => (
              <li key={item.id}>
                <Button
                  variant={selected === item.id ? "secondary" : "ghost"}
                  className="h-auto w-full justify-between gap-3 text-left whitespace-normal"
                  disabled={disabled}
                  onClick={() => {
                    setSelected(item.id);
                    onJobSelected?.(item.id);
                  }}
                >
                  <span className="min-w-0 break-all">{item.id}</span>
                  <span className="shrink-0">
                    {depthStatusLabels[item.status]}
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            variant="ghost"
            aria-label="上一页深度任务"
            disabled={disabled || pageIndex === 0}
            onClick={() => setPageIndex((i) => i - 1)}
          >
            上一页
          </Button>
          <Button
            variant="ghost"
            aria-label="下一页深度任务"
            disabled={disabled || !listed?.next_cursor || page.isFetching}
            onClick={() => {
              if (!listed?.next_cursor) return;
              setCursors((current) => [
                ...current.slice(0, pageIndex + 1),
                listed.next_cursor!,
              ]);
              setPageIndex((i) => i + 1);
            }}
          >
            下一页
          </Button>
        </div>
      </div>
      {detail.error ? (
        <DepthFailure
          title="深度任务详情读取失败"
          error={detail.error}
          retry={() => void detail.refetch()}
        />
      ) : null}
      {selected && detail.isPending ? (
        <p role="status">正在读取任务详情…</p>
      ) : null}
      {job ? (
        <div className="min-w-0 space-y-4 rounded-xl bg-muted/40 p-4">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">{depthStatusLabels[job.status]}</Badge>
            <span>{depthStageLabels[job.stage]}</span>
          </div>
          <dl className="grid min-w-0 gap-2 text-sm">
            <div>
              <dt className="text-muted-foreground">任务</dt>
              <dd className="break-all">{job.id}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">冻结来源</dt>
              <dd className="break-all">
                画布 {job.source.canvas_id} · 视频节点 {job.source.node_id} ·
                修订 {job.source.revision}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">原件身份</dt>
              <dd className="break-all">
                {job.source_asset_id} · 媒体修订 {job.source_asset_revision}
                <br />
                SHA-256 {job.source_sha256}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">当前事实</dt>
              <dd>
                尝试 {job.attempt} · 修订 {job.revision}
              </dd>
            </div>
            {job.sha256 ? (
              <div>
                <dt className="text-muted-foreground">输出 SHA-256</dt>
                <dd className="break-all">{job.sha256}</dd>
              </div>
            ) : null}
          </dl>
          {job.failure_code ? (
            <p className="text-sm break-all">
              {job.failure_code === "depth_control_forbidden"
                ? "原取消请求的发起者已无操作权限。当前有权限的成员可重新请求取消，完成此任务的清理。"
                : `失败代码：${job.failure_code}`}
            </p>
          ) : null}
          {job.execution_unconfirmed ? (
            <p role="status">
              等待原尝试停止；取消只能记录意图，当前不能重试或核验结果。
            </p>
          ) : null}
          {job.reconciliation_requested ? (
            <p role="status">原结果核验已请求，等待真实任务状态更新。</p>
          ) : null}
          {job.status === "failed" && job.cancellation_requested ? (
            <p role="status">
              取消意图已保留，待原结果核验后清理，不能直接重试。
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={disabled || !actions?.cancel}
              onClick={() => control("cancel")}
            >
              请求取消
            </Button>
            <Button
              variant="outline"
              disabled={disabled || !actions?.retry}
              onClick={() => control("retry")}
            >
              重试原任务
            </Button>
            <Button
              variant="outline"
              disabled={disabled || !actions?.reconcile}
              onClick={() => control("reconcile")}
            >
              核验原结果
            </Button>
            {["review_required", "succeeded"].includes(job.status) ? (
              <Button disabled={disabled} onClick={() => setPreview(true)}>
                {job.status === "review_required"
                  ? "预览并人工审核"
                  : "预览已审核结果"}
              </Button>
            ) : null}
            {actions?.download ? (
              <Button
                variant="outline"
                disabled={disabled}
                onClick={() => void download()}
              >
                正式下载 MP4
              </Button>
            ) : null}
          </div>
          <Link
            className="text-sm underline"
            href={`/canvas?project=${scope.projectId}&canvas=${scope.canvasId}&node_id=${scope.nodeId}&depth_job=${job.id}`}
          >
            {job.status === "succeeded" ? "到来源画布采纳结果" : "打开来源画布"}
          </Link>
        </div>
      ) : null}
      {preview && job ? (
        <MediaDepthPreviewDialog
          key={`${job.id}:${job.revision}`}
          job={job}
          locked={locked}
          onClose={() => setPreview(false)}
          onReview={review}
          onVerifyOriginal={
            recovery.intent && !recovery.error
              ? () => send(recovery.intent!, true)
              : undefined
          }
          verifying={pending}
          onRecoverStorage={
            recovery.error
              ? () => {
                  try {
                    const stored = loadDepthIntent(
                      window.sessionStorage,
                      scope,
                    );
                    setRecovery({ intent: stored ?? recovery.intent });
                  } catch (failure) {
                    setRecovery({ intent: recovery.intent, error: failure });
                  }
                }
              : undefined
          }
          onAdopt={
            onAdopt
              ? async (value, asset) => {
                  await freshScope();
                  const current = await getDepth(scope.projectId, value.id);
                  newerDepth(value, current);
                  if (
                    current.revision !== value.revision ||
                    current.sha256 !== value.sha256 ||
                    !depthActions(current).adopt
                  )
                    throw new ApiError(409, "media_depth_conflict");
                  return onAdopt(current, asset);
                }
              : undefined
          }
        />
      ) : null}
    </section>
  );
}
