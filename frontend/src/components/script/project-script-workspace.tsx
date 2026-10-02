"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import dynamic from "next/dynamic";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { getProject } from "@/components/project/queries";
import { ApiError } from "@/lib/request";
import { SourceEditForm } from "./source-edit-form";
import { SourceImportDialog } from "./source-import-dialog";
import { SourceIntentRecovery } from "./source-intent-recovery";
import { SourceList } from "./source-list";
import { SourceReorderDialog } from "./source-reorder-dialog";
import {
  getSource,
  getWorkspace,
  listSources,
  readAllSources,
  requireScriptScope,
  scriptScopeKey,
  scriptWorkspaceKey,
} from "./source-queries";
import {
  scriptUUID,
  type ScriptWorkspace,
  type SourceCommand,
  type SourceDetail,
  type SourceReceipt,
  type SourceSummary,
} from "./source-model";
import type { ScriptScope } from "./source-intent";
import { useSourceWriter } from "./use-source-writer";
import { useReviewWriter } from "./use-review-writer";
import { ReviewRecovery } from "./review-recovery";
import { ReviewConflict } from "./review-conflict";
import { useFileImport } from "./use-file-import";
import { fileImportScopeKey } from "./file-import-queries";
import type { DocumentAsset } from "./document-media";

const subscribeOrigin = () => () => {};
const ScriptEpisodePanel = dynamic(
  () =>
    import("./script-episode-panel").then(
      (module) => module.ScriptEpisodePanel,
    ),
  { ssr: false, loading: () => <p role="status">正在加载分集与结构编辑…</p> },
);
const ScriptHistoryDialog = dynamic(
  () =>
    import("./script-history-dialog").then(
      (module) => module.ScriptHistoryDialog,
    ),
  { ssr: false },
);
const SourceWriteDialog = dynamic(
  () =>
    import("./source-write-dialog").then((module) => module.SourceWriteDialog),
  { ssr: false },
);
const DocumentLibrary = dynamic(
  () => import("./document-library").then((module) => module.DocumentLibrary),
  { ssr: false, loading: () => <p role="status">正在加载文档原件入口…</p> },
);
const FileImportDialog = dynamic(
  () =>
    import("./file-import-dialog").then((module) => module.FileImportDialog),
  { ssr: false },
);
const FileImportCreateDialog = dynamic(
  () =>
    import("./file-import-dialog").then(
      (module) => module.FileImportCreateDialog,
    ),
  { ssr: false },
);
type WriteBase = { expected_revision: number; base_version_id?: string };
function writeBase(workspace: ScriptWorkspace): WriteBase {
  return {
    expected_revision: workspace.state.revision,
    ...(workspace.state.draft_version_id
      ? { base_version_id: workspace.state.draft_version_id }
      : {}),
  };
}
function message(error: unknown) {
  return error instanceof Error ? error.message : "读取未完成，请重试。";
}
type EditFrame = {
  mode: "create" | "update";
  source?: SourceDetail;
  base: WriteBase;
  key: string;
};
type Navigation =
  | { kind: "create" | "reload" | "reorder" | "delete" }
  | { kind: "select"; lineage: string }
  | { kind: "version"; version?: string }
  | { kind: "href"; href: string };

export function ProjectScriptWorkspace({
  projectId: inputProjectId,
}: {
  projectId: string;
}) {
  const parsedProject = scriptUUID.safeParse(inputProjectId);
  const valid = parsedProject.success;
  const projectId = parsedProject.success
    ? parsedProject.data.toLowerCase()
    : inputProjectId;
  const origin = useSyncExternalStore(
    subscribeOrigin,
    () => window.location.origin,
    () => null,
  );
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: ({ signal }) => getProject(projectId, signal),
    enabled: valid,
    staleTime: 0,
  });
  const workspace = useQuery({
    queryKey: scriptWorkspaceKey(projectId),
    queryFn: ({ signal }) => getWorkspace(projectId, signal),
    enabled: valid,
    staleTime: 0,
    gcTime: 0,
  });
  if (!valid) return <p role="alert">项目 UUID 无效。</p>;
  return (
    <main className="mx-auto w-full max-w-7xl space-y-5 p-4 sm:p-6">
      <h1 className="text-xl font-semibold">
        {project.data ? `${project.data.name} · 剧本` : "项目剧本"}
      </h1>
      {project.isError && (
        <div>
          <p role="alert">{message(project.error)}</p>
          <Button variant="outline" onClick={() => void project.refetch()}>
            重试读取项目
          </Button>
        </div>
      )}
      {workspace.isError && (
        <div>
          <p role="alert">{message(workspace.error)}</p>
          <Button variant="outline" onClick={() => void workspace.refetch()}>
            重试读取剧本工作区
          </Button>
        </div>
      )}
      {!project.data ||
      project.isError ||
      !workspace.data ||
      !workspace.isFetchedAfterMount ||
      !workspace.isSuccess ||
      !origin ? (
        !project.isError &&
        !workspace.isError && <p role="status">正在读取项目与剧本工作区…</p>
      ) : (
        <ScopedScriptWorkspace
          key={`${origin}:${workspace.data.current_actor_id}:${workspace.data.current_org_id}:${projectId}`}
          scope={{
            origin,
            actorId: workspace.data.current_actor_id,
            orgId: workspace.data.current_org_id,
            projectId,
          }}
          head={workspace.data}
          readOnly={project.data.status !== "active" || project.data.is_delete}
          refreshHead={async () => {
            const result = await workspace.refetch({ throwOnError: true });
            if (!result.data) throw new ApiError(503, "context_unavailable");
            return result.data;
          }}
        />
      )}
    </main>
  );
}

function ScopedScriptWorkspace({
  scope,
  head,
  readOnly,
  refreshHead,
}: {
  scope: ScriptScope;
  head: ScriptWorkspace;
  readOnly: boolean;
  refreshHead: () => Promise<ScriptWorkspace>;
}) {
  const pathname = usePathname();
  const params = useSearchParams();
  const router = useRouter();
  const client = useQueryClient();
  const versionParam = params.get("script_version");
  const sourceParam = params.get("script_source");
  const parsedVersion = scriptUUID.safeParse(versionParam);
  const parsedSource = scriptUUID.safeParse(sourceParam);
  const requestedVersion = parsedVersion.success
    ? parsedVersion.data.toLowerCase()
    : undefined;
  const requestedSource = parsedSource.success
    ? parsedSource.data.toLowerCase()
    : undefined;
  const validURL =
    (versionParam === null || parsedVersion.success) &&
    (sourceParam === null || parsedSource.success);
  const historical = versionParam !== null;
  const viewVersion = requestedVersion ?? head.state.draft_version_id;
  const [frame, setFrame] = useState<EditFrame | null>(null);
  const [dirty, setDirty] = useState(false);
  const [documentBlocked, setDocumentBlocked] = useState(false);
  const [writeRecords, setWriteRecords] = useState(false);
  const [history, setHistory] = useState(false);
  const [episodesOpen, setEpisodesOpen] = useState(false);
  const [episodeBlocked, setEpisodeBlocked] = useState(false);
  const [reviewSequence, setReviewSequence] = useState(0);
  const [modalDirty, setModalDirty] = useState(false);
  const [navigation, setNavigation] = useState<Navigation | null>(null);
  const [importBase, setImportBase] = useState<WriteBase | null>(null);
  const [importsOpen, setImportsOpen] = useState(
    () => scriptUUID.safeParse(params.get("script_import")).success,
  );
  const [importJobId, setImportJobId] = useState<string>();
  const [fileFrame, setFileFrame] = useState<{
    assets: DocumentAsset[];
    base: WriteBase;
  } | null>(null);
  const [importAdmission, setImportAdmission] = useState(false);
  const [reorder, setReorder] = useState<{
    items: SourceSummary[];
    base: WriteBase;
  } | null>(null);
  const [deleting, setDeleting] = useState<{
    source: SourceDetail;
    base: WriteBase;
  } | null>(null);
  const [actionError, setActionError] = useState<string | undefined>();
  const [reading, setReading] = useState(false);
  const [confirmation, setConfirmation] = useState<SourceReceipt | null>(null);
  const [latest, setLatest] = useState<{
    workspace: ScriptWorkspace;
    source?: SourceDetail;
    items?: SourceSummary[];
    missing?: boolean;
  } | null>(null);
  const writer = useSourceWriter(scope, (receipt) => {
    setConfirmation(receipt);
    setFrame(null);
    setDirty(false);
    setModalDirty(false);
    setImportBase(null);
    setReorder(null);
    setDeleting(null);
    setLatest(null);
  });
  const reviewer = useReviewWriter(scope, async () => {
    setEpisodeBlocked(false);
    setReviewSequence((sequence) => sequence + 1);
    await Promise.all([
      client.invalidateQueries({ queryKey: ["project", scope.projectId] }),
      client.invalidateQueries({ queryKey: scriptScopeKey(scope) }),
    ]);
    await refreshHead();
  });
  const fileImporter = useFileImport(scope, async (receipt) => {
    setFileFrame(null);
    setModalDirty(false);
    setImportsOpen(true);
    setImportJobId(receipt.id);
    setImportAdmission(true);
    const next = new URLSearchParams(params.toString());
    next.set("script_import", receipt.id);
    router.replace(`${pathname}?${next}`, { scroll: false });
    await Promise.all([
      client.invalidateQueries({ queryKey: fileImportScopeKey(scope) }),
      client.invalidateQueries({ queryKey: scriptScopeKey(scope) }),
      client.invalidateQueries({
        queryKey: scriptWorkspaceKey(scope.projectId),
      }),
    ]);
  });
  const sources = useInfiniteQuery({
    queryKey: [...scriptScopeKey(scope), "sources", viewVersion ?? "empty"],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      listSources(scope, viewVersion, pageParam, signal),
    getNextPageParam: (page) => page.next_position,
    enabled: validURL,
    staleTime: 0,
  });
  const items = sources.data?.pages.flatMap((page) => page.items) ?? [];
  const duplicatePages =
    new Set(items.map((item) => item.id)).size !== items.length ||
    new Set(items.map((item) => item.source_lineage_id)).size !== items.length;
  const firstVersion = sources.data?.pages[0].version_id;
  const consistent =
    historical || !sources.data || firstVersion === head.state.draft_version_id;
  const selected =
    frame?.source?.source_lineage_id ??
    requestedSource ??
    items[0]?.source_lineage_id;
  const detailVersion =
    frame?.base.base_version_id ?? viewVersion ?? firstVersion;
  const detail = useQuery({
    queryKey: [
      ...scriptScopeKey(scope),
      "source",
      detailVersion ?? "empty",
      selected,
    ],
    queryFn: ({ signal }) => getSource(scope, selected!, detailVersion, signal),
    enabled: Boolean(validURL && selected && frame?.mode !== "create"),
    staleTime: 0,
  });
  const shown = frame ? frame.source : detail.data;
  const base = frame?.base ?? writeBase(head);
  const formKey =
    frame?.key ?? `${shown?.id ?? "empty"}:${detailVersion ?? "empty"}`;
  const locked =
    writer.locked ||
    writer.conflicted ||
    reviewer.locked ||
    fileImporter.locked ||
    reading ||
    documentBlocked ||
    episodeBlocked;
  const cannotWrite =
    locked ||
    readOnly ||
    historical ||
    !validURL ||
    !consistent ||
    duplicatePages;
  const showFileTasks =
    !fileFrame &&
    (importsOpen ||
      Boolean(
        fileImporter.intent ||
        fileImporter.storageError ||
        fileImporter.rejected,
      ));
  const hasModal = Boolean(
    importBase || reorder || deleting || fileFrame || showFileTasks,
  );
  useEffect(() => {
    if (
      !dirty &&
      !modalDirty &&
      !writer.conflicted &&
      !episodeBlocked &&
      !reviewer.rejected
    )
      return;
    function prevent(event: BeforeUnloadEvent) {
      event.preventDefault();
      event.returnValue = "";
    }
    window.addEventListener("beforeunload", prevent);
    return () => window.removeEventListener("beforeunload", prevent);
  }, [dirty, modalDirty, writer.conflicted, episodeBlocked, reviewer.rejected]);
  useEffect(() => {
    if (
      !dirty &&
      !modalDirty &&
      !writer.locked &&
      !writer.conflicted &&
      !reviewer.locked &&
      !fileImporter.locked &&
      !episodeBlocked &&
      !documentBlocked
    )
      return;
    function intercept(event: MouseEvent) {
      const anchor =
        event.target instanceof Element
          ? event.target.closest("a[href]")
          : null;
      if (
        !(anchor instanceof HTMLAnchorElement) ||
        anchor.hasAttribute("download") ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const target = new URL(anchor.href);
      if (target.href === window.location.href) return;
      event.preventDefault();
      event.stopPropagation();
      if (
        writer.locked ||
        writer.conflicted ||
        reviewer.locked ||
        fileImporter.locked ||
        episodeBlocked ||
        documentBlocked
      )
        setActionError(
          "请先核验原保存或读取并确认最新事实，再离开当前工作区。",
        );
      else setNavigation({ kind: "href", href: target.href });
    }
    document.addEventListener("click", intercept, true);
    return () => document.removeEventListener("click", intercept, true);
  }, [
    dirty,
    modalDirty,
    writer.locked,
    writer.conflicted,
    reviewer.locked,
    fileImporter.locked,
    episodeBlocked,
    documentBlocked,
  ]);
  function locate(lineage?: string) {
    const next = new URLSearchParams(params.toString());
    if (lineage) next.set("script_source", lineage);
    else next.delete("script_source");
    router.replace(`${pathname}${next.size ? `?${next}` : ""}`, {
      scroll: false,
    });
  }
  async function navigate(target: Navigation, discard = false) {
    if (locked) {
      setActionError(
        "当前保存、上传或审核尚未结束，请先核验原键或明确关闭本地审核草稿。",
      );
      return;
    }
    if (dirty && !discard) {
      setNavigation(target);
      return;
    }
    if (discard) {
      setFrame(null);
      setDirty(false);
    }
    setNavigation(null);
    setActionError(undefined);
    if (target.kind === "select") {
      setFrame(null);
      setDirty(false);
      locate(target.lineage);
    }
    if (target.kind === "version") {
      setFrame(null);
      setDirty(false);
      const next = new URLSearchParams(params.toString());
      if (target.version) next.set("script_version", target.version);
      else next.delete("script_version");
      router.replace(`${pathname}${next.size ? `?${next}` : ""}`, {
        scroll: false,
      });
    }
    if (target.kind === "create") {
      setFrame({
        mode: "create",
        base: writeBase(head),
        key: crypto.randomUUID(),
      });
      setDirty(false);
    }
    if (target.kind === "href") window.location.assign(target.href);
    if (target.kind === "reload") {
      setFrame(null);
      setDirty(false);
      try {
        await refreshHead();
        await sources.refetch();
      } catch (error) {
        setActionError(message(error));
      }
    }
    if (target.kind === "delete" && shown)
      setDeleting({ source: shown, base: writeBase(head) });
    if (target.kind === "reorder") {
      setReading(true);
      try {
        const fresh = await refreshHead();
        requireScriptScope(fresh, scope);
        const full = await readAllSources(scope, fresh.state.draft_version_id);
        setReorder({ items: full.items, base: writeBase(fresh) });
        setModalDirty(false);
      } catch (error) {
        setActionError(message(error));
      } finally {
        setReading(false);
      }
    }
  }
  async function readLatest() {
    if (!writer.rejected) return;
    setReading(true);
    setActionError(undefined);
    try {
      const fresh = await refreshHead();
      requireScriptScope(fresh, scope);
      const command = writer.rejected.intent;
      let selectedSource: SourceDetail | undefined;
      let missing = false;
      let full: SourceSummary[] | undefined;
      if (command.action === "update" || command.action === "delete") {
        try {
          selectedSource = await getSource(
            scope,
            command.lineageId,
            fresh.state.draft_version_id,
          );
        } catch (error) {
          if (error instanceof ApiError && error.status === 404) missing = true;
          else throw error;
        }
      }
      if (command.action === "reorder")
        full = (await readAllSources(scope, fresh.state.draft_version_id))
          .items;
      setLatest({
        workspace: fresh,
        source: selectedSource,
        items: full,
        missing,
      });
    } catch (error) {
      setActionError(message(error));
    } finally {
      setReading(false);
    }
  }
  function acceptLatest(asNew = false) {
    if (!latest || !writer.rejected) return;
    const next = writeBase(latest.workspace);
    const action = writer.rejected.intent.action;
    if (action === "update" || action === "create")
      setFrame((previous) =>
        previous
          ? {
              ...previous,
              base: next,
              mode: asNew ? "create" : previous.mode,
              source: asNew
                ? previous.source
                : (latest.source ?? previous.source),
            }
          : null,
      );
    if (action === "import") setImportBase(next);
    if (action === "delete") {
      if (latest.source) setDeleting({ source: latest.source, base: next });
      else setDeleting(null);
    }
    if (action === "reorder" && latest.items)
      setReorder({ items: latest.items, base: next });
    writer.acknowledgeLatest();
    setLatest(null);
    setActionError(undefined);
  }
  const feedback = (
    <>
      <SourceIntentRecovery
        intent={writer.intent}
        busy={writer.busy}
        storageError={writer.storageError}
        error={writer.intent ? writer.error : undefined}
        onReplay={() => void writer.replay()}
        onStorageCheck={() => void writer.restoreStorage()}
        onReviewPending={() => setWriteRecords(true)}
      />
      <ReviewRecovery writer={reviewer} />
      <ReviewConflict
        key={reviewer.rejected?.intent.key}
        scope={scope}
        writer={reviewer}
        onDiscard={() => {
          reviewer.acknowledgeLatest();
          setEpisodeBlocked(false);
          setReviewSequence((sequence) => sequence + 1);
        }}
      />
      {writer.error && !writer.intent && !writer.storageError && (
        <p role="alert" className="text-sm text-destructive">
          {writer.error}
        </p>
      )}
      {writer.conflicted && (
        <section
          className="space-y-2 rounded-lg border p-3"
          aria-label="确认剧本版本冲突"
        >
          <p>
            保存被确定拒绝，当前草稿保留。先读取最新事实，审阅后另明确提交新意图。
          </p>
          <Button
            type="button"
            variant="outline"
            disabled={reading}
            onClick={() => void readLatest()}
          >
            {reading ? "正在读取最新事实…" : "读取最新事实并保留草稿"}
          </Button>
          {latest && (
            <>
              <p>
                当前脚本版本 {latest.workspace.state.revision}；原请求版本{" "}
                {writer.rejected?.intent.body.expected_revision}。
              </p>
              {latest.source && (
                <details>
                  <summary>查看当前服务端来源正文</summary>
                  <p className="text-sm break-all">
                    {latest.source.title} · 来源版本{" "}
                    {latest.source.source_revision} · 格式 SHA{" "}
                    {latest.source.rich_sha256}
                  </p>
                  <pre className="max-h-64 overflow-auto break-words whitespace-pre-wrap">
                    {latest.source.plain_text}
                  </pre>
                </details>
              )}
              {latest.missing ? (
                <>
                  <p>来源已从当前草稿移除，原本地草稿仍保留。</p>
                  {writer.rejected?.intent.action === "update" ? (
                    <Button onClick={() => acceptLatest(true)}>
                      将当前草稿明确另建来源
                    </Button>
                  ) : (
                    <Button onClick={() => acceptLatest()}>
                      确认当前来源已移除
                    </Button>
                  )}
                </>
              ) : writer.rejected?.intent.action === "reorder" ? (
                <p>请在下方审阅完整来源集合，再明确采用新的版本与顺序草稿。</p>
              ) : (
                <Button onClick={() => acceptLatest()}>
                  确认最新版本并继续原草稿
                </Button>
              )}
            </>
          )}
        </section>
      )}
      {actionError && (
        <p role="alert" className="text-sm text-destructive">
          {actionError}
        </p>
      )}
    </>
  );
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          disabled={cannotWrite}
          onClick={() => void navigate({ kind: "create" })}
        >
          新建来源
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={cannotWrite || dirty}
          onClick={() => {
            setImportBase(writeBase(head));
            setModalDirty(false);
          }}
        >
          批量导入章节
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={cannotWrite || !items.length}
          onClick={() => void navigate({ kind: "reorder" })}
        >
          调整完整来源顺序
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={locked}
          onClick={() => {
            void refreshHead().catch((error) => setActionError(message(error)));
          }}
        >
          读取当前事实
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => setWriteRecords(true)}
        >
          正文保存记录与恢复
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={
            dirty ||
            modalDirty ||
            episodeBlocked ||
            writer.locked ||
            reviewer.locked
          }
          onClick={() => {
            setImportsOpen(true);
            setImportAdmission(false);
          }}
        >
          文件导入任务
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => setHistory(true)}
        >
          完整版本与来源历史
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!validURL || !viewVersion || episodeBlocked}
          onClick={() => setEpisodesOpen((open) => !open)}
        >
          {episodesOpen ? "收起分集与结构" : "分集与正式结构"}
        </Button>
        {historical && (
          <Button
            type="button"
            variant="outline"
            disabled={locked}
            onClick={() => void navigate({ kind: "version" })}
          >
            返回当前草稿
          </Button>
        )}
        {dirty && (
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => void navigate({ kind: "reload" })}
          >
            放弃草稿并载入最新
          </Button>
        )}
      </div>
      <p className="text-sm text-muted-foreground">
        脚本版本 {head.state.revision}
        {historical ? " · 正在只读查看不可变历史版本" : ""}
        {readOnly ? " · 项目只读" : ""}
      </p>
      {!validURL && (
        <p role="alert">来源或历史版本 UUID 无效，请检查定位地址。</p>
      )}
      {(!consistent || duplicatePages) && (
        <p role="alert">工作区头与来源分页事实不一致，请读取当前事实后继续。</p>
      )}
      {confirmation && (
        <p role="status">
          原保存已确认：脚本版本 {confirmation.script_revision}
          。当前显示的工作区头来自重新读取。
        </p>
      )}
      {!hasModal && feedback}
      <div className="grid min-w-0 gap-6 lg:grid-cols-[300px_minmax(0,1fr)]">
        <aside className="min-w-0">
          {sources.isPending ? (
            <p role="status">正在读取来源摘要…</p>
          ) : sources.isError && !sources.data ? (
            <div>
              <p role="alert">{message(sources.error)}</p>
              <Button variant="outline" onClick={() => void sources.refetch()}>
                重试读取来源
              </Button>
            </div>
          ) : (
            <SourceList
              items={duplicatePages ? [] : items}
              selected={selected}
              locked={locked}
              nextPage={Boolean(sources.hasNextPage)}
              loadingNext={sources.isFetchingNextPage}
              pageError={
                sources.isFetchNextPageError
                  ? message(sources.error)
                  : undefined
              }
              onSelect={(lineage) => void navigate({ kind: "select", lineage })}
              onNext={() => void sources.fetchNextPage()}
            />
          )}
        </aside>
        <section className="min-w-0 space-y-4" aria-label="选中来源正文">
          {frame?.mode === "create" || shown ? (
            <>
              <h2 className="font-semibold">
                {frame?.mode === "create" ? "新建来源草稿" : shown?.title}
              </h2>
              <SourceEditForm
                key={formKey}
                source={shown}
                base={base}
                locked={cannotWrite}
                readOnly={readOnly || historical}
                submitLabel={frame?.mode === "create" ? "创建来源" : undefined}
                onDirty={(value) => {
                  setDirty(value);
                  if (value && !frame && shown)
                    setFrame({
                      mode: "update",
                      source: shown,
                      base,
                      key: formKey,
                    });
                }}
                onSubmit={async (body) => {
                  const frozen = frame ?? {
                    mode: shown ? ("update" as const) : ("create" as const),
                    source: shown,
                    base,
                    key: formKey,
                  };
                  setFrame(frozen);
                  const command: SourceCommand =
                    frozen.mode === "update" && frozen.source
                      ? {
                          action: "update",
                          lineageId: frozen.source.source_lineage_id,
                          sourceId: frozen.source.id,
                          body,
                        }
                      : { action: "create", body };
                  await writer.submit(command);
                }}
              />
              {shown && frame?.mode !== "create" && (
                <Button
                  type="button"
                  variant="destructive"
                  disabled={cannotWrite}
                  onClick={() => void navigate({ kind: "delete" })}
                >
                  移除当前来源
                </Button>
              )}
            </>
          ) : detail.isError ? (
            <div>
              <p role="alert">{message(detail.error)}</p>
              <Button variant="outline" onClick={() => void detail.refetch()}>
                重试读取选中来源
              </Button>
            </div>
          ) : selected ? (
            <p role="status">正在读取选中正文…</p>
          ) : (
            <p>此项目尚无来源。可新建来源或导入完整章节。</p>
          )}
        </section>
      </div>
      {episodesOpen && validURL && viewVersion && (
        <ScriptEpisodePanel
          key={`${viewVersion}:${reviewSequence}`}
          scope={scope}
          versionId={viewVersion}
          head={head}
          disabled={
            writer.locked ||
            writer.conflicted ||
            fileImporter.locked ||
            reading ||
            documentBlocked ||
            readOnly ||
            dirty ||
            hasModal
          }
          historical={historical}
          writer={reviewer}
          onBlockedChange={setEpisodeBlocked}
        />
      )}
      <DocumentLibrary
        scope={scope}
        disabled={
          writer.locked ||
          writer.conflicted ||
          reviewer.locked ||
          fileImporter.locked ||
          hasModal ||
          episodeBlocked ||
          reading ||
          readOnly ||
          historical ||
          dirty
        }
        onBlockedChange={setDocumentBlocked}
        onImport={(assets) => {
          if (cannotWrite || dirty || hasModal || !assets.length) return;
          setFileFrame({ assets: [...assets], base: writeBase(head) });
          setModalDirty(true);
          setImportAdmission(false);
        }}
      />
      {fileFrame && (
        <FileImportCreateDialog
          scope={scope}
          assets={fileFrame.assets}
          base={fileFrame.base}
          writer={fileImporter}
          onClose={() => {
            setFileFrame(null);
            setModalDirty(false);
          }}
          onRebase={(newBase) =>
            setFileFrame((previous) =>
              previous ? { ...previous, base: newBase } : null,
            )
          }
        />
      )}
      {showFileTasks && (
        <FileImportDialog
          key={importJobId ?? params.get("script_import") ?? "list"}
          scope={scope}
          writer={fileImporter}
          initialJobId={
            importJobId ??
            (scriptUUID.safeParse(params.get("script_import")).success
              ? params.get("script_import")!.toLowerCase()
              : undefined)
          }
          admissionConfirmed={importAdmission}
          onClose={() => setImportsOpen(false)}
          onReadCurrent={() => {
            setImportsOpen(false);
            void navigate({ kind: "reload" });
          }}
        />
      )}
      {history && (
        <ScriptHistoryDialog
          scope={scope}
          lineageId={shown?.source_lineage_id}
          onClose={() => setHistory(false)}
          onChooseVersion={(version) => {
            setHistory(false);
            void navigate({ kind: "version", version });
          }}
        />
      )}
      {writeRecords && (
        <SourceWriteDialog
          scope={scope}
          onClose={() => setWriteRecords(false)}
        />
      )}
      {importBase && (
        <SourceImportDialog
          open
          base={importBase}
          locked={locked || readOnly}
          feedback={feedback}
          onDirty={setModalDirty}
          onClose={() => {
            setImportBase(null);
            setModalDirty(false);
          }}
          onSubmit={(body) => writer.submit({ action: "import", body })}
        />
      )}
      {reorder && (
        <SourceReorderDialog
          items={reorder.items}
          latestItems={latest?.items}
          onAcceptLatest={() => acceptLatest()}
          base={reorder.base}
          locked={locked || readOnly}
          feedback={feedback}
          onDirty={setModalDirty}
          onClose={() => {
            setReorder(null);
            setModalDirty(false);
          }}
          onSubmit={(body) => writer.submit({ action: "reorder", body })}
        />
      )}
      {deleting && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !locked) setDeleting(null);
          }}
        >
          <DialogContent
            showCloseButton={!locked}
            onEscapeKeyDown={(event) => {
              if (locked) event.preventDefault();
            }}
            onInteractOutside={(event) => {
              if (locked) event.preventDefault();
            }}
          >
            <DialogHeader>
              <DialogTitle>确认移除来源</DialogTitle>
              <DialogDescription>
                从新的草稿组合中移除“{deleting.source.title}
                ”。旧来源、正文和历史版本继续保留；服务端会原子验证下游影响。
              </DialogDescription>
            </DialogHeader>
            {feedback}
            <DialogFooter>
              <Button
                variant="outline"
                disabled={locked}
                onClick={() => setDeleting(null)}
              >
                保留此来源
              </Button>
              <Button
                variant="destructive"
                disabled={locked || readOnly}
                onClick={() =>
                  void writer.submit({
                    action: "delete",
                    lineageId: deleting.source.source_lineage_id,
                    sourceId: deleting.source.id,
                    body: deleting.base,
                  })
                }
              >
                确认移除来源
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
      {navigation && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setNavigation(null);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>保留当前来源草稿</DialogTitle>
              <DialogDescription>
                当前标题、状态或正文修改尚未保存。继续编辑可保留全部原稿；明确放弃后才能切换。
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setNavigation(null)}>
                继续编辑当前草稿
              </Button>
              <Button
                variant="destructive"
                onClick={() => void navigate(navigation, true)}
              >
                明确放弃草稿并继续
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
