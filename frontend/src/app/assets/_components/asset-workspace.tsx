"use client";

import { useMemo, useState } from "react";
import dynamic from "next/dynamic";
import { useRouter } from "next/navigation";
import { ChevronDown, PanelsTopLeft } from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { PageLayout } from "@/components/layout/page-layout";
import { PageHeading } from "@/components/layout/page-heading";
import { ContentState } from "@/components/layout/content-state";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
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
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import { readErrorMessage } from "@/lib/read-error";
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
}: {
  initialView?: AssetSource | "reference";
}) {
  const session = useOptionalSession();
  const space = useSpace();
  return (
    <WorkspaceScope
      key={`${session?.actor.id ?? "guest"}:${space.selected?.id ?? "none"}`}
      initialView={initialView}
      authenticated={!!session}
    />
  );
}

function WorkspaceScope({
  initialView,
  authenticated,
}: {
  initialView: AssetSource | "reference";
  authenticated: boolean;
}) {
  const router = useRouter();
  const [view, setView] = useState(initialView);
  const [kind, setKind] = useState<AssetKind>("image");
  const [search, setSearch] = useState("");
  const [filters, setFilters] = useState(defaultFilters);
  const login = useLoginDialog();
  const [locked, setLocked] = useState(false);
  const [batch, setBatch] = useState(false);
  const [toolbarTarget, setToolbarTarget] = useState<HTMLDivElement | null>(
    null,
  );
  const [collection, setCollection] = useState<"current" | "legacy">("current");
  function changeView(value: AssetSource | "reference") {
    setView(value);
    setSearch("");
    setFilters(defaultFilters);
    router.push(value === "history" ? "/assets" : `/assets?view=${value}`);
  }
  const pageSearch = useMemo(
    () => ({
      value: search,
      onValueChange: setSearch,
      onSearch: (value: string) => {
        if (!locked)
          setFilters((current) => ({ ...current, search: value.trim() }));
      },
      label:
        view === "history"
          ? "搜索本人生成历史"
          : view === "short_drama"
            ? "搜索短剧"
            : "搜索画布",
      disabled: !authenticated || locked,
    }),
    [search, view, authenticated, locked],
  );
  if (view === "reference")
    return <ReferenceAssetsPage onBack={() => changeView("history")} />;
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
      <PageHeading
        title="资产"
        actions={
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            <div ref={setToolbarTarget} className="min-w-0" />
            {!batch && (
              <Button
                variant="ghost"
                disabled={locked}
                onClick={() => changeView("reference")}
              >
                参考素材
              </Button>
            )}
          </div>
        }
      />
      <Tabs
        value={view}
        onValueChange={(value) => changeView(value as AssetSource)}
        className="gap-6"
      >
        <TabsList
          variant="navigation"
          aria-label="资产来源"
          className="max-w-full"
        >
          {sources.map((source) => (
            <TabsTrigger
              key={source.id}
              value={source.id}
              className="min-h-11"
              disabled={locked}
            >
              {source.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <div className="flex min-w-0 flex-wrap items-center gap-x-6 gap-y-3">
          {view === "history" && (
            <ToggleGroup
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
                  className="min-h-11 px-3"
                  disabled={locked}
                >
                  {item.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          )}
          {view === "canvas" && (
            <ToggleGroup
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
          <div className="flex flex-wrap items-center gap-2">
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
                  variant="ghost"
                  disabled={!authenticated || locked}
                  className="min-h-11"
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
                variant="ghost"
                className="min-h-11"
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
  function apply(nextFrom: string, nextThrough: string) {
    try {
      assetDateRange(nextFrom, nextThrough);
      onChange({ ...value, from: nextFrom, through: nextThrough });
      setError("");
      setOpen(false);
    } catch (reason) {
      setError(readErrorMessage(reason));
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
      {error && <FeedbackToast message={error} />}
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
            variant={value.from || value.through ? "secondary" : "ghost"}
            disabled={disabled}
            className="min-h-11"
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
            className="grid gap-4"
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="asset-from">开始日期</FieldLabel>
                <Input
                  id="asset-from"
                  type="date"
                  value={from}
                  onChange={(event) => setFrom(event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="asset-through">结束日期</FieldLabel>
                <Input
                  id="asset-through"
                  type="date"
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
