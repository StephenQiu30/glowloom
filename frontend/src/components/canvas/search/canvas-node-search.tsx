"use client";
import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { useSearchParams } from "next/navigation";
import { useOptionalSession } from "@/components/auth/session";
import { previewLibraryMediaAsset } from "@/api/media";
import { getSubjectMedia, getSubjectVersion } from "@/api/subjects";
import { requireCanvasSubjectVersion } from "../lib/canvas-subject-source";
import {
  getCreationScope,
  getScopeGeneration,
  getSessionGeneration,
  subscribeContextChange,
} from "@/lib/request";
import {
  AudioLines,
  Box,
  ChevronRight,
  Clapperboard,
  FileText,
  Group,
  ImageIcon,
  Layers,
  Search,
  UserRound,
  Video,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { PreviewImage } from "../runtime/components/media/preview-image";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { CanvasNodeData } from "../types/canvas";
import {
  canvasNodeName,
  canvasNodeTypeLabel,
  canvasSearchType,
  canvasSearchTypes,
  searchCanvasNodes,
  type CanvasSearchType,
} from "./canvas-node-search-state";

const icons = {
  all: Layers,
  image: ImageIcon,
  video: Video,
  audio: AudioLines,
  text: FileText,
  subject: UserRound,
  timeline: Clapperboard,
  cinema_studio: Clapperboard,
  group: Group,
  stack: Layers,
  other: Box,
};

export function CanvasNodeSearch({
  nodes,
  onLocate,
  children,
}: {
  nodes: CanvasNodeData[];
  onLocate: (id: string) => void;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false),
    [query, setQuery] = useState("");
  const [type, setType] = useState<CanvasSearchType>("all");
  const inputID = useId();
  const target = useRef<string | null>(null);
  const categories = useRef<HTMLDivElement>(null);
  const { counts, results } = searchCanvasNodes(nodes, query, type);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        align="center"
        side="bottom"
        sideOffset={8}
        aria-label="搜索画布"
        className="w-[min(320px,calc(100vw-24px))] gap-2 rounded-2xl p-4"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onMouseDown={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
        onCloseAutoFocus={(event) => {
          const id = target.current;
          target.current = null;
          if (id && nodes.some((node) => node.id === id)) {
            event.preventDefault();
            onLocate(id);
          }
        }}
      >
        <Label htmlFor={inputID} className="sr-only">
          节点名称
        </Label>
        <div className="relative">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
          />
          <Input
            id={inputID}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索节点…"
            className="h-9 pr-8 pl-9"
          />
          {query && (
            <Button
              variant="ghost"
              size="icon-xs"
              className="absolute top-1 right-1"
              aria-label="清空节点搜索"
              onClick={() => setQuery("")}
            >
              <X />
            </Button>
          )}
        </div>
        <SearchPreviews>
          <Tabs
            value={type}
            onValueChange={(value) => setType(value as CanvasSearchType)}
            className="gap-2"
          >
            <div className="relative h-8">
              <div
                ref={categories}
                className="[scrollbar-width:none] overflow-x-auto overflow-y-hidden pr-6"
              >
                <TabsList
                  variant="line"
                  aria-label="节点类型筛选"
                  className="h-8! justify-start gap-4 p-0"
                >
                  {canvasSearchTypes.map((item) => (
                    <TabsTrigger
                      key={item.type}
                      value={item.type}
                      aria-label={`${item.label}，${counts[item.type]} 个节点`}
                      onFocus={(event) =>
                        event.currentTarget.scrollIntoView?.({
                          block: "nearest",
                          inline: "nearest",
                        })
                      }
                      className="h-8 flex-none gap-1 rounded-none border-0 px-0 text-xs font-normal after:bottom-0! after:h-px!"
                    >
                      {item.label}
                      {counts[item.type] > 0 && (
                        <span className="tabular-nums">
                          {counts[item.type]}
                        </span>
                      )}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </div>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label="下一组节点类型"
                className="absolute top-1 right-0 bg-popover"
                onClick={() =>
                  categories.current?.scrollBy?.({
                    left: 160,
                    behavior: "smooth",
                  })
                }
              >
                <ChevronRight />
              </Button>
            </div>
            <TabsContent
              value={type}
              aria-label="画布搜索结果"
              className="max-h-[min(340px,calc(100dvh-180px))] overflow-y-auto"
            >
              <p role="status" className="sr-only">
                {results.length} 个结果，共 {nodes.length} 个节点
              </p>
              <ul aria-label="画布搜索结果">
                {results.map((node, index) => {
                  const Icon = icons[canvasSearchType(node.type, node)];
                  return (
                    <li key={node.id}>
                      <Button
                        variant="ghost"
                        className="h-16 w-full justify-start gap-3 rounded-lg px-0 text-left"
                        aria-label={`定位${canvasNodeTypeLabel(node.type, node)}节点：${canvasNodeName(node)}`}
                        onClick={() => {
                          target.current = node.id;
                          setOpen(false);
                        }}
                      >
                        <span
                          aria-hidden="true"
                          className="relative flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
                        >
                          <span className="absolute top-0 left-1 text-[10px] tabular-nums">
                            {index + 1}
                          </span>
                          <SearchPreview
                            node={node}
                            fallback={<Icon className="size-5" />}
                          />
                        </span>
                        <span className="grid min-w-0 flex-1 gap-1">
                          <span className="truncate text-sm font-normal">
                            {canvasNodeName(node)}
                          </span>
                          <span className="text-xs font-normal text-muted-foreground">
                            {canvasNodeTypeLabel(node.type, node)}
                          </span>
                        </span>
                      </Button>
                    </li>
                  );
                })}
              </ul>
              {!results.length && (
                <p className="py-6 text-center text-sm text-muted-foreground">
                  {nodes.length
                    ? "没有找到节点，请调整名称或类型筛选。"
                    : "当前画布暂无节点。"}
                </p>
              )}
            </TabsContent>
          </Tabs>
        </SearchPreviews>
      </PopoverContent>
    </Popover>
  );
}

type Grant = { url: string; expires: number };
type Target =
  | { kind: "image" | "video"; asset: string; revision: number }
  | { kind: "subject"; config: NonNullable<API.NodeConfig["subject"]> };
/** Private, bounded requests live only as long as this open search. */
class PreviewQueue {
  controller = new AbortController();
  cache = new Map<string, Promise<Grant>>();
  urls = new Set<string>();
  waiting: (() => void)[] = [];
  running = 0;
  mounts = 0;
  constructor(
    readonly project: string,
    readonly scope: string,
  ) {}
  load(target: Target) {
    if (this.controller.signal.aborted) this.controller = new AbortController();
    const key = JSON.stringify(target),
      cached = this.cache.get(key);
    if (cached) return cached;
    const result = new Promise<Grant>((resolve, reject) => {
      this.waiting.push(() => {
        this.running++;
        this.resolve(target)
          .then(resolve, reject)
          .finally(() => {
            this.running--;
            this.drain();
          });
      });
    });
    this.cache.set(key, result);
    this.drain();
    return result;
  }
  drain() {
    while (this.running < 3 && this.waiting.length) this.waiting.shift()!();
  }
  async resolve(target: Target): Promise<Grant> {
    const signal = this.controller.signal;
    if (signal.aborted) throw new Error("cancelled");
    if (target.kind !== "subject") {
      const value = await previewLibraryMediaAsset(
        { scope: "project", project_id: this.project, item_id: target.asset },
        { signal },
      );
      if (
        value.asset?.id !== target.asset ||
        value.asset.revision !== target.revision ||
        value.asset.kind !== target.kind
      )
        throw new Error("changed_media");
      const kinds =
        target.kind === "video" ? ["poster"] : ["thumb_256", "thumb_640"];
      const rendition = kinds
        .map((kind) => value.renditions?.find((item) => item.kind === kind))
        .find(Boolean);
      const url =
        rendition?.url ?? (target.kind === "image" ? value.url : undefined);
      const expires = Date.parse(
        rendition?.expires_at ?? value.expires_at ?? "",
      );
      if (
        !url ||
        !Number.isFinite(expires) ||
        expires <= Date.now() ||
        !["https:", "http:"].includes(new URL(url).protocol)
      )
        throw new Error("expired_preview");
      return { url, expires };
    }
    const config = target.config,
      local = config.local?.primary;
    if (config.local && !local) throw new Error("empty_subject");
    if (local && !local.binding_id) {
      if (local.kind !== "image" && local.kind !== "video")
        throw new Error("no_visual");
      return this.resolve({
        kind: local.kind,
        asset: local.asset_id!,
        revision: local.asset_revision!,
      });
    }
    if (!config.subject_id || !config.version_id)
      throw new Error("empty_subject");
    const pin = {
      subject_id: config.subject_id,
      version_id: config.version_id,
    };
    const subject = requireCanvasSubjectVersion(
      await getSubjectVersion(
        { id: pin.subject_id, version: pin.version_id },
        { signal },
      ),
      this.scope,
      pin,
    );
    const media = local
      ? [subject.primary, ...subject.references].find(
          (item) =>
            item.binding_id === local.binding_id &&
            item.asset_id === local.asset_id &&
            item.asset_revision === local.asset_revision &&
            item.kind === local.kind,
        )
      : subject.primary;
    // Bound videos need an authorized poster endpoint; never download originals for search.
    if (!media || media.kind !== "image")
      throw new Error("no_authorized_poster");
    const blob = await getSubjectMedia(
      {
        id: pin.subject_id,
        version: pin.version_id,
        binding: media.binding_id,
      },
      { responseType: "blob", signal },
    );
    if (
      signal.aborted ||
      !(blob instanceof Blob) ||
      !blob.size ||
      !blob.type.startsWith("image/")
    )
      throw new Error("invalid_image");
    const url = URL.createObjectURL(blob);
    this.urls.add(url);
    return { url, expires: Date.now() + 60_000 };
  }
  retain() {
    this.mounts++;
    return () => {
      this.mounts--;
      queueMicrotask(() => {
        if (!this.mounts) this.dispose();
      });
    };
  }
  dispose() {
    this.controller.abort();
    this.urls.forEach((url) => URL.revokeObjectURL(url));
    this.urls.clear();
    this.cache.clear();
    this.waiting = [];
  }
}
const PreviewContext = createContext<PreviewQueue | null>(null);
const contextVersion = () =>
  `${getSessionGeneration()}/${getScopeGeneration()}`;
function SearchPreviews({ children }: { children: ReactNode }) {
  const session = useOptionalSession(),
    params = useSearchParams();
  const generation = useSyncExternalStore(
    subscribeContextChange,
    contextVersion,
    () => "server",
  );
  const project = params.get("project_id"),
    scope = getCreationScope();
  const identity = JSON.stringify([
    session?.session_id,
    session?.actor.id,
    scope,
    project,
    generation,
  ]);
  return session && scope && project ? (
    <SearchPreviewSession key={identity} project={project} scope={scope}>
      {children}
    </SearchPreviewSession>
  ) : (
    children
  );
}
function SearchPreviewSession({
  project,
  scope,
  children,
}: {
  project: string;
  scope: string;
  children: ReactNode;
}) {
  const [queue] = useState(() => new PreviewQueue(project, scope));
  useEffect(() => queue.retain(), [queue]);
  return (
    <PreviewContext.Provider value={queue}>{children}</PreviewContext.Provider>
  );
}
function previewTarget(node: CanvasNodeData): Target | null {
  const kind = canvasSearchType(node.type, node);
  if (kind === "subject") {
    const config = node.serviceData?.config?.subject;
    return config ? { kind, config } : null;
  }
  if (kind !== "image" && kind !== "video") return null;
  const file = node.documentData?.fileReferences?.find(
    (file) => file.purpose === "CONTENT",
  );
  const asset = file?.fileId ?? node.serviceData?.media_ref?.asset_id;
  const revision = Number(file?.version ?? node.serviceData?.content_version);
  return asset && Number.isSafeInteger(revision) && revision > 0
    ? { kind, asset, revision }
    : null;
}
function SearchPreview({
  node,
  fallback,
}: {
  node: CanvasNodeData;
  fallback: ReactNode;
}) {
  const queue = useContext(PreviewContext),
    element = useRef<HTMLSpanElement>(null);
  const [visible, setVisible] = useState(
    () => !globalThis.IntersectionObserver,
  );
  useEffect(() => {
    if (!element.current || !globalThis.IntersectionObserver) return;
    const observer = new IntersectionObserver(
      ([entry]) => setVisible(entry.isIntersecting),
      { root: element.current.closest('[role="tabpanel"]') },
    );
    observer.observe(element.current);
    return () => observer.disconnect();
  }, []);
  const target = previewTarget(node);
  return (
    <span ref={element} className="flex size-full items-center justify-center">
      {visible && queue && target ? (
        <ResolvedSearchImage
          key={JSON.stringify(target)}
          queue={queue}
          target={target}
          fallback={fallback}
        />
      ) : (
        fallback
      )}
    </span>
  );
}
function ResolvedSearchImage({
  queue,
  target,
  fallback,
}: {
  queue: PreviewQueue;
  target: Target;
  fallback: ReactNode;
}) {
  const [grant, setGrant] = useState<Grant | null>(null),
    [failed, setFailed] = useState(false),
    [loaded, setLoaded] = useState(false);
  const identity = JSON.stringify(target);
  useEffect(() => {
    let active = true;
    let expiry: ReturnType<typeof setTimeout>;
    queue
      .load(JSON.parse(identity) as Target)
      .then((value) => {
        if (!active || queue.controller.signal.aborted) return;
        const remaining = value.expires - Date.now();
        if (remaining <= 0) return;
        setGrant(value);
        expiry = setTimeout(() => setFailed(true), remaining);
      })
      .catch(() => {
        if (active) setFailed(true);
      });
    return () => {
      active = false;
      clearTimeout(expiry);
    };
  }, [queue, identity]);
  return grant && !failed && !queue.controller.signal.aborted ? (
    <>
      {!loaded && fallback}
      <PreviewImage
        src={grant.url}
        alt=""
        loading="lazy"
        className={`absolute inset-0 size-full rounded-lg object-cover ${loaded ? "" : "opacity-0"}`}
        onLoad={() => setLoaded(true)}
        onError={() => setFailed(true)}
      />
    </>
  ) : (
    fallback
  );
}
