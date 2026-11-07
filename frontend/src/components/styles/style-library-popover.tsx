"use client";
import { useEffect, useRef, useState, type ReactElement } from "react";
import dynamic from "next/dynamic";
import {
  Check,
  ChevronLeft,
  ChevronRight,
  ImageIcon,
  Pencil,
  Plus,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { ContentState } from "@/components/layout/content-state";
import { PersonalMediaPreview } from "@/components/media/personal-reference-picker";
import { RequestError } from "@/lib/request";
import {
  deleteStyle,
  freezeStyle,
  listStyles,
  readStyle,
  styleError,
  type StyleCategory,
  type StyleDetail,
  type StylePage,
  type StyleQuery,
  type StyleSummary,
} from "./style-api";
export type { StyleDetail } from "./style-api";
const Editor = dynamic(() =>
  import("./style-editor-dialog").then((module) => module.StyleEditorDialog),
);
const tabs = [
  { id: "official", label: "官方" },
  { id: "mine", label: "我的风格" },
  { id: "realistic", label: "真人" },
  { id: "2d", label: "2D" },
  { id: "3d", label: "3D" },
] as const;
export type StyleOwner = { actorId: string; scopeId: string };
export type StyleLibraryPopoverProps = {
  value: Readonly<StyleDetail> | null;
  onChange: (value: Readonly<StyleDetail> | null) => void;
  disabled?: boolean;
  owner: StyleOwner;
  children?: ReactElement;
};
export function StyleLibraryPopover(props: StyleLibraryPopoverProps) {
  return (
    <ScopedStyleLibrary
      key={`${props.owner.actorId}/${props.owner.scopeId}`}
      {...props}
    />
  );
}
function ScopedStyleLibrary({
  value,
  onChange,
  owner,
  disabled,
  children,
}: StyleLibraryPopoverProps) {
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState("official");
  const [search, setSearch] = useState("");
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [index, setIndex] = useState(0);
  const [reload, setReload] = useState(0);
  const [page, setPage] = useState<StylePage | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [selecting, setSelecting] = useState(false);
  const [editor, setEditor] = useState<Readonly<StyleDetail> | "new" | null>(
    null,
  );
  const [deletion, setDeletion] = useState<{
    style: StyleSummary;
    key: string;
  } | null>(null);
  const [deleting, setDeleting] = useState(false);
  const flight = useRef<AbortController | null>(null);
  const busy = useRef(false);
  useEffect(() => () => flight.current?.abort(), []);
  const cursor = cursors[index];
  useEffect(() => {
    if (!open || disabled) return;
    const controller = new AbortController();
    const timer = setTimeout(() => {
      setLoading(true);
      setPage(null);
      setError("");
      const query: StyleQuery = {
        limit: 24,
        ...(search.trim() ? { q: search.trim() } : {}),
        ...(cursor ? { cursor } : {}),
        ...(tab === "official"
          ? { mine: false }
          : tab === "mine"
            ? { mine: true }
            : { category: tab as StyleCategory }),
      };
      void listStyles(query, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted) setPage(result);
        })
        .catch((cause) => {
          if (!controller.signal.aborted) setError(styleError(cause));
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    }, 150);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [open, disabled, tab, search, cursor, reload]);
  function filter(nextTab: string, nextSearch: string) {
    if (deletion) return;
    setTab(nextTab);
    setSearch(nextSearch);
    setCursors([undefined]);
    setIndex(0);
    setPage(null);
  }
  async function choose(item: StyleSummary, edit = false) {
    if (
      busy.current ||
      disabled ||
      deletion ||
      (!edit && !item.references_available)
    )
      return;
    const controller = new AbortController();
    flight.current = controller;
    busy.current = true;
    setSelecting(true);
    setError("");
    try {
      const detail = await readStyle(item.id, controller.signal);
      if (controller.signal.aborted) return;
      if (edit) {
        setEditor(detail);
        setOpen(false);
      } else {
        onChange(freezeStyle(detail));
        setOpen(false);
      }
    } catch (cause) {
      if (!controller.signal.aborted) setError(styleError(cause));
    } finally {
      if (!controller.signal.aborted) {
        busy.current = false;
        setSelecting(false);
      }
    }
  }
  async function remove(item?: StyleSummary) {
    if (busy.current || disabled) return;
    const command =
      deletion ?? (item ? { style: item, key: crypto.randomUUID() } : null);
    if (!command || !command.style.owned || !command.style.can_edit) return;
    setDeletion(command);
    busy.current = true;
    setDeleting(true);
    setError("");
    const controller = new AbortController();
    flight.current = controller;
    try {
      await deleteStyle(
        command.style.id,
        command.style.revision,
        command.key,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      setDeletion(null);
      setReload((n) => n + 1);
    } catch (cause) {
      if (!controller.signal.aborted) {
        setError(styleError(cause));
        if (
          cause instanceof RequestError &&
          cause.status >= 400 &&
          cause.status < 500
        )
          setDeletion(null);
      }
    } finally {
      if (!controller.signal.aborted) {
        busy.current = false;
        setDeleting(false);
      }
    }
  }
  function clear() {
    if (!disabled && !deletion) {
      onChange(null);
      setOpen(false);
    }
  }
  return (
    <>
      <Popover
        open={open}
        onOpenChange={(next) => {
          if (!disabled && !deletion) {
            if (!next) {
              flight.current?.abort();
              busy.current = false;
              setSelecting(false);
            }
            setOpen(next);
          }
        }}
      >
        <PopoverTrigger asChild>
          {children ?? (
            <Button
              variant="ghost"
              size="toolbar"
              disabled={disabled}
              aria-label="选择风格"
            >
              <ImageIcon data-icon="inline-start" />
              {value?.name ?? "风格"}
            </Button>
          )}
        </PopoverTrigger>
        <PopoverContent
          align="start"
          side="top"
          collisionPadding={16}
          className="flex max-h-[min(80dvh,var(--radix-popover-content-available-height))] w-[640px] max-w-[calc(100vw-32px)] flex-col gap-3 overflow-y-auto p-4"
          aria-label="风格库"
        >
          <PopoverHeader className="flex-row items-center justify-between gap-2">
            <PopoverTitle>风格库</PopoverTitle>
            <div className="flex gap-1">
              {value ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={clear}
                  disabled={selecting || !!deletion}
                >
                  <X data-icon="inline-start" />
                  清除风格
                </Button>
              ) : null}
              <Button
                variant="outline"
                size="sm"
                disabled={selecting || !!deletion}
                onClick={() => {
                  setOpen(false);
                  setEditor("new");
                }}
              >
                <Plus data-icon="inline-start" />
                创建风格
              </Button>
            </div>
          </PopoverHeader>
          <Tabs value={tab} onValueChange={(next) => filter(next, search)}>
            <div className="overflow-x-auto">
              <TabsList variant="navigation" aria-label="风格分类">
                {tabs.map((item) => (
                  <TabsTrigger
                    key={item.id}
                    value={item.id}
                    disabled={!!deletion || selecting}
                  >
                    {item.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>
          </Tabs>
          <InputGroup>
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput
              aria-label="搜索风格"
              placeholder="搜索风格"
              value={search}
              disabled={!!deletion || selecting}
              onChange={(event) => filter(tab, event.target.value)}
            />
          </InputGroup>
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {deletion ? (
            <Button
              variant="outline"
              disabled={deleting}
              onClick={() => void remove()}
            >
              {deleting ? "正在删除…" : "重试原删除"}
            </Button>
          ) : null}
          {loading ? (
            <div
              className="grid grid-cols-4 gap-3"
              role="status"
              aria-label="正在读取风格"
            >
              {Array.from({ length: 8 }, (_, n) => (
                <Skeleton key={n} className="aspect-square" />
              ))}
            </div>
          ) : (
            <div className="grid grid-cols-4 gap-3">
              <Button
                variant="outline"
                className="aspect-square min-w-0 flex-col gap-2"
                aria-label="不使用风格"
                aria-pressed={!value}
                onClick={clear}
                disabled={selecting || !!deletion}
              >
                无{!value ? <Check /> : null}
              </Button>
              {page?.items.map((item) => (
                <div key={item.id} className="flex min-w-0 flex-col gap-1">
                  <Button
                    variant="outline"
                    className="h-auto min-w-0 flex-col gap-2 p-1"
                    aria-label={`选择${item.name}`}
                    aria-pressed={value?.id === item.id}
                    disabled={
                      selecting || !!deletion || !item.references_available
                    }
                    onClick={() => void choose(item)}
                  >
                    <span className="relative aspect-square w-full overflow-hidden rounded-md">
                      {item.cover_item_id && item.cover_asset_id ? (
                        <PersonalMediaPreview
                          itemID={item.cover_item_id}
                          assetID={item.cover_asset_id}
                          title={item.name}
                        />
                      ) : (
                        <span className="flex size-full items-center justify-center">
                          <ImageIcon />
                        </span>
                      )}
                    </span>
                    <span className="flex w-full min-w-0 items-center justify-center gap-1">
                      <span className="truncate">{item.name}</span>
                      {value?.id === item.id ? <Check /> : null}
                    </span>
                  </Button>
                  {!item.references_available ? (
                    <span className="text-xs text-muted-foreground">
                      参考图不可用
                    </span>
                  ) : null}
                  {item.owned && item.can_edit ? (
                    <div className="flex justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        aria-label={`编辑${item.name}`}
                        disabled={selecting || !!deletion}
                        onClick={() => void choose(item, true)}
                      >
                        <Pencil />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        aria-label={`删除${item.name}`}
                        disabled={selecting || !!deletion}
                        onClick={() => void remove(item)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          )}
          {!loading && page?.items.length === 0 ? (
            <ContentState
              title="没有匹配的风格"
              description={
                tab === "mine"
                  ? "创建自己的风格，或调整搜索条件。"
                  : "试试其他分类或关键词。"
              }
            />
          ) : null}
          {!loading && !page && error ? (
            <Button variant="outline" onClick={() => setReload((n) => n + 1)}>
              重新读取风格
            </Button>
          ) : null}
          {page ? (
            <div
              className="flex items-center justify-end gap-2"
              role="navigation"
              aria-label="风格分页"
            >
              <Button
                variant="outline"
                size="icon"
                aria-label="上一页风格"
                disabled={index === 0 || !!deletion}
                onClick={() => {
                  setPage(null);
                  setIndex((n) => n - 1);
                }}
              >
                <ChevronLeft />
              </Button>
              <span className="text-xs text-muted-foreground">
                第 {index + 1} 页
              </span>
              <Button
                variant="outline"
                size="icon"
                aria-label="下一页风格"
                disabled={!page.next_cursor || !!deletion}
                onClick={() => {
                  if (!page.next_cursor) return;
                  if (cursors.slice(0, index + 1).includes(page.next_cursor)) {
                    setError("风格分页回执重复，请重新读取。");
                    return;
                  }
                  setCursors((items) => [
                    ...items.slice(0, index + 1),
                    page.next_cursor!,
                  ]);
                  setIndex((n) => n + 1);
                  setPage(null);
                }}
              >
                <ChevronRight />
              </Button>
            </div>
          ) : null}
        </PopoverContent>
      </Popover>
      {editor ? (
        <Editor
          key={editor === "new" ? "new" : `${editor.id}/${editor.revision}`}
          owner={owner}
          value={editor === "new" ? undefined : editor}
          onClose={() => {
            setEditor(null);
            setOpen(true);
          }}
          onSaved={(saved) => {
            if (editor === "new") {
              onChange(freezeStyle(saved));
              setEditor(null);
              setOpen(false);
              return;
            }
            setEditor(null);
            setTab("mine");
            setCursors([undefined]);
            setIndex(0);
            setReload((n) => n + 1);
            setOpen(true);
          }}
        />
      ) : null}
    </>
  );
}
