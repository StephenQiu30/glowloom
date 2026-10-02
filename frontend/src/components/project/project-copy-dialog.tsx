"use client";

import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { ApiError } from "@/lib/request";
import { PROJECTS_KEY, getProject, type ProjectDetail } from "./queries";
import {
  COPY_KEY,
  copyActions,
  copyUUID,
  getCopy,
  listCopies,
  newerCopy,
  runCopyIntent,
  type CopyJob,
} from "./copy-queries";
import {
  clearCopyIntent,
  copyName,
  loadCopyIntent,
  sameCopyScope,
  saveCopyIntent,
  unknownCopyWrite,
  type CopyIntent,
  type CopyScope,
} from "./copy-intent";

type Props = {
  sourceId: string;
  jobId?: string;
  onClose: () => void;
  onJobSelected?: (id: string) => void;
};
const statusLabels: Record<CopyJob["status"], string> = {
  queued: "等待复制",
  running: "正在复制",
  failed: "复制失败",
  cancel_requested: "取消请求已记录",
  cancelled: "副本清理完成，已取消",
  succeeded: "复制完成",
};
const stageLabels: Record<CopyJob["stage"], string> = {
  media: "复制媒体与衍生物",
  bible: "复制角色与设定历史",
  script: "复制剧本与全部历史",
  canvases: "重建全部画布",
  finalizing: "核验并发布副本",
  cleanup: "核验并清理未发布副本",
  complete: "已完成",
};
const scriptCountLabels: Record<
  keyof NonNullable<CopyJob["script"]>["counts"],
  string
> = {
  sources: "来源快照",
  versions: "剧本版本",
  version_sources: "版本来源",
  project_states: "当前剧本",
  version_heads: "版本状态",
  split_sets: "分集方案",
  split_confirmations: "分集确认",
  episodes: "全部分集",
  structures: "结构历史",
  scenes: "场景",
  dialogue_lines: "对白",
  action_lines: "动作",
  objects: "正文与原件",
};
function ScriptProgress({
  script,
}: {
  script: NonNullable<CopyJob["script"]>;
}) {
  return (
    <section aria-label="剧本与历史复制进度" className="flex flex-col gap-2">
      <h3 className="text-sm font-medium">剧本与全部历史</h3>
      <dl className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-3">
        {(
          Object.keys(scriptCountLabels) as (keyof typeof scriptCountLabels)[]
        ).map((key) => (
          <div key={key} className="min-w-0">
            <dt className="text-muted-foreground">{scriptCountLabels[key]}</dt>
            <dd>
              {script.completed_counts?.[key] ?? 0} / {script.counts[key]}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
const bibleCountLabels: Record<
  keyof NonNullable<CopyJob["bible"]>["counts"],
  string
> = {
  characters: "角色",
  character_versions: "角色版本",
  character_confirmations: "角色确认",
  locations: "地点",
  location_versions: "地点版本",
  location_confirmations: "地点确认",
  props: "道具",
  prop_versions: "道具版本",
  prop_confirmations: "道具确认",
  looks: "造型",
  look_versions: "造型版本",
  references: "参考版本",
  voices: "声音版本",
  redirects: "角色合并",
  splits: "角色拆分",
};
function BibleProgress({ bible }: { bible: NonNullable<CopyJob["bible"]> }) {
  return (
    <section
      aria-label="角色与设定历史复制进度"
      className="flex flex-col gap-2"
    >
      <h3 className="text-sm font-medium">角色与设定历史</h3>
      <dl className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-3">
        {(
          Object.keys(bibleCountLabels) as (keyof typeof bibleCountLabels)[]
        ).map((key) => (
          <div key={key} className="min-w-0">
            <dt className="text-muted-foreground">{bibleCountLabels[key]}</dt>
            <dd>
              {bible.completed_counts?.[key] ?? 0} / {bible.counts[key]}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
const formSchema = z.object({ target_name: copyName });
const subscribeOrigin = () => () => {};
const browserOrigin = () => window.location.origin;
const serverOrigin = () => "";
function failureMessage(error: unknown) {
  if (error instanceof Error) return error.message;
  return "读取未完成，请重试。";
}
function CopyFailure({
  title,
  error,
  retry,
}: {
  title: string;
  error: unknown;
  retry?: () => void;
}) {
  return (
    <Alert variant="destructive" className="border-0 bg-muted/40">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        {failureMessage(error)}
        {error instanceof ApiError && error.requestId ? (
          <p>请求编号：{error.requestId}</p>
        ) : null}
      </AlertDescription>
      {retry ? (
        <Button type="button" variant="ghost" onClick={retry}>
          重试读取
        </Button>
      ) : null}
    </Alert>
  );
}

export function ProjectCopyDialog({
  sourceId,
  jobId,
  onClose,
  onJobSelected,
}: Props) {
  const origin = useSyncExternalStore(
    subscribeOrigin,
    browserOrigin,
    serverOrigin,
  );
  const sessionId = useId();
  const [locked, setLocked] = useState(false);
  const valid =
    copyUUID.safeParse(sourceId).success &&
    (!jobId || copyUUID.safeParse(jobId).success);
  // Scope and source facts have no dependency on each other.
  const first = useQuery({
    queryKey: [...COPY_KEY, sourceId, "scope", sessionId],
    queryFn: ({ signal }) => listCopies(sourceId, undefined, signal),
    enabled: valid,
    retry: false,
    staleTime: 0,
    refetchOnMount: "always",
    refetchOnWindowFocus: false,
    refetchInterval: 5000,
  });
  const sourceQueryKey = [
    ...PROJECTS_KEY,
    "copy-source",
    sourceId,
    sessionId,
  ] as const;
  const source = useQuery({
    queryKey: sourceQueryKey,
    queryFn: ({ signal }) => getProject(sourceId, signal),
    enabled: valid,
    retry: false,
    staleTime: 0,
    refetchOnMount: "always",
    refetchOnWindowFocus: false,
  });
  const actorId = first.data?.current_actor_id;
  const orgId = first.data?.current_org_id;
  const scope = useMemo(
    () => (actorId && orgId ? { origin, actorId, orgId, sourceId } : undefined),
    [origin, actorId, orgId, sourceId],
  );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !locked) onClose();
      }}
    >
      <DialogContent
        className="max-h-[90dvh] min-w-0 overflow-y-auto ring-0 sm:max-w-2xl"
        showCloseButton={false}
        onEscapeKeyDown={(event) => {
          if (locked) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (locked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>复制完整项目</DialogTitle>
          <DialogDescription>
            冻结当前项目内容，复制全部画布、剧本历史、素材原件与衍生物。副本预算为
            ¥0，完成后才能打开。
          </DialogDescription>
        </DialogHeader>
        {!valid ? (
          <CopyFailure
            title="复制链接无效"
            error={new Error("来源和任务必须是有效的项目 UUID。")}
          />
        ) : first.error && !first.data ? (
          <CopyFailure
            title="复制任务未能读取"
            error={first.error}
            retry={() => void first.refetch()}
          />
        ) : first.isPending || source.isPending || !scope?.origin ? (
          <p role="status">正在读取来源与复制任务…</p>
        ) : (
          <CopySession
            key={`${scope.actorId}:${scope.orgId}:${scope.sourceId}`}
            scope={scope}
            firstPage={first.data!}
            source={source.data}
            sourceQueryKey={sourceQueryKey}
            sourceError={source.error}
            initialJobId={jobId}
            onClose={onClose}
            onJobSelected={onJobSelected}
            onLockChange={setLocked}
            onReloadSource={() => void source.refetch()}
          />
        )}
        {!scope || !valid ? (
          <DialogFooter className="border-0 bg-transparent">
            <Button
              type="button"
              variant="ghost"
              disabled={locked}
              onClick={onClose}
            >
              关闭复制任务
            </Button>
          </DialogFooter>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

type SessionProps = {
  scope: CopyScope;
  firstPage: Awaited<ReturnType<typeof listCopies>>;
  source?: ProjectDetail;
  sourceQueryKey: readonly string[];
  sourceError: unknown;
  initialJobId?: string;
  onClose: () => void;
  onJobSelected?: (id: string) => void;
  onLockChange: (locked: boolean) => void;
  onReloadSource: () => void;
};
function CopySession({
  scope,
  firstPage,
  source,
  sourceQueryKey,
  sourceError,
  initialJobId,
  onClose,
  onJobSelected,
  onLockChange,
  onReloadSource,
}: SessionProps) {
  const cache = useQueryClient();
  const nameId = useId();
  const inFlight = useRef(false);
  const seenSuccess = useRef<string>("");
  const [pending, setPending] = useState(false);
  // This session mounts only after the browser origin and a fresh scope GET exist.
  const [recovery, setRecovery] = useState<{
    loaded: boolean;
    intent: CopyIntent | null;
    error?: unknown;
  }>(() => {
    try {
      return {
        loaded: true,
        intent: loadCopyIntent(window.sessionStorage, scope),
      };
    } catch (failure) {
      return { loaded: true, intent: null, error: failure };
    }
  });
  const [error, setError] = useState<unknown>(null);
  const [conflict, setConflict] = useState(false);
  const [notice, setNotice] = useState("");
  const [selectedId, setSelectedId] = useState(
    recovery.intent && recovery.intent.action !== "create"
      ? recovery.intent.jobId
      : initialJobId,
  );
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [pageIndex, setPageIndex] = useState(0);
  const form = useForm<{ target_name: string }>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      target_name:
        recovery.intent?.action === "create"
          ? recovery.intent.body.target_name
          : source
            ? `${Array.from(source.name).slice(0, 47).join("")} 副本`
            : "",
    },
  });
  const scopeKey = [
    ...COPY_KEY,
    scope.sourceId,
    scope.actorId,
    scope.orgId,
  ] as const;
  const page = useQuery({
    queryKey: [...scopeKey, "list", cursors[pageIndex]],
    queryFn: ({ signal }) =>
      listCopies(scope.sourceId, cursors[pageIndex], signal),
    enabled: pageIndex > 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: pageIndex > 0 ? 5000 : false,
  });
  const listed = pageIndex === 0 ? firstPage : page.data;
  const detail = useQuery({
    queryKey: [...scopeKey, "detail", selectedId],
    queryFn: ({ signal }) => getCopy(scope.sourceId, selectedId!, signal),
    enabled: Boolean(selectedId),
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
    structuralSharing: (current, incoming) =>
      newerCopy(current as CopyJob | undefined, incoming as CopyJob),
    refetchInterval: (query) =>
      query.state.data &&
      !["succeeded", "cancelled"].includes(query.state.data.status)
        ? 2000
        : false,
  });
  const job = detail.data;
  const locked = pending || Boolean(recovery.intent);
  const disabled =
    locked || !recovery.loaded || Boolean(recovery.error) || conflict;
  useEffect(() => {
    onLockChange(locked);
    return () => onLockChange(false);
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
    const click = (event: MouseEvent) => {
      if (event.target instanceof Element && event.target.closest("a[href]")) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    window.addEventListener("beforeunload", warn);
    window.addEventListener("popstate", back, true);
    document.addEventListener("click", click, true);
    return () => {
      window.removeEventListener("beforeunload", warn);
      window.removeEventListener("popstate", back, true);
      document.removeEventListener("click", click, true);
    };
  }, [locked]);
  useEffect(() => {
    if (job?.status !== "succeeded" || seenSuccess.current === job.id) return;
    seenSuccess.current = job.id;
    void Promise.all([
      cache.invalidateQueries({ queryKey: PROJECTS_KEY }),
      cache.invalidateQueries({ queryKey: ["canvas", "projects"] }),
      cache.invalidateQueries({
        queryKey: ["canvas", "list", job.target_project_id],
      }),
    ]);
  }, [job, cache]);
  function select(id: string) {
    if (!disabled) {
      setSelectedId(id);
      onJobSelected?.(id);
    }
  }
  async function send(intent: CopyIntent, replay: boolean) {
    if (inFlight.current || !recovery.loaded || recovery.error) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    setNotice("");
    let posted = false;
    let persisted = false;
    try {
      const [freshScope, facts] = await Promise.all([
        listCopies(scope.sourceId),
        intent.action === "create"
          ? replay
            ? Promise.resolve(undefined)
            : getProject(scope.sourceId)
          : getCopy(scope.sourceId, intent.jobId),
      ]);
      if (
        !sameCopyScope(scope, {
          ...scope,
          actorId: freshScope.current_actor_id,
          orgId: freshScope.current_org_id,
        })
      )
        throw new Error(
          "当前工作区身份已变化。原请求已保留，不会在其他身份下重放。请重新读取复制任务。",
        );
      if (!replay && facts?.revision !== intent.body.expected_revision)
        throw new ApiError(409, "revision_conflict");
      if (
        !replay &&
        intent.action !== "create" &&
        !copyActions(facts as CopyJob)[intent.action]
      )
        throw new ApiError(409, "copy_state_conflict");
      saveCopyIntent(window.sessionStorage, intent);
      persisted = true;
      setRecovery({ loaded: true, intent });
      posted = true;
      const accepted = await runCopyIntent(intent);
      clearCopyIntent(window.sessionStorage, scope);
      setRecovery({ loaded: true, intent: null });
      setConflict(false);
      setNotice("操作已受理，以最新任务状态确认结果。");
      // Never install a possibly stale idempotent admission receipt as current facts.
      setSelectedId(accepted.id);
      onJobSelected?.(accepted.id);
      await cache.invalidateQueries({
        queryKey: [...COPY_KEY, scope.sourceId],
      });
    } catch (failure) {
      setError(failure);
      if (posted && !unknownCopyWrite(failure)) {
        try {
          clearCopyIntent(window.sessionStorage, scope);
          setRecovery({ loaded: true, intent: null });
        } catch (storageError) {
          setError(storageError);
        }
      } else if (persisted) setRecovery({ loaded: true, intent });
      if (failure instanceof ApiError && failure.status === 409)
        setConflict(true);
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }
  function create(values: { target_name: string }) {
    if (disabled || !source) return;
    void send(
      {
        ...scope,
        version: 1,
        key: crypto.randomUUID(),
        action: "create",
        body: {
          expected_revision: source.revision,
          target_name: values.target_name,
        },
      },
      false,
    );
  }
  function control(action: "cancel" | "retry" | "reconcile") {
    if (disabled || !job || !copyActions(job)[action]) return;
    void send(
      {
        ...scope,
        version: 1,
        key: crypto.randomUUID(),
        action,
        jobId: job.id,
        body: { expected_revision: job.revision },
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
      const [freshScope] = await Promise.all([
        listCopies(scope.sourceId),
        cache.fetchQuery({
          queryKey: sourceQueryKey,
          queryFn: () => getProject(scope.sourceId),
          staleTime: 0,
        }),
        selectedId ? detail.refetch({ throwOnError: true }) : Promise.resolve(),
      ]);
      if (
        !sameCopyScope(scope, {
          ...scope,
          actorId: freshScope.current_actor_id,
          orgId: freshScope.current_org_id,
        })
      )
        throw new Error("当前工作区身份已变化，请重新打开复制任务。");
      setConflict(false);
      setNotice("已读取最新版本，请重新确认此次操作。");
    } catch (failure) {
      setError(failure);
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }
  return (
    <>
      {sourceError ? (
        <CopyFailure
          title="来源项目未能读取"
          error={sourceError}
          retry={onReloadSource}
        />
      ) : (
        <form
          noValidate
          onSubmit={(event) => void form.handleSubmit(create)(event)}
          className="flex flex-col gap-4"
        >
          <p className="text-sm text-muted-foreground">
            来源：{source?.name} · 当前修订 {source?.revision}
          </p>
          <FieldGroup>
            <Field
              data-invalid={Boolean(form.formState.errors.target_name)}
              data-disabled={disabled}
            >
              <FieldLabel htmlFor={nameId}>副本名称</FieldLabel>
              <Input
                id={nameId}
                autoComplete="off"
                disabled={disabled || !source}
                aria-invalid={Boolean(form.formState.errors.target_name)}
                aria-describedby={`${nameId}-help`}
                {...form.register("target_name")}
              />
              <FieldDescription id={`${nameId}-help`}>
                1–50 个字符。每次明确创建都会产生独立副本，来源项目保留。
              </FieldDescription>
              <FieldError errors={[form.formState.errors.target_name]} />
            </Field>
          </FieldGroup>
          <Button
            type="submit"
            disabled={disabled || !source}
            className="self-start"
          >
            {pending ? "正在核验请求…" : "创建完整副本"}
          </Button>
        </form>
      )}
      {recovery.error ? (
        <CopyFailure title="复制操作暂不可用" error={recovery.error} />
      ) : null}
      {recovery.intent ? (
        <Alert className="border-0 bg-muted/40">
          <AlertTitle>原操作结果尚未确认</AlertTitle>
          <AlertDescription>
            <p>
              已保留原请求、正文和修订。请用同一次请求人工核验，当前不能提交新操作或关闭。
            </p>
            <p className="break-all">请求编号：{recovery.intent.key}</p>
          </AlertDescription>
          <Button
            type="button"
            variant="secondary"
            disabled={pending}
            onClick={() => void send(recovery.intent!, true)}
          >
            使用原请求核验结果
          </Button>
        </Alert>
      ) : null}
      {error ? <CopyFailure title="复制操作未完成" error={error} /> : null}
      {conflict && !recovery.intent ? (
        <Button
          type="button"
          variant="secondary"
          disabled={pending}
          onClick={() => void reloadLatest()}
        >
          读取最新后重新确认
        </Button>
      ) : null}
      {notice ? (
        <p role="status" className="text-sm">
          {notice}
        </p>
      ) : null}
      <section
        aria-labelledby={`${nameId}-list`}
        className="flex min-w-0 flex-col gap-3"
      >
        <h2 id={`${nameId}-list`} className="font-medium">
          复制任务
        </h2>
        {pageIndex > 0 && page.error ? (
          <CopyFailure
            title="本页复制任务未能读取"
            error={page.error}
            retry={() => void page.refetch()}
          />
        ) : !listed ? (
          <p role="status">正在读取复制任务…</p>
        ) : listed.copies.length === 0 ? (
          <Empty className="border-0 py-6">
            <EmptyHeader>
              <EmptyTitle>暂无复制任务</EmptyTitle>
              <EmptyDescription>
                创建副本后，可在这里查看实际复制与核验结果。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ul className="flex flex-col gap-2">
            {listed.copies.map((item) => (
              <li key={item.id}>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={disabled}
                  aria-current={selectedId === item.id ? "true" : undefined}
                  onClick={() => select(item.id)}
                  className="h-auto w-full flex-wrap justify-between gap-2 bg-muted/30 p-3 text-left"
                >
                  <span className="min-w-0 break-words whitespace-normal">
                    {item.target_name}
                  </span>
                  <Badge variant="secondary" className="border-0">
                    {statusLabels[item.status]}
                  </Badge>
                </Button>
              </li>
            ))}
          </ul>
        )}
        <nav
          aria-label="复制任务分页"
          className="flex flex-wrap items-center justify-end gap-2"
        >
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled || pageIndex === 0 || page.isFetching}
            onClick={() => setPageIndex((index) => index - 1)}
          >
            上一页复制任务
          </Button>
          <span className="text-sm tabular-nums" aria-live="polite">
            第 {pageIndex + 1} 页
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled || !listed?.next_cursor || page.isFetching}
            onClick={() => {
              const cursor = listed?.next_cursor;
              if (cursor) {
                setCursors((current) => [
                  ...current.slice(0, pageIndex + 1),
                  cursor,
                ]);
                setPageIndex((index) => index + 1);
              }
            }}
          >
            下一页复制任务
          </Button>
        </nav>
      </section>
      {selectedId ? (
        <section
          className="flex min-w-0 flex-col gap-3"
          aria-labelledby={`${nameId}-detail`}
        >
          <h2 id={`${nameId}-detail`} className="font-medium">
            复制任务详情
          </h2>
          {detail.error ? (
            <CopyFailure
              title="复制详情未能读取"
              error={detail.error}
              retry={() => void detail.refetch()}
            />
          ) : detail.isPending ? (
            <p role="status">正在读取复制详情…</p>
          ) : job ? (
            <>
              <p className="text-sm">
                {statusLabels[job.status]} · {stageLabels[job.stage]}
              </p>
              <p className="text-sm text-muted-foreground">
                冻结来源修订 {job.source_revision} · 任务修订 {job.revision} ·
                第 {job.attempt} 次尝试
              </p>
              <p className="text-sm tabular-nums">
                画布 {job.completed_documents} / {job.documents} · 素材{" "}
                {job.completed_assets} / {job.assets} · 衍生物{" "}
                {job.completed_renditions} / {job.renditions}
              </p>
              {job.bible ? <BibleProgress bible={job.bible} /> : null}
              {job.script ? <ScriptProgress script={job.script} /> : null}
              <p className="text-xs break-all text-muted-foreground">
                任务：{job.id}
                <br />
                来源：{job.source_project_id}
                <br />
                目标：{job.target_project_id}
              </p>
              {job.failure_code ? (
                <p className="text-sm">失败原因：{job.failure_code}</p>
              ) : null}
              {job.execution_unconfirmed ? (
                <p role="status" className="text-sm">
                  原尝试尚未确认退出。等待执行结束后才能核验旧结果；请求取消只记录意图。
                </p>
              ) : job.cancellation_requested && job.status !== "cancelled" ? (
                <p className="text-sm">
                  取消意图已记录，待核验原结果并清理副本。来源项目保留。
                </p>
              ) : job.needs_reconciliation ? (
                <p className="text-sm">
                  原尝试已停止，旧写入结果需要人工核验。核验恢复原任务与原冻结内容。
                </p>
              ) : null}
              <div className="flex flex-wrap gap-2">
                {copyActions(job).cancel ? (
                  <Button
                    type="button"
                    variant="secondary"
                    disabled={disabled}
                    onClick={() => control("cancel")}
                  >
                    请求取消复制
                  </Button>
                ) : null}
                {copyActions(job).retry ? (
                  <Button
                    type="button"
                    variant="secondary"
                    disabled={disabled}
                    onClick={() => control("retry")}
                  >
                    重试原任务
                  </Button>
                ) : null}
                {job.needs_reconciliation ? (
                  <Button
                    type="button"
                    variant="secondary"
                    disabled={disabled || !copyActions(job).reconcile}
                    onClick={() => control("reconcile")}
                  >
                    核验旧结果
                  </Button>
                ) : null}
                {job.status === "succeeded" ? (
                  <Button asChild disabled={disabled}>
                    <Link
                      href={`/projects/${job.target_project_id}/canvas`}
                      aria-disabled={disabled || undefined}
                      tabIndex={disabled ? -1 : undefined}
                    >
                      打开副本画布
                    </Link>
                  </Button>
                ) : null}
                {job.status === "succeeded" && job.script ? (
                  <Button asChild disabled={disabled} variant="secondary">
                    <Link
                      href={`/projects/${job.target_project_id}/script`}
                      aria-disabled={disabled || undefined}
                      tabIndex={disabled ? -1 : undefined}
                    >
                      打开副本剧本
                    </Link>
                  </Button>
                ) : null}
                {job.status === "succeeded" && job.bible?.completed_counts ? (
                  <Button asChild disabled={disabled} variant="secondary">
                    <Link
                      href={`/projects/${job.target_project_id}/bible`}
                      aria-disabled={disabled || undefined}
                      tabIndex={disabled ? -1 : undefined}
                    >
                      打开副本设定集
                    </Link>
                  </Button>
                ) : null}
                <Button
                  type="button"
                  variant="ghost"
                  disabled={pending || detail.isFetching}
                  onClick={() => void detail.refetch()}
                >
                  刷新任务状态
                </Button>
              </div>
            </>
          ) : null}
        </section>
      ) : null}
      <DialogFooter className="border-0 bg-transparent">
        <Button
          type="button"
          variant="ghost"
          disabled={locked}
          onClick={onClose}
        >
          关闭复制任务
        </Button>
      </DialogFooter>
    </>
  );
}
