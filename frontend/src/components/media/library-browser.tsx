"use client";
import dynamic from "next/dynamic";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationEllipsis,
} from "@/components/ui/pagination";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { getProject, listProjects } from "@/components/project/queries";
import {
  folderAncestry,
  type LibraryFilter,
  type LibraryIdentity,
  type LibraryPage,
  type LibraryDetail,
} from "./library-model";
import {
  freshLibrary,
  getLibraryDetail,
  libraryKey,
  listLibrary,
} from "./library-queries";
import { LibrarySelect } from "./library-select";
import { LibraryFilters } from "./library-filters";
import { LibraryItems } from "./library-items";
import { LibraryFolderCard } from "./library-folder-card";
import { useLibraryWriter } from "./use-library-writer";
import type { LibraryEditFrame } from "./library-command-dialog";
import { loadLibraryUploads } from "./library-upload-intent";
import { loadTransferIntent } from "@/components/library/transfer-intent";
import type { TransferSelection } from "@/components/library/transfer-form";
import { LibraryStorageMeter } from "./library-storage-meter";
import { usePurgeWriter } from "./library-purge-writer";
import type { PurgeFrame } from "./library-purge-dialog";
import type { PurgeJob } from "./library-purge-model";
const PurgeDialog = dynamic(
  () =>
    import("./library-purge-dialog").then(
      (module) => module.LibraryPurgeDialog,
    ),
  { ssr: false },
);
const TransferDialog = dynamic(
  () =>
    import("@/components/library/transfer-dialog").then(
      (module) => module.TransferDialog,
    ),
  { ssr: false },
);
const CommandDialog = dynamic(
  () =>
    import("./library-command-dialog").then(
      (module) => module.LibraryCommandDialog,
    ),
  { ssr: false },
);
const DetailDialog = dynamic(
  () =>
    import("./library-detail-dialog").then(
      (module) => module.LibraryDetailDialog,
    ),
  { ssr: false },
);
const UploadDialog = dynamic(
  () =>
    import("./library-upload-dialog").then(
      (module) => module.LibraryUploadDialog,
    ),
  { ssr: false },
);
export function LibraryBrowser({
  identity,
  initial,
  filter,
  selected,
  view,
  refreshContext,
}: {
  identity: LibraryIdentity;
  initial: LibraryPage;
  filter: LibraryFilter;
  selected: string | null;
  view: string;
  refreshContext: () => Promise<LibraryPage>;
}) {
  const parameters = useSearchParams(),
    pathname = usePathname(),
    router = useRouter(),
    cache = useQueryClient();
  const surface = useRef<HTMLElement>(null),
    returnFocus = useRef<HTMLElement>(null);
  const [selection, setSelection] = useState<
      { id: string; revision: number }[]
    >([]),
    [frame, setFrame] = useState<LibraryEditFrame>(),
    [uploadOpen, setUploadOpen] = useState(false),
    [uploadLocked, setUploadLocked] = useState(false),
    [transferOpen, setTransferOpen] = useState(false),
    [transferSelection, setTransferSelection] = useState<TransferSelection>(),
    [purgeFrame, setPurgeFrame] = useState<PurgeFrame>(),
    [purgeAccepted, setPurgeAccepted] = useState<PurgeJob>(),
    [reading, setReading] = useState(false),
    [notice, setNotice] = useState<string>();
  const chooser = useInfiniteQuery({
    queryKey: [...libraryKey(identity), "projects"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listProjects({ status: "active", limit: 100, cursor: pageParam }, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  const project = useQuery({
    queryKey: [...libraryKey(identity), "project"],
    queryFn: ({ signal }) =>
      getProject(
        identity.scope.kind === "project" ? identity.scope.project_id : "",
        signal,
      ),
    enabled: identity.scope.kind === "project",
    retry: false,
  });
  const page = useQuery({
    queryKey: [...libraryKey(identity), "page", filter],
    queryFn: ({ signal }) =>
      listLibrary(identity.scope, filter, signal, identity),
    retry: false,
    staleTime: 0,
  });
  const current = page.data;
  const uploadRecovery = useQuery({
    queryKey: [...libraryKey(identity), "upload-recovery"],
    queryFn: async ({ signal }) => {
      await freshLibrary(identity, signal);
      return loadLibraryUploads(sessionStorage, identity);
    },
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const hasUploadRecovery = Boolean(uploadRecovery.data?.length);
  const transferRecovery = useQuery({
    queryKey: [...libraryKey(identity), "transfer-recovery"],
    queryFn: async ({ signal }) => {
      await freshLibrary(identity, signal);
      return loadTransferIntent(sessionStorage, identity);
    },
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const hasTransferRecovery = Boolean(transferRecovery.data);
  useEffect(() => {
    if (!hasUploadRecovery) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [hasUploadRecovery]);
  const writer = useLibraryWriter(identity, async (receipt) => {
    setFrame(undefined);
    setSelection([]);
    setNotice(
      `素材库修改已确认，回执版本 ${receipt.revision}。列表将读取当前事实。`,
    );
    await Promise.all([
      cache.invalidateQueries({ queryKey: libraryKey(identity) }),
      refreshContext(),
      ...(identity.scope.kind === "project"
        ? [
            cache.invalidateQueries({
              queryKey: ["project", identity.scope.project_id],
            }),
            cache.invalidateQueries({
              queryKey: ["canvas", "media", identity.scope.project_id],
            }),
          ]
        : []),
    ]);
  });
  const purgeWriter = usePurgeWriter(identity, async (job) => {
    setPurgeAccepted(job);
    setPurgeFrame({ mode: "history" });
    setSelection([]);
    await Promise.all([
      cache.invalidateQueries({ queryKey: libraryKey(identity) }),
      refreshContext(),
      ...(identity.scope.kind === "project"
        ? [
            cache.invalidateQueries({
              queryKey: ["project", identity.scope.project_id],
            }),
            cache.invalidateQueries({
              queryKey: ["canvas", "media", identity.scope.project_id],
            }),
          ]
        : []),
    ]);
  });
  const readOnly =
    identity.scope.kind === "project" &&
    (!project.data ||
      project.isError ||
      project.data.status !== "active" ||
      project.data.is_delete);
  const blocked =
    writer.locked ||
    purgeWriter.locked ||
    Boolean(purgeFrame) ||
    !uploadRecovery.isFetchedAfterMount ||
    uploadRecovery.isError ||
    hasUploadRecovery ||
    Boolean(frame) ||
    uploadLocked ||
    uploadOpen ||
    transferOpen ||
    hasTransferRecovery ||
    !transferRecovery.isFetchedAfterMount ||
    transferRecovery.isError ||
    reading;
  const change = (
    changes: Record<string, string | undefined>,
    reset = true,
  ) => {
    if (blocked) return;
    const next = new URLSearchParams(parameters.toString());
    for (const [key, value] of Object.entries(changes)) {
      if (value === undefined || value === "") next.delete(key);
      else next.set(key, value);
    }
    if (reset) next.set("page", "1");
    router.replace(`${pathname}?${next}`, { scroll: false });
    if (
      Object.keys(changes).some(
        (key) => !["page", "asset_id", "view"].includes(key),
      )
    )
      setSelection([]);
  };
  function changeScope(value: string) {
    if (value === "personal")
      change({
        scope: "personal",
        project_id: undefined,
        asset_id: undefined,
        page_size: "40",
        folder: undefined,
        favorite: undefined,
        state: undefined,
      });
    else
      change({
        scope: "project",
        project_id: value,
        asset_id: undefined,
        page_size: "20",
        folder: undefined,
        favorite: undefined,
        state: undefined,
      });
  }
  function toggle(id: string, revision: number) {
    setSelection((previous) =>
      previous.some((item) => item.id === id)
        ? previous.filter((item) => item.id !== id)
        : previous.length < 200
          ? [...previous, { id, revision }]
          : previous,
    );
  }
  async function detailFrame(id: string) {
    if (blocked || readOnly || !current) return;
    setReading(true);
    setNotice(undefined);
    try {
      const fresh = await freshLibrary(identity);
      const detail = await getLibraryDetail(identity, id);
      setFrame({ action: "update_item", revision: fresh.revision, detail });
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "素材详情无法读取。");
    } finally {
      setReading(false);
    }
  }
  async function copyText(id: string) {
    if (blocked) return;
    setReading(true);
    try {
      await freshLibrary(identity);
      const detail = await getLibraryDetail(identity, id);
      if (detail.kind !== "text" || typeof detail.plain_text !== "string")
        throw new Error("此条目不是文本素材。");
      await navigator.clipboard.writeText(detail.plain_text);
      setNotice("已复制完整文本正文。");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "正文复制未完成。");
    } finally {
      setReading(false);
    }
  }
  const projects = chooser.data?.pages.flatMap((page) => page.items) ?? [];
  const chosen =
    identity.scope.kind === "project" ? identity.scope.project_id : "personal";
  const projectOptions = [
    { value: "personal", label: "我的个人素材库" },
    ...projects.map((project) => ({
      value: project.id,
      label: `项目 · ${project.name}`,
    })),
    ...(chosen !== "personal" &&
    !projects.some((project) => project.id === chosen)
      ? [{ value: chosen, label: `项目 · ${project.data?.name ?? chosen}` }]
      : []),
  ];
  const folders = current?.folders ?? initial.folders;
  const activeFolder =
    filter.folder !== "all" && filter.folder !== "root" ? filter.folder : null;
  const missingFolder = Boolean(
    activeFolder && !folders.some((folder) => folder.id === activeFolder),
  );
  const ancestry =
    activeFolder && !missingFolder ? folderAncestry(folders, activeFolder) : [];
  const visibleFolders =
    filter.folder === "all"
      ? folders.filter((folder) => folder.parent_id === null)
      : filter.folder === "root"
        ? []
        : folders.filter((folder) => folder.parent_id === activeFolder);
  const totalPages = current
    ? Math.max(1, Math.ceil(current.total / filter.page_size))
    : 1;
  const pageNumbers = Array.from(
    new Set([1, filter.page - 1, filter.page, filter.page + 1, totalPages]),
  )
    .filter((number) => number >= 1 && number <= Math.min(totalPages, 100000))
    .sort((a, b) => a - b);
  const batch = (
    action: "move_items" | "recycle_items" | "restore_items" | "remove_items",
  ) => {
    if (!blocked && !readOnly && current && selection.length)
      setFrame({ action, revision: current.revision, items: selection });
  };
  function restoreDialogFocus(event: Event) {
    event.preventDefault();
    // A detail-to-editor transition keeps focus in the new dialog.
    if (document.querySelector("[data-library-dialog]")) return;
    const target = returnFocus.current;
    if (target?.isConnected && !target.matches(":disabled")) target.focus();
    else surface.current?.focus();
  }
  return (
    <section
      ref={surface}
      tabIndex={-1}
      onClickCapture={(event) => {
        if (blocked || selected || !(event.target instanceof Element)) return;
        const trigger = event.target.closest<HTMLElement>("button,a[href]");
        if (trigger && surface.current?.contains(trigger))
          returnFocus.current = trigger;
      }}
      className="flex min-w-0 flex-col gap-5"
      aria-label={
        identity.scope.kind === "personal" ? "个人素材库" : "项目素材库"
      }
    >
      <LibraryStorageMeter identity={identity} />
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="outline"
          disabled={blocked}
          onClick={() => {
            setPurgeAccepted(undefined);
            setPurgeFrame({ mode: "history" });
          }}
        >
          查看永久清理记录
        </Button>
        <Button
          variant="outline"
          disabled={blocked}
          onClick={() => setTransferOpen(true)}
        >
          查看素材迁移任务
        </Button>
        <div className="w-full sm:max-w-md">
          <LibrarySelect
            label="素材库范围"
            value={chosen}
            options={projectOptions}
            disabled={blocked}
            onChange={changeScope}
          />
        </div>
        {chooser.hasNextPage && (
          <Button
            variant="outline"
            disabled={blocked || chooser.isFetchingNextPage}
            onClick={() => void chooser.fetchNextPage()}
          >
            加载更多项目
          </Button>
        )}
        {chooser.isError && (
          <Button variant="outline" onClick={() => void chooser.refetch()}>
            重试项目列表
          </Button>
        )}
        {identity.scope.kind === "project" && (
          <Button variant="outline" disabled={blocked} asChild>
            <Link
              href={`/projects/${identity.scope.project_id}/canvas`}
              aria-disabled={blocked}
              tabIndex={blocked ? -1 : undefined}
              onClick={(event) => {
                if (blocked) event.preventDefault();
              }}
            >
              打开项目画布
            </Link>
          </Button>
        )}
      </div>
      {readOnly && (
        <Alert>
          <AlertTitle>项目素材库只读</AlertTitle>
          <AlertDescription>
            {project.error?.message ??
              "当前项目未处于可编辑状态，仍可读取素材。"}
          </AlertDescription>
        </Alert>
      )}
      {uploadRecovery.isError && (
        <Alert variant="destructive">
          <AlertTitle>原上传恢复状态不可读取</AlertTitle>
          <AlertDescription>
            <p>{uploadRecovery.error.message}</p>
            <Button
              variant="outline"
              onClick={() => void uploadRecovery.refetch()}
            >
              重试读取原上传状态
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <LibraryFilters
        key={filter.search}
        scope={identity.scope}
        filter={filter}
        view={view}
        page={current}
        folders={folders}
        disabled={blocked}
        onChange={change}
      />
      <nav
        aria-label="素材目录面包屑"
        className="flex flex-wrap items-center gap-2 text-sm"
      >
        <Button
          variant="ghost"
          disabled={blocked}
          onClick={() => change({ folder: "all" })}
        >
          全部素材
        </Button>
        {ancestry.map((folder) => (
          <Button
            key={folder.id}
            variant="ghost"
            disabled={blocked}
            onClick={() => change({ folder: folder.id })}
          >
            / {folder.name}
          </Button>
        ))}
      </nav>
      {missingFolder && (
        <Alert variant="destructive">
          <AlertTitle>此目录已不存在或当前不可访问</AlertTitle>
          <AlertDescription>
            <Button
              variant="outline"
              disabled={blocked}
              onClick={() => change({ folder: "all" })}
            >
              返回全部素材
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {filter.catalog_state === "active" && !missingFolder && (
        <>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={blocked || readOnly || !current}
              onClick={() =>
                setFrame({
                  action: "create_folder",
                  revision: current!.revision,
                  folderDraft: {
                    name: "",
                    parent_id: activeFolder,
                    style: identity.scope.kind === "personal" ? "" : "paper",
                    theme: identity.scope.kind === "personal" ? "" : "pearl",
                  },
                })
              }
            >
              新建目录
            </Button>
            <Button
              variant="outline"
              disabled={blocked || readOnly || !current}
              onClick={() =>
                setFrame({
                  action: "create_text",
                  revision: current!.revision,
                  metadata: {
                    title: "",
                    plain_text: "",
                    folder_id: activeFolder,
                    category: "material",
                    tags: [],
                    source_label: "",
                    note: "",
                    favorite: false,
                  },
                })
              }
            >
              新建文本
            </Button>
            <Button
              disabled={blocked || readOnly}
              onClick={() => setUploadOpen(true)}
            >
              上传素材原件
            </Button>
          </div>
          {visibleFolders.length > 0 && (
            <div
              className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"
              aria-label="当前层级素材目录"
            >
              {visibleFolders.map((folder) => (
                <LibraryFolderCard
                  key={folder.id}
                  folder={folder}
                  count={current?.folder_counts[folder.id] ?? 0}
                  childrenCount={
                    folders.filter((child) => child.parent_id === folder.id)
                      .length
                  }
                  disabled={blocked || !current}
                  readOnly={readOnly}
                  onOpen={() => change({ folder: folder.id })}
                  onEdit={() =>
                    setFrame({
                      action: "update_folder",
                      revision: current!.revision,
                      folder,
                    })
                  }
                  onDelete={() =>
                    setFrame({
                      action: "delete_folder",
                      revision: current!.revision,
                      folder,
                    })
                  }
                />
              ))}
            </div>
          )}
        </>
      )}
      {notice && (
        <p role="status" className="text-sm">
          {notice}
        </p>
      )}
      {transferRecovery.isError && (
        <Alert variant="destructive">
          <AlertTitle>迁移原意图暂时不能恢复</AlertTitle>
          <AlertDescription>
            <p>{transferRecovery.error.message}</p>
            <Button variant="outline" onClick={() => setTransferOpen(true)}>
              打开迁移恢复
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {page.isPending ? (
        <p role="status">正在读取完整素材库分页…</p>
      ) : page.isError ? (
        <Alert variant="destructive">
          <AlertTitle>本页素材暂时不可读取</AlertTitle>
          <AlertDescription>
            <p>{page.error.message}</p>
            <Button
              variant="outline"
              disabled={blocked}
              onClick={() => void page.refetch()}
            >
              重新读取本页
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        current && (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="outline"
                disabled={blocked || !current.items.length}
                onClick={() =>
                  setSelection((old) =>
                    [
                      ...old,
                      ...current.items
                        .filter(
                          (item) =>
                            !old.some((selected) => selected.id === item.id),
                        )
                        .map((item) => ({
                          id: item.id,
                          revision: item.revision,
                        })),
                    ].slice(0, 200),
                  )
                }
              >
                选择当前页
              </Button>
              <Button
                variant="ghost"
                disabled={blocked || !selection.length}
                onClick={() => setSelection([])}
              >
                清空选择
              </Button>
              <span className="text-sm">
                已选择 {selection.length} 项 / 最多200项
              </span>
              {selection.length > 0 && filter.catalog_state === "active" && (
                <Button
                  variant="outline"
                  disabled={blocked}
                  onClick={() => {
                    setTransferSelection({
                      items: selection.map((item) => ({ ...item })),
                      sourceRevision: current.revision,
                    });
                    setTransferOpen(true);
                  }}
                >
                  {identity.scope.kind === "personal"
                    ? "加入项目素材库"
                    : "保存到个人素材库"}
                </Button>
              )}
              {selection.length > 0 && !readOnly && (
                <>
                  {filter.catalog_state === "active" ? (
                    <>
                      <Button
                        variant="outline"
                        disabled={blocked}
                        onClick={() => batch("move_items")}
                      >
                        移动所选素材
                      </Button>
                      <Button
                        variant="outline"
                        disabled={blocked}
                        onClick={() => batch("recycle_items")}
                      >
                        移入回收站
                      </Button>
                      {identity.scope.kind === "project" && (
                        <Button
                          variant="outline"
                          disabled={blocked}
                          onClick={() => batch("remove_items")}
                        >
                          移出项目素材库
                        </Button>
                      )}
                    </>
                  ) : (
                    <>
                      <Button
                        variant="outline"
                        disabled={blocked}
                        onClick={() => batch("restore_items")}
                      >
                        恢复所选素材
                      </Button>
                      <Button
                        variant="destructive"
                        disabled={blocked}
                        onClick={() => {
                          setPurgeAccepted(undefined);
                          setPurgeFrame({
                            mode: "selected",
                            items: selection.map((item) => ({ ...item })),
                          });
                        }}
                      >
                        永久删除所选素材
                      </Button>
                    </>
                  )}
                </>
              )}
              {filter.catalog_state === "trashed" && !readOnly && (
                <Button
                  variant="destructive"
                  disabled={blocked}
                  onClick={() => {
                    setPurgeAccepted(undefined);
                    setPurgeFrame({ mode: "all" });
                  }}
                >
                  清空整个回收站
                </Button>
              )}
            </div>
            {current.items.length ? (
              <LibraryItems
                identity={identity}
                items={current.items}
                view={view}
                selection={selection}
                disabled={blocked}
                readOnly={readOnly}
                onToggle={toggle}
                onView={(id) => change({ asset_id: id }, false)}
                onEdit={(id) => void detailFrame(id)}
                onCopyText={(id) => void copyText(id)}
              />
            ) : (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>
                    {filter.catalog_state === "trashed"
                      ? "回收站没有符合筛选的条目"
                      : "没有符合筛选的素材"}
                  </EmptyTitle>
                  <EmptyDescription>
                    调整范围、类型、目录或搜索条件后重新读取。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
            <Pagination aria-label="素材分页">
              <PaginationContent>
                {pageNumbers.map((number, index) => (
                  <PaginationItem key={number}>
                    <div className="flex items-center">
                      {index > 0 && number - pageNumbers[index - 1] > 1 && (
                        <PaginationEllipsis />
                      )}
                      <Button
                        variant={number === filter.page ? "outline" : "ghost"}
                        size="icon"
                        aria-current={
                          number === filter.page ? "page" : undefined
                        }
                        aria-label={`第${number}页`}
                        disabled={blocked}
                        onClick={() => change({ page: String(number) }, false)}
                      >
                        {number}
                      </Button>
                    </div>
                  </PaginationItem>
                ))}
              </PaginationContent>
            </Pagination>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="text-sm">
                第 {filter.page} / {totalPages} 页 · 共 {current.total} 项
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <LibrarySelect
                  label="每页素材数量"
                  value={String(filter.page_size)}
                  disabled={blocked}
                  options={(identity.scope.kind === "personal"
                    ? [40, 80, 120]
                    : [20, 40, 80]
                  ).map((value) => ({
                    value: String(value),
                    label: `${value}项/页`,
                  }))}
                  onChange={(value) => change({ page_size: value })}
                />
                <Button
                  variant="outline"
                  disabled={blocked || filter.page <= 1}
                  onClick={() =>
                    change({ page: String(filter.page - 1) }, false)
                  }
                >
                  上一页
                </Button>
                <Button
                  variant="outline"
                  disabled={blocked || filter.page >= totalPages}
                  onClick={() =>
                    change({ page: String(filter.page + 1) }, false)
                  }
                >
                  下一页
                </Button>
              </div>
            </div>
          </>
        )
      )}
      {(frame || writer.intent || writer.storageError) && (
        <CommandDialog
          key={
            frame
              ? `${frame.action}:${frame.revision}:${frame.detail?.id ?? frame.folder?.id ?? "new"}`
              : "recovery"
          }
          scope={identity.scope}
          folders={folders}
          frame={frame}
          writer={writer}
          onClose={() => setFrame(undefined)}
          onCloseAutoFocus={restoreDialogFocus}
        />
      )}
      {(purgeFrame ||
        purgeWriter.intent ||
        purgeWriter.plan ||
        purgeWriter.storageError) &&
        !writer.intent &&
        !writer.storageError &&
        !hasUploadRecovery &&
        !hasTransferRecovery && (
          <PurgeDialog
            key={
              purgeWriter.intent?.key ??
              `${purgeFrame?.mode ?? "recovery"}:${purgeAccepted?.id ?? "new"}`
            }
            identity={identity}
            frame={purgeFrame}
            writer={purgeWriter}
            accepted={purgeAccepted}
            readOnly={readOnly}
            onClose={() => {
              setPurgeFrame(undefined);
              setPurgeAccepted(undefined);
            }}
            onCloseAutoFocus={restoreDialogFocus}
            onObserved={async () => {
              await Promise.all([
                cache.invalidateQueries({
                  queryKey: [...libraryKey(identity), "page"],
                }),
                cache.invalidateQueries({
                  queryKey: [...libraryKey(identity), "storage-usage"],
                }),
                refreshContext(),
              ]);
            }}
          />
        )}
      {(transferOpen || hasTransferRecovery) &&
        !writer.intent &&
        !writer.storageError &&
        !purgeFrame &&
        !purgeWriter.intent &&
        !purgeWriter.plan &&
        !purgeWriter.storageError &&
        !hasUploadRecovery && (
          <TransferDialog
            identity={identity}
            selection={transferSelection}
            projects={projects}
            hasMoreProjects={Boolean(chooser.hasNextPage)}
            projectsLoading={chooser.isFetchingNextPage}
            onMoreProjects={() => {
              void chooser.fetchNextPage();
            }}
            onClose={() => {
              setTransferOpen(false);
              setTransferSelection(undefined);
              void transferRecovery.refetch();
              void Promise.all([
                cache.invalidateQueries({ queryKey: libraryKey(identity) }),
                refreshContext(),
              ]);
            }}
            onCloseAutoFocus={restoreDialogFocus}
            onChanged={async () => {
              setSelection([]);
              await Promise.all([
                cache.invalidateQueries({
                  queryKey: [
                    "media-library",
                    identity.origin,
                    identity.actorId,
                    identity.orgId,
                  ],
                }),
                cache.invalidateQueries({ queryKey: ["projects"] }),
                cache.invalidateQueries({ queryKey: ["project"] }),
                cache.invalidateQueries({ queryKey: ["canvas", "media"] }),
                refreshContext(),
              ]);
            }}
          />
        )}
      {selected &&
        !frame &&
        !writer.intent &&
        !writer.storageError &&
        !purgeFrame &&
        !purgeWriter.intent &&
        !purgeWriter.plan &&
        !purgeWriter.storageError &&
        transferRecovery.isFetchedAfterMount &&
        !transferRecovery.isError &&
        !transferOpen &&
        !hasTransferRecovery && (
          <DetailDialog
            key={selected}
            identity={identity}
            id={selected}
            readOnly={readOnly}
            onClose={() => change({ asset_id: undefined }, false)}
            onCloseAutoFocus={restoreDialogFocus}
            onEdit={(detail: LibraryDetail) =>
              setFrame({
                action: "update_item",
                revision: current?.revision ?? initial.revision,
                detail,
              })
            }
          />
        )}
      {(uploadOpen || hasUploadRecovery) &&
        !writer.intent &&
        !writer.storageError &&
        !purgeFrame &&
        !purgeWriter.intent &&
        !purgeWriter.plan &&
        !purgeWriter.storageError && (
          <UploadDialog
            identity={identity}
            onBusyChange={setUploadLocked}
            onClose={() => setUploadOpen(false)}
            onCloseAutoFocus={restoreDialogFocus}
            onUploaded={async () => {
              await Promise.all([
                cache.invalidateQueries({ queryKey: libraryKey(identity) }),
                refreshContext(),
              ]);
            }}
          />
        )}
    </section>
  );
}
