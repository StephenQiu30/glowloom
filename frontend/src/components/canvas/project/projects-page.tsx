"use client";
import { Button } from "@/components/ui/button";
import { FolderOpen, Square, PanelLeft, Plus, Trash2 } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useDeferredValue, useEffect, useState } from "react";
import { CanvasProjectCard } from "@/components/canvas/project/canvas-project-card";
import {
  useAppDispatch,
  useAppSelector,
} from "@/components/canvas/runtime/store/hooks";
import { workspaceActions } from "@/components/canvas/runtime/store/app-store";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { WorkspaceCardSkeleton } from "@/components/canvas/runtime/components/feedback/workspace-card-skeleton";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useProjects } from "@/components/canvas/hooks/use-projects";
import { ProjectReadEmpty } from "./project-feedback";
import { useProjectActions } from "./project-action-provider";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import type { ProjectCommandTarget } from "../hooks/use-projects";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { cn } from "@/lib/utils";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { LoginPrompt } from "@/components/canvas/runtime/components/account/auth/login-prompt";
import { PersonalProjects } from "./personal-projects";
import Link from "next/link";

export default function ProjectsPage() {
  const params = useSearchParams();
  const { account, ready, signingOut } = useAccount();
  if (!ready || !account || signingOut)
    return <LoginPrompt description="登录后查看和创建项目。" />;
  return (
    <section className="workspace-page projects-page">
      <PersonalProjects key={`${account.id}:${params?.get("type") ?? "all"}`} />
    </section>
  );
}

export function IndependentCanvasesPage() {
  const { account, ready, signingOut } = useAccount();
  if (!ready || !account || signingOut)
    return <LoginPrompt description="登录后查看和创建画布。" />;
  return <ProjectLibrary key={String(account.id)} />;
}

function ProjectLibrary() {
  const dispatch = useAppDispatch();
  const router = useRouter();
  const searchParams = useSearchParams();
  const keyword = useAppSelector((state) => state.workspace.projects.keyword);
  const sort = useAppSelector((state) => state.workspace.projects.sort);
  const pageSize = useAppSelector((state) => state.workspace.projects.pageSize);
  const [current, setCurrent] = useState(1);
  const [selected, setSelected] = useState<ProjectCommandTarget[]>([]);
  const [view, setView] = useState("grid");
  const { active, openCreate, openRename, openDelete, openBatchDelete } =
    useProjectActions();
  const query = useDeferredValue(keyword);
  const { data, error, isLoading, isValidating, mutate } = useProjects({
    current,
    pageSize,
    keyword: query,
    sortField: sort === "name" ? "title" : "updateTime",
    sortOrder: sort === "name" ? "ASC" : "DESC",
  });
  const mode = searchParams?.get("mode") ?? null;
  useEffect(() => {
    if (mode === "recent" && data?.records[0])
      router.replace(`/canvases/${data.records[0].id}`);
  }, [mode, data, router]);
  useEffect(() => {
    if (mode === "new") {
      openCreate();
      router.replace("/canvases");
    }
  }, [mode, openCreate, router]);
  const pages = data ? Math.max(1, Number(data.pages)) : 1;
  // A successful reread can remove the last page after deletion or a rename.
  if (data && !error && current > pages) setCurrent(pages);
  const selectionDisabled =
    active || isLoading || isValidating || !!error || query !== keyword;
  const pageRecords = data?.records ?? [];
  const selectedOnPage = pageRecords.filter((project) =>
    selected.some((item) => String(item.id) === String(project.id)),
  ).length;
  const choose = (project: ProjectCommandTarget, checked: boolean) => {
    setSelected((previous) => {
      const remaining = previous.filter(
        (item) => String(item.id) !== String(project.id),
      );
      return checked
        ? [
            ...remaining,
            {
              id: project.id,
              title: project.title,
              revision: project.revision,
              projectBindingRevision: project.projectBindingRevision,
            },
          ]
        : remaining;
    });
  };
  return (
    <section className="workspace-page projects-page">
      <header className="workspace-list-header">
        <h1 className="workspace-list-title">独立画布</h1>
        <div className="workspace-list-actions">
          <Button
            className="workspace-list-action projects-create"
            onClick={openCreate}
          >
            <Plus data-icon="inline-start" aria-hidden="true" />
            <span className="hidden sm:inline">新建画布</span>
            <span className="sm:hidden">新建</span>
          </Button>
        </div>
      </header>
      <Button asChild variant="link" className="self-start">
        <Link href="/projects">返回项目空间</Link>
      </Button>
      <div className="workspace-list-filters">
        <Input
          className="workspace-list-search"
          type="search"
          placeholder="搜索独立画布…"
          aria-label="搜索独立画布"
          value={keyword}
          onChange={(event) => {
            setCurrent(1);
            setSelected([]);
            dispatch(
              workspaceActions.setProjectFilters({
                keyword: event.target.value,
              }),
            );
          }}
        />
        <div className="workspace-list-filter-actions">
          <Select
            value={sort}
            onValueChange={(value) => {
              setCurrent(1);
              setSelected([]);
              dispatch(
                workspaceActions.setProjectFilters({
                  sort: value === "name" ? "name" : "updated",
                }),
              );
            }}
          >
            <SelectTrigger
              className="workspace-list-sort"
              aria-label="画布排序"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="updated">最近更新</SelectItem>
                <SelectItem value="name">按名称排序</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <div
            className="workspace-list-selection flex flex-wrap items-center justify-between gap-3"
            aria-label="画布批量操作"
          >
            <div className="flex flex-wrap items-center gap-4">
              <Field orientation="horizontal" className="w-auto">
                <Checkbox
                  id="select-project-page"
                  checked={
                    pageRecords.length > 0 &&
                    selectedOnPage === pageRecords.length
                      ? true
                      : selectedOnPage > 0
                        ? "indeterminate"
                        : false
                  }
                  disabled={selectionDisabled || !pageRecords.length}
                  onCheckedChange={(checked) => {
                    setSelected((previous) => {
                      const remaining = previous.filter(
                        (item) =>
                          !pageRecords.some(
                            (project) => String(project.id) === String(item.id),
                          ),
                      );
                      return checked === true
                        ? [
                            ...remaining,
                            ...pageRecords.map(
                              ({
                                id,
                                title,
                                revision,
                                projectBindingRevision,
                              }) => ({
                                id,
                                title,
                                revision,
                                projectBindingRevision,
                              }),
                            ),
                          ]
                        : remaining;
                    });
                  }}
                />
                <FieldLabel htmlFor="select-project-page">
                  全选当前页
                </FieldLabel>
              </Field>
              <span
                className={cn(
                  "text-xs text-muted-foreground",
                  selected.length === 0 && "project-count",
                )}
                aria-live="polite"
              >
                {selected.length
                  ? `已选择 ${selected.length} 个画布（支持跨页）`
                  : error
                    ? "暂未读取"
                    : data
                      ? `${data.total} 个画布`
                      : "正在读取"}
              </span>
            </div>
            {selected.length > 0 && (
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="ghost"
                  disabled={active || !selected.length}
                  onClick={() => setSelected([])}
                >
                  清空选择
                </Button>
                <Button
                  variant="destructive"
                  disabled={selectionDisabled || !selected.length}
                  onClick={() =>
                    openBatchDelete(selected, (id) =>
                      setSelected((previous) =>
                        previous.filter((item) => String(item.id) !== id),
                      ),
                    )
                  }
                >
                  <Trash2 data-icon="inline-start" aria-hidden="true" />
                  批量删除
                </Button>
              </div>
            )}
          </div>
          <ToggleGroup
            type="single"
            value={view}
            onValueChange={(value) => {
              if (value) setView(value);
            }}
            aria-label="画布视图"
            className="workspace-list-view"
          >
            <ToggleGroupItem value="grid" aria-label="网格视图">
              <Square aria-hidden="true" />
            </ToggleGroupItem>
            <ToggleGroupItem value="list" aria-label="列表视图">
              <PanelLeft aria-hidden="true" />
            </ToggleGroupItem>
          </ToggleGroup>
        </div>
      </div>
      {error ? (
        <ProjectReadEmpty
          canvas
          retrying={isValidating}
          onRetry={() => void mutate().catch(() => undefined)}
        />
      ) : isLoading || !data ? (
        <div
          className="projects-grid"
          aria-busy="true"
          aria-label="正在读取画布列表"
        >
          {Array.from({ length: pageSize }, (_, index) => (
            <WorkspaceCardSkeleton key={index} showDate />
          ))}
        </div>
      ) : data.records.length ? (
        <div
          className={cn(view === "grid" ? "projects-grid" : "projects-list")}
          aria-label="画布列表"
          aria-busy={isValidating}
        >
          {data.records.map((project) => (
            <CanvasProjectCard
              key={project.id}
              project={project}
              onRename={() => openRename(project)}
              onDelete={() => openDelete(project)}
              selected={selected.some(
                (item) => String(item.id) === String(project.id),
              )}
              onSelectionChange={(checked) => choose(project, checked)}
              selectionDisabled={selectionDisabled}
            />
          ))}
        </div>
      ) : (
        <Empty className="min-h-72">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <FolderOpen />
            </EmptyMedia>
            <EmptyTitle>{query ? "没有匹配的画布" : "还没有画布"}</EmptyTitle>
            <EmptyDescription>
              {query
                ? "试试其他名称，或清空搜索查看所有画布。"
                : "新建一张画布，开始创作。"}
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button onClick={openCreate}>新建画布</Button>
          </EmptyContent>
        </Empty>
      )}
      {data && !error && (
        <ListPagination
          label="画布分页"
          total={Number(data.total)}
          current={current}
          pageSize={pageSize}
          disabled={isValidating || query !== keyword}
          onPageChange={setCurrent}
          onPageSizeChange={(size) => {
            setCurrent(1);
            dispatch(workspaceActions.setProjectFilters({ pageSize: size }));
          }}
        />
      )}
    </section>
  );
}
