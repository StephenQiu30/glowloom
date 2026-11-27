"use client";

import "./asset-workspace.css";
import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import dynamic from "next/dynamic";
import Link from "next/link";
import { ChevronDown } from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { PageHeader } from "@/components/layout/page-header";
import { PageLayout } from "@/components/layout/page-layout";
import { HeaderNavigation } from "@/components/layout/header-navigation";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ToolbarGroup } from "@/components/ui/toolbar";
import { CollectionFilterBar } from "@/components/layout/collection-filter-bar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import {
  assetDateRange,
  assetKinds,
  type AssetFilters,
  type AssetKind,
  type AssetSource,
  defaultAssetFilters as defaultFilters,
  hasAssetFilters,
} from "./asset-query";
import { AssetFilterMenu } from "./asset-filter-menu";
import { PersonalHistoryList } from "./personal-history-list";
import { AssetCanvasList } from "./asset-canvas-list";
import {
  readAssetLocation,
  writeAssetLocation,
  type AssetLocation,
} from "./asset-location";

const ReferenceAssetsPage = dynamic(
  () => import("./assets-page").then((m) => m.ReferenceAssetsPage),
  {
    loading: () => (
      <ContentState
        state="loading"
        loadingLayout="grid"
        title="正在读取参考素材"
      />
    ),
  },
);
const UnifiedSubjectLibrary = dynamic(
  () =>
    import("@/components/subjects/unified-library").then(
      (m) => m.UnifiedSubjectLibrary,
    ),
  {
    loading: () => (
      <ContentState state="loading" loadingLayout="grid" title="正在读取主体" />
    ),
  },
);
const sources = [
  { id: "history", label: "生成历史" },
  { id: "canvas", label: "画布" },
  { id: "short_drama", label: "短剧工作室" },
  { id: "motion", label: "动效工作室" },
  { id: "marketing", label: "营销工作室" },
  { id: "subjects", label: "主体" },
] as const;

export function AssetWorkspace({
  initialView = "history",
  initialLocation,
}: {
  initialView?: AssetSource | "reference";
  initialLocation?: AssetLocation;
}) {
  const session = useOptionalSession();
  const space = useSpace();
  return (
    <WorkspaceScope
      key={`${session?.actor.id ?? "guest"}:${space.selected?.id ?? "none"}`}
      initialView={initialView}
      initialLocation={initialLocation}
      authenticated={!!session}
    />
  );
}

function WorkspaceScope({
  initialView,
  initialLocation,
  authenticated,
}: {
  initialView: AssetSource | "reference";
  initialLocation?: AssetLocation;
  authenticated: boolean;
}) {
  const session = useOptionalSession();
  const space = useSpace();
  const [view, setView] = useState(initialLocation?.view ?? initialView);
  const [kind, setKind] = useState<AssetKind>(initialLocation?.kind ?? "image");
  const [search, setSearch] = useState(initialLocation?.filters.search ?? "");
  const [filters, setFilters] = useState(
    initialLocation?.filters ?? defaultFilters,
  );
  const [referenceFilter, setReferenceFilter] = useState(
    initialLocation?.reference,
  );
  const login = useLoginDialog();
  const [locked, setLocked] = useState(false);
  const [batch, setBatch] = useState(false);
  const [collection, setCollection] = useState<"current" | "legacy">(
    initialLocation?.collection ?? "current",
  );
  const restoreLocation = useEffectEvent(() => {
    const restored = readAssetLocation(
      new URLSearchParams(window.location.search),
    );
    setBatch(false);
    setView(restored.view);
    setKind(restored.kind);
    setCollection(restored.collection);
    setSearch(restored.filters.search);
    setFilters(restored.filters);
    setReferenceFilter(restored.reference);
  });
  useEffect(() => {
    const restore = () => restoreLocation();
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);
  useEffect(() => {
    if (view === "reference") return;
    const url = new URL(window.location.href);
    url.search = writeAssetLocation(
      { view, kind, collection, filters },
      url.searchParams,
    );
    if (url.href !== window.location.href)
      window.history.replaceState(null, "", url);
  }, [view, kind, collection, filters]);
  function changeView(value: AssetSource | "reference") {
    if (locked) return;
    setBatch(false);
    setView(value);
    setKind(value === "motion" ? "video" : "image");
    setCollection("current");
    setSearch("");
    setFilters(defaultFilters);
    setReferenceFilter(undefined);
    const url = new URL(window.location.href);
    url.search = writeAssetLocation(
      {
        view: value,
        kind: value === "motion" ? "video" : "image",
        collection: "current",
        filters: defaultFilters,
      },
      url.searchParams,
    );
    window.history.pushState(null, "", url);
  }
  const pageSearch = useMemo(
    () => ({
      value: search,
      onValueChange: setSearch,
      onSearch: (value: string) => {
        if (!locked && !batch)
          setFilters((current) => ({ ...current, search: value.trim() }));
      },
      label:
        view === "history"
          ? "搜索本人生成历史"
          : view === "short_drama"
            ? "搜索短剧"
            : view === "subjects"
              ? "搜索主体"
              : "搜索画布",
      disabled: !authenticated || locked || batch,
    }),
    [search, view, authenticated, locked, batch],
  );
  if (view === "reference")
    return (
      <ReferenceAssetsPage
        key={JSON.stringify(referenceFilter)}
        initialFilter={referenceFilter}
        onBack={() => changeView("history")}
      />
    );
  const filtered = hasAssetFilters(filters);
  const clear = () => {
    setSearch("");
    setFilters((current) => ({
      ...current,
      search: "",
      from: "",
      through: "",
      favorite: false,
      processTypes: [],
      resolutions: [],
      ratios: [],
      generationModes: [],
    }));
  };
  return (
    <PageLayout
      screen="assets"
      search={pageSearch}
      hideSearch={!authenticated || batch}
    >
      <PageHeader title="资产和主体" hideTitle />
      <Tabs
        value={view}
        onValueChange={(value) => changeView(value as AssetSource)}
        className="asset-workspace-tabs"
      >
        <HeaderNavigation label="资产分类" active={view}>
          <TabsList
            variant="workspace"
            aria-label="资产来源"
            className="gap-(--secondary-tab-gap)"
          >
            {sources.map((source) => (
              <TabsTrigger
                key={source.id}
                value={source.id}
                disabled={locked}
                className="rounded-full px-(--secondary-tab-padding-inline)"
              >
                {source.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </HeaderNavigation>
        {authenticated && view !== "subjects" && (
          <CollectionFilterBar aria-label="资产筛选">
            {(view === "history" ||
              view === "motion" ||
              view === "marketing") && (
              <ToggleGroup
                variant="text"
                size="toolbar"
                spacing={1}
                type="single"
                value={kind}
                onValueChange={(value) => {
                  if (value) {
                    setKind(value as AssetKind);
                    setFilters((current) => ({
                      ...current,
                      processTypes: [],
                      resolutions: [],
                      ratios: [],
                      generationModes: [],
                    }));
                  }
                }}
                aria-label="生成内容类型"
                className="max-w-full gap-0"
              >
                {assetKinds
                  .filter((item) => view !== "motion" || item.id !== "document")
                  .map((item) => (
                    <ToggleGroupItem
                      key={item.id}
                      value={item.id}
                      disabled={locked}
                    >
                      {item.label}
                    </ToggleGroupItem>
                  ))}
              </ToggleGroup>
            )}
            {view === "canvas" && (
              <ToggleGroup
                variant="text"
                size="toolbar"
                spacing={1}
                type="single"
                value={collection}
                onValueChange={(value) => {
                  if (value) setCollection(value as "current" | "legacy");
                }}
                aria-label="画布类型"
                className="gap-0"
              >
                <ToggleGroupItem value="current" disabled={locked}>
                  画布
                </ToggleGroupItem>
                <ToggleGroupItem value="legacy" disabled={locked}>
                  历史画布
                </ToggleGroupItem>
              </ToggleGroup>
            )}
            {view !== "short_drama" && (
              <Separator orientation="vertical" className="max-md:hidden" />
            )}
            <ToolbarGroup
              appearance="plain"
              className="flex-wrap"
              aria-label="筛选与排序"
            >
              <AssetFilterMenu
                kind={view === "history" ? kind : undefined}
                value={filters}
                disabled={!authenticated || locked}
                onChange={setFilters}
              />
              <TimeFilter
                value={filters}
                disabled={!authenticated || locked}
                onChange={setFilters}
              />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="filter"
                    size="toolbar"
                    disabled={!authenticated || locked}
                    aria-label="资产排序"
                  >
                    排序
                    <ChevronDown data-icon="inline-end" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <DropdownMenuGroup>
                    <DropdownMenuLabel>顺序</DropdownMenuLabel>
                    <DropdownMenuRadioGroup
                      value={filters.order}
                      onValueChange={(value) =>
                        setFilters((current) => ({
                          ...current,
                          order: value as AssetFilters["order"],
                        }))
                      }
                    >
                      <DropdownMenuRadioItem value="desc">
                        近-远
                      </DropdownMenuRadioItem>
                      <DropdownMenuRadioItem value="asc">
                        远-近
                      </DropdownMenuRadioItem>
                    </DropdownMenuRadioGroup>
                  </DropdownMenuGroup>
                  {view !== "history" && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuGroup>
                        <DropdownMenuLabel>排序方式</DropdownMenuLabel>
                        <DropdownMenuRadioGroup
                          value={filters.sort}
                          onValueChange={(value) =>
                            setFilters((current) => ({
                              ...current,
                              sort: value as AssetFilters["sort"],
                            }))
                          }
                        >
                          <DropdownMenuRadioItem value="updated">
                            修改时间
                          </DropdownMenuRadioItem>
                          <DropdownMenuRadioItem value="created">
                            创建时间
                          </DropdownMenuRadioItem>
                        </DropdownMenuRadioGroup>
                      </DropdownMenuGroup>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
              {filtered && (
                <Button
                  variant="filter"
                  size="toolbar"
                  disabled={locked}
                  onClick={clear}
                >
                  清除筛选
                </Button>
              )}
            </ToolbarGroup>
          </CollectionFilterBar>
        )}
        <TabsContent value={view} className="min-w-0">
          {!authenticated ? (
            <ContentState
              icon={null}
              className="asset-guest-empty"
              title="登录查看你的资产"
              actions={<Button onClick={login}>登录 / 注册</Button>}
            />
          ) : view === "subjects" ? (
            session && space.selected ? (
              <UnifiedSubjectLibrary
                actorID={session.actor.id}
                scopeID={space.selected.id}
                query={filters.search}
                embedded
                toolbarInHeader
                onLockChange={setLocked}
                onBatchMode={setBatch}
              />
            ) : (
              <ContentState title="请选择空间" />
            )
          ) : view === "motion" || view === "marketing" ? (
            <ContentState
              variant="workspace"
              title="暂无相关资产"
              description={`${view === "motion" ? "动效" : "营销"}生成暂未开放`}
              actions={
                view === "motion" ? (
                  <Button asChild variant="outline">
                    <Link href="/motion-studio">进入动效工作室</Link>
                  </Button>
                ) : undefined
              }
            />
          ) : view === "history" ? (
            <PersonalHistoryList
              key={JSON.stringify([kind, filters])}
              kind={kind}
              filters={filters}
              onClear={clear}
              onLock={setLocked}
              onBatchMode={setBatch}
            />
          ) : (
            <AssetCanvasList
              key={JSON.stringify([view, collection, filters])}
              kind={view}
              collection={view === "short_drama" ? "current" : collection}
              filters={filters}
              onClear={clear}
              onLock={setLocked}
              onBatchMode={setBatch}
              toolbarInHeader
            />
          )}
        </TabsContent>
      </Tabs>
    </PageLayout>
  );
}

function TimeFilter({
  value,
  onChange,
  disabled,
}: {
  value: AssetFilters;
  onChange: (value: AssetFilters) => void;
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [from, setFrom] = useState(value.from);
  const [through, setThrough] = useState(value.through);
  const [error, setError] = useState("");
  const fromInput = useRef<HTMLInputElement>(null);
  function apply(nextFrom: string, nextThrough: string) {
    try {
      assetDateRange(nextFrom, nextThrough);
      onChange({ ...value, from: nextFrom, through: nextThrough });
      setError("");
      setOpen(false);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "请选择有效日期。");
      fromInput.current?.focus();
    }
  }
  function preset(months: number, days = 0) {
    const end = new Date();
    const start = new Date(end.getFullYear(), end.getMonth(), end.getDate());
    if (days) start.setDate(start.getDate() - days + 1);
    else {
      const day = start.getDate();
      start.setDate(1);
      start.setMonth(start.getMonth() - months);
      start.setDate(
        Math.min(
          day,
          new Date(start.getFullYear(), start.getMonth() + 1, 0).getDate(),
        ),
      );
    }
    const format = (date: Date) =>
      `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
    setFrom(format(start));
    setThrough(format(end));
    apply(format(start), format(end));
  }
  return (
    <>
      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          if (next) {
            setFrom(value.from);
            setThrough(value.through);
            setError("");
          }
        }}
      >
        <PopoverTrigger asChild>
          <Button
            variant="filter"
            size="toolbar"
            data-active={!!(value.from || value.through) || undefined}
            disabled={disabled}
          >
            时间
            <ChevronDown data-icon="inline-end" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          collisionPadding={16}
          className="max-h-[var(--radix-popover-content-available-height)] w-80 max-w-[calc(100vw-2rem)] overflow-y-auto"
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              apply(from, through);
            }}
            className="flex flex-col gap-4"
          >
            <FieldGroup>
              <Field data-invalid={!!error}>
                <FieldLabel htmlFor="asset-from">开始日期</FieldLabel>
                <Input
                  id="asset-from"
                  ref={fromInput}
                  name="from"
                  autoComplete="off"
                  type="date"
                  aria-invalid={!!error}
                  aria-describedby={error ? "asset-date-error" : undefined}
                  value={from}
                  onChange={(event) => setFrom(event.target.value)}
                />
                {error && (
                  <FieldError id="asset-date-error">{error}</FieldError>
                )}
              </Field>
              <Field data-invalid={!!error}>
                <FieldLabel htmlFor="asset-through">结束日期</FieldLabel>
                <Input
                  id="asset-through"
                  name="through"
                  autoComplete="off"
                  type="date"
                  aria-invalid={!!error}
                  aria-describedby={error ? "asset-date-error" : undefined}
                  value={through}
                  onChange={(event) => setThrough(event.target.value)}
                />
              </Field>
            </FieldGroup>
            <div className="flex flex-wrap gap-1">
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  setFrom("");
                  setThrough("");
                  apply("", "");
                }}
              >
                全部
              </Button>
              <Button
                type="button"
                variant="ghost"
                onClick={() => preset(0, 7)}
              >
                最近一周
              </Button>
              <Button type="button" variant="ghost" onClick={() => preset(1)}>
                最近一个月
              </Button>
              <Button type="button" variant="ghost" onClick={() => preset(3)}>
                最近三个月
              </Button>
            </div>
            <Button type="submit">应用时间</Button>
          </form>
        </PopoverContent>
      </Popover>
    </>
  );
}
