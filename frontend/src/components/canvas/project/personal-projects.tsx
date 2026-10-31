"use client";
import { pageBusinessMemory } from "@/lib/page-business-memory";

import { FilterToolbar } from "@/components/canvas/runtime/components/layout/workspace/filter-toolbar";
import { PageTabsList as TabsList } from "@/components/canvas/runtime/components/layout/workspace/page-tabs-list";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";
import { useDeferredValue, useEffect, useRef, useState } from "react";
import useSWR, { useSWRConfig } from "swr";
import useLiveQuery from "@/components/canvas/runtime/lib/use-live-query";
import Link from "next/link";
import * as canvasApi from "@/components/canvas/runtime/api/canvas-document-bff";
import { getCanvasSearchStatus } from "@/components/canvas/runtime/api/canvas-search-bff";
import { ProjectCanvasDialog } from "./project-canvases";
import { WorkspaceResourceCard } from "./workspace-resource-card";
import {
  readResourcePrefix,
  mergeResourcePage,
  type WorkspaceResource,
  type WorkspaceResourceTarget,
  resourceTarget,
  resourceKey,
} from "../lib/workspace-resources";
import { WorkspaceCardSkeleton } from "@/components/canvas/runtime/components/feedback/workspace-card-skeleton";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { Tabs, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  useAppDispatch,
  useAppSelector,
} from "@/components/canvas/runtime/store/hooks";
import { workspaceActions } from "@/components/canvas/runtime/store/app-store";
import { useProjectActions } from "./project-action-provider";
import { invalidateWorkspaceResource } from "../lib/workspace-commands";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { useRouter, useSearchParams } from "next/navigation";
import * as api from "@/components/canvas/runtime/api/personal-workspace-bff";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { PageLoading } from "@/components/canvas/runtime/components/feedback/page-state";
import { LoginPrompt } from "@/components/canvas/runtime/components/account/auth/login-prompt";
import { ProjectBreadcrumbs, ProjectMetadataActions } from "./project-metadata";

export function PersonalProjects({
  parent,
  onChanged,
}: {
  parent?: CanvasContract.ProjectWorkspaceVO;
  onChanged?: () => void;
}) {
  const router = useRouter();
  const params = useSearchParams();
  const parentId =
    parent?.projectId == null ? undefined : String(parent.projectId);
  const { account, ready, signingOut } = useAccount();
  const identity =
    ready && account && !signingOut ? `USER:${account.id}` : null;
  const [page, setPage] = useState(1);
  const [keyword, setKeyword] = useState("");
  const [sort, setSort] = useState("updated");
  const [kind, setKind] = useState(() => {
    const type = params?.get("type");
    return type === "project" || type === "canvas" ? type : "all";
  });
  const dispatch = useAppDispatch();
  const pageSize = useAppSelector((state) => state.workspace.projects.pageSize);
  const canvasActions = useProjectActions();
  const { mutate: mutatePrivateCache } = useSWRConfig();
  const intentHandled = useRef(false);
  const newButton = useRef<HTMLButtonElement>(null);
  const selectPage = useRef<HTMLButtonElement>(null);
  const [canvasMode, setCanvasMode] = useState<"create" | "attach">("create");
  const [canvasOpen, setCanvasOpen] = useState(false);
  const search = useDeferredValue(keyword);
  const selectionScope = JSON.stringify([
    identity,
    parentId,
    kind,
    keyword,
    sort,
  ]);
  const [selection, setSelection] = useState<{
    scope: string;
    items: WorkspaceResourceTarget[];
  }>({ scope: selectionScope, items: [] });
  if (selection.scope !== selectionScope) {
    setSelection({ scope: selectionScope, items: [] });
  }
  const selected = selection.scope === selectionScope ? selection.items : [];
  const changeSelection = (
    update: (items: WorkspaceResourceTarget[]) => WorkspaceResourceTarget[],
  ) =>
    setSelection((previous) => ({
      scope: selectionScope,
      items: update(previous.scope === selectionScope ? previous.items : []),
    }));
  const removeDeleted = (key: string) =>
    setSelection((previous) =>
      previous.scope === selectionScope
        ? {
            ...previous,
            items: previous.items.filter((item) => resourceKey(item) !== key),
          }
        : previous,
    );
  const [creating, setCreating] = useState(
    !parentId && params?.get("mode") === "new",
  );
  const [creationMounted, setCreationMounted] = useState(
    !parentId && params?.get("mode") === "new",
  );
  useEffect(() => {
    if (parentId) return;
    if (params?.get("mode") === "new") router.replace("/projects");
    if (params?.get("mode") === "new-canvas" && !intentHandled.current) {
      intentHandled.current = true;
      canvasActions.openCreate();
      router.replace("/projects?type=canvas");
    }
  }, [parentId, params, router, canvasActions]);
  const query = useLiveQuery(
    identity && kind !== "canvas"
      ? ([
          "personal-projects",
          identity,
          parentId ?? "root",
          page,
          search,
          sort,
          pageSize,
          kind,
        ] as const)
      : null,
    async (key, options) => {
      if (!key) throw new Error("项目查询范围不可用");
      const [, , scope, currentPage, keyword, order, size] = key;
      return readResourcePrefix(
        async (current, pageSize) =>
          unwrap(
            await api.listPersonalProjects(
              {
                parentProjectId: scope === "root" ? undefined : scope,
                current,
                pageSize,
                keyword,
                sortField: "updateTime",
                sortOrder: order === "oldest" ? "ascend" : "descend",
              },
              options,
            ),
          ),
        currentPage * size,
      );
    },
    { shouldRetryOnError: false },
  );
  const canvasQuery = useLiveQuery(
    identity && kind !== "project"
      ? [
          "project-canvases",
          identity,
          parentId ?? "root",
          page,
          search,
          sort,
          pageSize,
          kind,
        ]
      : null,
    async (key, options = {}) => {
      if (!key) throw new Error("画布目录范围不可用");
      const [, , scope, currentPage, keyword, order, size] = key;
      return readResourcePrefix(
        async (current, pageSize) =>
          unwrap(
            await canvasApi.listCanvases(
              {
                projectId: scope === "root" ? undefined : scope,
                current,
                pageSize,
                keyword,
                sortField: "updateTime",
                sortOrder: order === "oldest" ? "ASC" : "DESC",
              },
              options,
            ),
          ),
        currentPage * size,
      );
    },
    { shouldRetryOnError: false },
  );
  const canvasStatus = useLiveQuery(
    identity && search.trim() && kind !== "project" && canvasQuery.error
      ? ["canvas-search-status", identity, "directory", parentId ?? "root"]
      : null,
    async (key, options = {}) => {
      if (!key) throw new Error("画布目录范围不可用");
      const [, , , project] = key;
      return unwrap(
        await getCanvasSearchStatus(
          {
            projectId: project === "root" ? undefined : String(project),
          },
          options,
        ),
      );
    },
    { shouldRetryOnError: false },
  );
  const canvasSyncing =
    !!search.trim() &&
    !!canvasQuery.error &&
    canvasStatus.data?.state === "SYNCING";
  useErrorNotification(query.error, "个人项目读取失败");
  useErrorNotification(
    canvasSyncing ? undefined : canvasQuery.error,
    "画布读取失败",
  );
  const projectItems: WorkspaceResource[] = (
    kind === "canvas" ? [] : (query.data?.records ?? [])
  ).map((item) => ({
    kind: "project",
    id: String(item.projectId),
    title: item.title,
    coverFileId: item.coverFileId,
    coverFileVersion: item.coverFileVersion,
    updatedAt: item.updatedAt,
    href: `/projects/${item.projectId}`,
    revision: item.revision,
    parentId,
  }));
  const canvasItems: WorkspaceResource[] = (
    kind === "project" ? [] : (canvasQuery.data?.records ?? [])
  ).map((item) => ({
    kind: "canvas",
    id: String(item.canvasId),
    title: item.title,
    coverFileId: item.coverFileId,
    coverFileVersion: item.coverFileVersion,
    updatedAt: item.updatedAt,
    href: `/canvases/${item.canvasId}${parentId ? `?projectId=${encodeURIComponent(parentId)}` : ""}`,
    revision: item.revision,
    projectBindingRevision: item.projectBindingRevision,
    parentId,
  }));
  const failed = !!query.error || !!canvasQuery.error;
  const loading = query.isLoading || canvasQuery.isLoading;
  const hasData =
    (kind === "canvas" || !!query.data) &&
    (kind === "project" || !!canvasQuery.data);
  const complete = hasData && !failed;
  const total =
    (kind === "canvas" ? 0 : Number(query.data?.total ?? 0)) +
    (kind === "project" ? 0 : Number(canvasQuery.data?.total ?? 0));
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (complete && page > pages) setPage(pages);
  const items = mergeResourcePage(
    projectItems,
    canvasItems,
    failed && !hasData ? 1 : page,
    pageSize,
    sort === "oldest",
  );
  const selectionDisabled =
    !complete ||
    query.isValidating ||
    canvasQuery.isValidating ||
    keyword !== search ||
    canvasActions.active ||
    creating ||
    canvasOpen;
  const pageTargets = items
    .map(resourceTarget)
    .filter((item): item is WorkspaceResourceTarget => item !== null);
  const pageSelected = pageTargets.filter((item) =>
    selected.some((value) => resourceKey(value) === resourceKey(item)),
  ).length;
  useEffect(() => {
    if (parentId || params?.get("mode") !== "recent" || !canvasQuery.data)
      return;
    const first = canvasQuery.data.records[0];
    router.replace(
      first ? `/canvases/${first.canvasId}` : "/projects?type=canvas",
    );
  }, [parentId, params, canvasQuery.data, router]);
  const restoreNewFocus = (event: Event) => {
    if (newButton.current?.isConnected) {
      event.preventDefault();
      newButton.current.focus();
    }
  };
  const createProject = () => {
    setCreationMounted(true);
    setCreating(true);
  };
  const openCanvas = (mode: "create" | "attach") => {
    setCanvasMode(mode);
    setCanvasOpen(true);
  };
  const createCanvas = () =>
    parent
      ? openCanvas("create")
      : canvasActions.openCreate({ currentTarget: newButton.current });
  return (
    <section
      className="project-resources flex flex-1 flex-col gap-6"
      aria-label={parent ? "项目内容" : "项目空间"}
    >
      <PageHeader title={parent?.title ?? "项目空间"}>
        <div className="flex flex-wrap items-center gap-2">
          {parent && identity && (
            <ProjectMetadataActions
              compact
              project={parent}
              onChanged={onChanged ?? (() => {})}
            />
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                ref={newButton}
                disabled={!identity || canvasActions.active}
              >
                新建
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuGroup>
                <DropdownMenuItem
                  onSelect={createProject}
                  disabled={parent ? !parent.canCreateChild : false}
                >
                  新建项目
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={createCanvas}>
                  新建画布
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/assets">上传素材</Link>
                </DropdownMenuItem>
                {parent && (
                  <DropdownMenuItem onSelect={() => openCanvas("attach")}>
                    添加已有画布
                  </DropdownMenuItem>
                )}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </PageHeader>
      <Tabs
        value={kind}
        onValueChange={(value) => {
          setKind(value);
          setPage(1);
        }}
        className="flex flex-1 flex-col gap-5"
      >
        <TabsList
          aria-label="资源类型"
          className="w-fit group-data-horizontal/tabs:h-10"
        >
          <TabsTrigger value="all" className="px-6">
            全部
          </TabsTrigger>
          <TabsTrigger value="project" className="px-6">
            项目
          </TabsTrigger>
          <TabsTrigger value="canvas" className="px-6">
            画布
          </TabsTrigger>
        </TabsList>
        <TabsContent value={kind} className="flex flex-1 flex-col gap-5">
          <FilterToolbar className="project-resource-filters">
            <Input
              className="min-w-0"
              aria-label="搜索名称"
              placeholder="搜索名称…"
              maxLength={100}
              value={keyword}
              onChange={(event) => {
                setKeyword(event.target.value);
                setPage(1);
              }}
            />
            <div className="flex justify-end">
              <Select
                value={sort}
                onValueChange={(value) => {
                  setSort(value);
                  setPage(1);
                }}
              >
                <SelectTrigger className="w-30 shrink-0" aria-label="排序">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="updated">最近更新</SelectItem>
                    <SelectItem value="oldest">最早更新</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </FilterToolbar>
          <div className="project-resource-selection" aria-label="内容批量管理">
            <div className="project-resource-selection-info">
              <Field orientation="horizontal" className="min-h-10 w-auto">
                <Checkbox
                  ref={selectPage}
                  id="select-resource-page"
                  aria-label="全选当前页"
                  disabled={selectionDisabled || !pageTargets.length}
                  checked={
                    pageTargets.length > 0 &&
                    pageSelected === pageTargets.length
                      ? true
                      : pageSelected
                        ? "indeterminate"
                        : false
                  }
                  onCheckedChange={(checked) =>
                    changeSelection((previous) => {
                      const other = previous.filter(
                        (item) =>
                          !pageTargets.some(
                            (current) =>
                              resourceKey(current) === resourceKey(item),
                          ),
                      );
                      return checked === true
                        ? [...other, ...pageTargets]
                        : other;
                    })
                  }
                />
                <FieldLabel htmlFor="select-resource-page">
                  全选当前页
                </FieldLabel>
              </Field>
              <span
                className="text-xs text-muted-foreground"
                aria-live="polite"
              >
                {selected.length > 0 ? `已选 ${selected.length} 项` : null}
              </span>
            </div>
            <div className="flex min-h-10 justify-end gap-2">
              {selected.length > 0 && (
                <>
                  <Button
                    variant="ghost"
                    disabled={!selected.length || canvasActions.active}
                    onClick={() => {
                      changeSelection(() => []);
                      selectPage.current?.focus();
                    }}
                  >
                    清空选择
                  </Button>
                  <Button
                    variant="destructive"
                    disabled={selectionDisabled || !selected.length}
                    onClick={(event) =>
                      canvasActions.openResourceAction(
                        "delete",
                        selected,
                        event.currentTarget,
                        removeDeleted,
                        (command) => {
                          if (identity)
                            invalidateWorkspaceResource(
                              mutatePrivateCache,
                              identity,
                              command,
                            );
                        },
                      )
                    }
                  >
                    删除选中
                  </Button>
                </>
              )}
            </div>
          </div>
          {failed && (
            <div role="status" className="flex items-center gap-3 text-sm">
              <p>
                {query.error && canvasQuery.error
                  ? "内容暂时无法加载"
                  : query.error
                    ? "项目暂时无法加载"
                    : canvasSyncing
                      ? "画布目录正在同步，请稍后重试。"
                      : "画布暂时无法加载"}
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  if (query.error) void query.mutate();
                  if (canvasQuery.error) {
                    void canvasQuery.mutate();
                    if (search.trim()) void canvasStatus.mutate();
                  }
                }}
              >
                重试
              </Button>
            </div>
          )}
          {loading ? (
            <div
              className="projects-grid"
              aria-label="正在加载"
              aria-busy="true"
            >
              {[0, 1, 2].map((i) => (
                <WorkspaceCardSkeleton key={i} showDate />
              ))}
            </div>
          ) : items.length > 0 ? (
            <div className="projects-grid">
              {items.map((item) => (
                <WorkspaceResourceCard
                  key={`${item.kind}:${item.id}`}
                  resource={item}
                  selected={selected.some(
                    (target) => resourceKey(target) === resourceKey(item),
                  )}
                  disabled={selectionDisabled || !resourceTarget(item)}
                  onSelectionChange={(checked) =>
                    changeSelection((previous) => {
                      const remaining = previous.filter(
                        (target) => resourceKey(target) !== resourceKey(item),
                      );
                      const target = resourceTarget(item);
                      return checked && target
                        ? [...remaining, target]
                        : remaining;
                    })
                  }
                  onAction={(operation, trigger) => {
                    const target = resourceTarget(item);
                    if (target)
                      canvasActions.openResourceAction(
                        operation,
                        [target],
                        trigger,
                        removeDeleted,
                        (command) => {
                          if (identity)
                            invalidateWorkspaceResource(
                              mutatePrivateCache,
                              identity,
                              command,
                            );
                        },
                      );
                  }}
                />
              ))}
            </div>
          ) : null}
          {complete && total === 0 && (
            <Empty className="min-h-project-empty flex-none">
              <EmptyHeader>
                <EmptyTitle>
                  {search ? "没有找到相关内容" : "还没有内容"}
                </EmptyTitle>
              </EmptyHeader>
              {!search && (
                <Button
                  onClick={kind === "canvas" ? createCanvas : createProject}
                >
                  {kind === "canvas" ? "新建画布" : "新建项目"}
                </Button>
              )}
            </Empty>
          )}
          {!failed && (
            <ListPagination
              label="项目内容分页"
              total={total}
              current={page}
              pageSize={pageSize}
              pending={!complete || keyword !== search}
              onPageChange={setPage}
              onPageSizeChange={(size) => {
                dispatch(
                  workspaceActions.setProjectFilters({ pageSize: size }),
                );
                setPage(1);
              }}
            />
          )}
        </TabsContent>
      </Tabs>
      {creationMounted && identity && (
        <CreatePersonalProjectDialog
          key={identity}
          open={creating}
          onCloseAutoFocus={restoreNewFocus}
          onOpenChange={setCreating}
          identity={identity}
          parent={parent}
        />
      )}
      {parent && identity && (
        <ProjectCanvasDialog
          key={`${identity}:${parentId}`}
          identity={identity}
          projectId={parentId!}
          projectPath={
            parent.breadcrumbs?.map((item) => item.title).join(" / ") ??
            parent.title
          }
          mode={canvasMode}
          open={canvasOpen}
          onCloseAutoFocus={restoreNewFocus}
          onOpenChange={setCanvasOpen}
          onCompleted={() => {
            void canvasQuery.mutate();
          }}
        />
      )}
    </section>
  );
}

export function CreatePersonalProjectDialog({
  open,
  onOpenChange,
  identity,
  source,
  parent,
  onCloseAutoFocus,
  onStarted,
  onAborted,
}: {
  open: boolean;
  onOpenChange: (value: boolean) => void;
  identity: string;
  source?: CanvasContract.CanvasVO;
  parent?: CanvasContract.ProjectWorkspaceVO;
  onCloseAutoFocus?: (event: Event) => void;
  onStarted?: () => void;
  onAborted?: () => void;
}) {
  const router = useRouter();
  const { mutate } = useSWRConfig();
  const parentId =
    parent?.projectId == null ? undefined : String(parent.projectId);
  const storageKey = `glowloom:personal-project:create:${identity}:${source?.canvasId ?? (parentId ? `parent:${parentId}` : "new")}`;
  const [title, setTitle] = useState(source?.title ?? "");
  const [command, setCommand] =
    useState<CanvasContract.ProjectCreateRequest | null>(() => {
      try {
        const value = JSON.parse(
          pageBusinessMemory.getItem(storageKey) ?? "null",
        );
        return value &&
          typeof value.commandId === "string" &&
          typeof value.title === "string"
          ? value
          : null;
      } catch {
        return null;
      }
    });
  const [busy, setBusy] = useState(false);
  const [result, setResult] =
    useState<CanvasContract.ProjectCreationStatusVO | null>(null);
  const active = useRef(true);
  const currentCommand = useRef(command);
  const handled = useRef<string | null>(null);
  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  }, [open]);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  useEffect(() => {
    if (command && result?.status !== "ABORTED") onStarted?.();
  }, [command, result?.status, onStarted]);
  const accept = (value: CanvasContract.ProjectCreationStatusVO) => {
    if (
      !active.current ||
      value.commandId !== currentCommand.current?.commandId ||
      handled.current === value.commandId
    )
      return;
    setResult(value);
    if (value.status === "COMPLETED" && value.result?.projectId) {
      handled.current = value.commandId;
      currentCommand.current = null;
      pageBusinessMemory.removeItem(storageKey);
      setCommand(null);
      setResult(null);
      onOpenChange(false);
      onAborted?.();
      void mutate(
        (key) =>
          Array.isArray(key) &&
          key[0] === "personal-projects" &&
          key[1] === identity,
      );
      if (openRef.current)
        router.push(
          value.result.canvasId
            ? `/canvases/${value.result.canvasId}?projectId=${encodeURIComponent(String(value.projectId))}`
            : `/projects/${value.projectId}`,
        );
    } else if (value.status === "ABORTED") {
      handled.current = value.commandId;
      currentCommand.current = null;
      pageBusinessMemory.removeItem(storageKey);
      setCommand(null);
      setResult(null);
      onAborted?.();
      onOpenChange(false);
      notifyError(
        new Error("项目未能完成初始化，原画布已保留。可以重新创建。"),
        "项目创建未完成",
      );
    }
  };
  const query = useSWR(
    command && !busy && result?.status !== "ABORTED"
      ? ["personal-project-creation", identity, command.commandId]
      : null,
    async () =>
      unwrap(
        await api.getPersonalProjectCreation({ commandId: command!.commandId }),
      ),
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      onSuccess: accept,
    },
  );
  useErrorNotification(
    query.error instanceof ApiError && query.error.code === 40400
      ? undefined
      : query.error,
    "项目状态查询未完成",
  );
  const submit = async () => {
    if (busy) return;
    if (!command && !title.trim()) {
      notifyError(new Error("请输入项目名称"));
      return;
    }
    const original = command ?? {
      commandId: crypto.randomUUID(),
      title: title.trim(),
      createMode: "METADATA_ONLY",
      parentProjectId: parentId,
    };
    try {
      pageBusinessMemory.setItem(storageKey, JSON.stringify(original));
      currentCommand.current = original;
      setCommand(original);
      onStarted?.();
      setBusy(true);
      accept(unwrap(await api.createPersonalProject(original)));
    } catch (error) {
      if (active.current) {
        if (error instanceof ApiError && error.code === 40000) {
          pageBusinessMemory.removeItem(storageKey);
          currentCommand.current = null;
          setCommand(null);
          onAborted?.();
        }
        notifyError(error, "项目创建未完成");
      }
    } finally {
      if (active.current) setBusy(false);
    }
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!busy) onOpenChange(value);
      }}
    >
      <DialogContent onCloseAutoFocus={onCloseAutoFocus}>
        <DialogHeader>
          <DialogTitle>新建项目</DialogTitle>
          <DialogDescription>
            {source
              ? "先创建项目，再在项目中明确选择归入此独立画布。画布内容和历史保留。"
              : parent
                ? `所属项目：${parent.breadcrumbs?.map((item) => item.title).join(" / ") ?? parent.title}`
                : "填写项目名称。"}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <FieldGroup>
            <Field data-disabled={busy || !!command}>
              <FieldLabel htmlFor="personal-project-title">项目名称</FieldLabel>
              <Input
                id="personal-project-title"
                maxLength={100}
                value={command?.title ?? title}
                disabled={!!command || busy}
                onChange={(event) => setTitle(event.target.value)}
              />
            </Field>
            {command && (
              <FieldDescription role="status">
                {result?.status === "ABORTED"
                  ? "初始化未完成，原画布保留"
                  : "正在确认原创建结果，可以关闭后在项目空间查看。"}
              </FieldDescription>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => onOpenChange(false)}
              >
                关闭
              </Button>
              <Button
                disabled={
                  busy ||
                  result?.status === "PROCESSING" ||
                  result?.status === "ABORTED"
                }
              >
                {busy ? <Spinner data-icon="inline-start" /> : null}
                {busy ? "正在创建" : command ? "重试原创建" : "创建项目"}
              </Button>
            </DialogFooter>
          </FieldGroup>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function PersonalProjectPage({ projectId }: { projectId: string }) {
  const { account, ready, signingOut } = useAccount();
  const identity =
    ready && account && !signingOut ? `USER:${account.id}` : null;
  const query = useSWR(
    identity ? ["personal-project-workspace", identity, projectId] : null,
    async () => unwrap(await api.getPersonalProjectWorkspace({ projectId })),
    {
      shouldRetryOnError: false,
    },
  );
  useErrorNotification(query.error, "项目读取失败");
  if (!identity) return <LoginPrompt description="登录后打开个人项目。" />;
  if (query.error)
    return (
      <section className="workspace-page">
        <Empty>
          <EmptyHeader>
            <EmptyTitle>项目暂时无法加载</EmptyTitle>
          </EmptyHeader>
          <Button variant="outline" onClick={() => void query.mutate()}>
            重试
          </Button>
        </Empty>
      </section>
    );
  if (query.data?.workspaceState === "READY")
    return (
      <section
        key={identity + ":" + projectId}
        className="workspace-page gap-6"
      >
        <ProjectBreadcrumbs project={query.data} />
        <PersonalProjects
          parent={query.data}
          onChanged={() => void query.mutate()}
        />
      </section>
    );
  return (
    <PageLoading
      label={
        query.data?.workspaceState === "INITIALIZING"
          ? "正在准备项目画布"
          : "正在读取项目"
      }
    />
  );
}
