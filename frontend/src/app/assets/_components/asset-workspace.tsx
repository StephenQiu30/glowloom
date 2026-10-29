"use client";

import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import dynamic from "next/dynamic";
import { ChevronDown, PanelsTopLeft } from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { PageLayout } from "@/components/layout/page-layout";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { ContentState } from "@/components/layout/content-state";
import { Separator } from "@/components/ui/separator";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
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
    loading: () => <ContentState state="loading" title="正在读取参考素材" />,
  },
);
const sources = [
  { id: "history", label: "生成历史" },
  { id: "canvas", label: "画布" },
  { id: "short_drama", label: "短剧" },
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
  const [toolbarTarget, setToolbarTarget] = useState<HTMLDivElement | null>(
    null,
  );
  const [collection, setCollection] = useState<"current" | "legacy">(
    initialLocation?.collection ?? "current",
  );
  const restoreLocation = useEffectEvent(() => {
    const restored = readAssetLocation(
      new URLSearchParams(window.location.search),
    );
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
    setView(value);
    setKind("image");
    setCollection("current");
    setSearch("");
    setFilters(defaultFilters);
    setReferenceFilter(undefined);
    const url = new URL(window.location.href);
    url.search = writeAssetLocation(
      {
        view: value,
        kind: "image",
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
    <PageLayout screen="assets" search={pageSearch}>
      <h1 className="sr-only">资产</h1>
      <Tabs
        value={view}
        onValueChange={(value) => changeView(value as AssetSource)}
        className="asset-workspace-tabs gap-6"
      >
        <PageToolbar inHeader>
          <div
            data-asset-toolbar
            className="flex w-full min-w-0 flex-wrap items-center gap-x-4 gap-y-2"
          >
            <TabsList
              variant="workspace"
              aria-label="资产来源"
              className="max-w-full"
            >
              {sources.map((source) => (
                <TabsTrigger
                  key={source.id}
                  value={source.id}
                  className="transition-[color,background-color,box-shadow] motion-reduce:transition-none"
                  disabled={locked}
                >
                  {source.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
        </PageToolbar>
        <PageToolbar inHeader slot="actions">
          <div
            className="flex min-w-0 flex-wrap items-center gap-1"
            data-asset-page-actions
          >
            <div ref={setToolbarTarget} className="min-w-0" />
          </div>
        </PageToolbar>
        <div
          aria-label="资产筛选"
          className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2"
        >
          {view === "history" && (
            <ToggleGroup
              variant="workspace"
              size="workspace"
              spacing={0}
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
              className="max-w-full"
            >
              {assetKinds.map((item) => (
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
              variant="workspace"
              size="workspace"
              spacing={0}
              type="single"
              value={collection}
              onValueChange={(value) => {
                if (value) setCollection(value as "current" | "legacy");
              }}
              aria-label="画布类型"
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
            <Separator
              orientation="vertical"
              className="hidden h-4 sm:block data-vertical:self-center"
            />
          )}
          <div className="flex flex-wrap items-center">
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
                  size="workspace"
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
                )}
              </DropdownMenuContent>
            </DropdownMenu>
            {filtered && (
              <Button
                variant="filter"
                size="workspace"
                disabled={locked}
                onClick={clear}
              >
                清除筛选
              </Button>
            )}
          </div>
        </div>
        <TabsContent value={view} className="min-w-0">
          {!authenticated ? (
            <ContentState
              icon={PanelsTopLeft}
              title="登录查看你的资产"
              description="登录后查看生成历史、画布与短剧。"
              actions={<Button onClick={login}>登录 / 注册</Button>}
            />
          ) : view === "history" ? (
            <PersonalHistoryList
              key={JSON.stringify([kind, filters])}
              kind={kind}
              filters={filters}
              onClear={clear}
              onLock={setLocked}
              onBatchMode={setBatch}
              toolbarTarget={toolbarTarget}
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
              toolbarTarget={toolbarTarget}
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
            size="workspace"
            data-active={!!(value.from || value.through) || undefined}
            disabled={disabled}
          >
            时间
            <ChevronDown data-icon="inline-end" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-80 max-w-[calc(100vw-2rem)]">
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
