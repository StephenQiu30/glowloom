"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";

import { useEffect, useRef, useState } from "react";
import {
  CalendarDays,
  ListFilter,
  AudioLines,
  Box,
  Check,
  FolderOpen,
  ImageIcon,
  Search,
  Upload,
  VideoIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@/components/ui/popover";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { getMediaPreview, listMediaAssets } from "@/api/media";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { UploadDialog } from "@/components/media/upload-dialog";
import { SubjectList } from "@/components/subjects/subject-list";
import { subjectText, type Subject } from "@/components/subjects/contracts";
import {
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import { flowError } from "@/components/generation/contracts";
import "./project-asset-dialog.css";
import { isProjectID } from "@/components/projects/project";

const mediaKinds = [
  { value: "image", label: "图片", icon: ImageIcon },
  { value: "video", label: "视频", icon: VideoIcon },
  { value: "audio", label: "音频", icon: AudioLines },
  { value: "model", label: "3D", icon: Box },
];

export function ProjectAssetDialog({
  projectID,
  initialTab = "assets",
  initialKind = "image",
  onClose,
  onAdd,
  onSubject,
  reference = false,
}: {
  projectID: string;
  initialTab?: "assets" | "subjects";
  initialKind?: string;
  onClose: () => void;
  onAdd: (assets: API.AssetSummary[]) => void | Promise<void>;
  reference?: boolean;
  onSubject?: (subject: Subject) => void;
}) {
  const limit = reference ? 1 : 24;
  const [query, setQuery] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [confirmError, setConfirmError] = useState("");
  const confirmLock = useRef(false);
  const confirmRequest = useRef<AbortController | null>(null);
  useEffect(() => () => confirmRequest.current?.abort(), []);
  const [tab, setTab] = useState(initialTab);
  const [kind, setKind] = useState(initialKind);
  const [subjectKind, setSubjectKind] = useState<API.Kind>("character");
  const [selected, setSelected] = useState<API.AssetSummary[]>([]);
  const [subject, setSubject] = useState<Subject | null>(null);
  const [upload, setUpload] = useState(false);
  const [revision, setRevision] = useState(0);
  function toggle(asset: API.AssetSummary) {
    setSelected((items) =>
      items.some((i) => i.id === asset.id)
        ? items.filter((i) => i.id !== asset.id)
        : reference
          ? [asset]
          : items.length < limit
            ? [...items, asset]
            : items,
    );
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !confirmLock.current) onClose();
        }}
      >
        <DialogContent
          showCloseButton={!confirming}
          className="canvas-material-dialog sm:max-w-[800px]"
        >
          <DialogHeader className="sr-only">
            <DialogTitle>{reference ? "选择参考素材" : "资产库"}</DialogTitle>
            <DialogDescription>
              从当前项目添加素材，或选择主体开始创作。
            </DialogDescription>
          </DialogHeader>
          <Tabs
            value={tab}
            onValueChange={(value) => setTab(value as typeof tab)}
            className="min-h-0 flex-1"
            inert={confirming}
          >
            <TabsList>
              <TabsTrigger value="assets">资产</TabsTrigger>
              {onSubject && <TabsTrigger value="subjects">主体</TabsTrigger>}
            </TabsList>
            <TabsContent value="assets" className="flex min-h-0 flex-col gap-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <ToggleGroup
                  type="single"
                  value={kind}
                  onValueChange={(v) => {
                    if (v) setKind(v);
                  }}
                  aria-label="素材类型"
                >
                  {mediaKinds
                    .filter((k) => !reference || k.value === initialKind)
                    .map(({ value, label, icon: Icon }) => (
                      <ToggleGroupItem key={value} value={value}>
                        <Icon data-icon="inline-start" />
                        {label}
                      </ToggleGroupItem>
                    ))}
                </ToggleGroup>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setUpload(true)}
                >
                  <Upload data-icon="inline-start" />
                  上传素材
                </Button>
              </div>
              <div className="flex items-center gap-2">
                <InputGroup>
                  <InputGroupAddon>
                    <Search />
                  </InputGroupAddon>
                  <InputGroupInput
                    aria-label="搜索项目素材"
                    placeholder="搜索项目素材"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                  />
                  {query && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setQuery("")}
                    >
                      清除搜索
                    </Button>
                  )}
                </InputGroup>
                {[
                  { icon: CalendarDays, label: "时间筛选" },
                  { icon: ListFilter, label: "来源筛选" },
                ].map(({ icon: Icon, label }) => (
                  <Popover key={label}>
                    <PopoverTrigger asChild>
                      <Button variant="ghost" size="icon" aria-label={label}>
                        <Icon />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent align="end">
                      <p className="font-medium">{label}暂不可用</p>
                      <p className="text-muted-foreground">
                        素材服务尚未提供对应信息，可先按类型或名称查找。
                      </p>
                    </PopoverContent>
                  </Popover>
                ))}
              </div>
              <MaterialList
                key={projectID + kind + revision + query}
                projectID={projectID}
                kind={kind}
                selected={selected}
                onToggle={toggle}
                query={query}
                limit={limit}
                onSelectPage={(items) =>
                  setSelected((previous) =>
                    [
                      ...new Map(
                        [...previous, ...items].map((a) => [a.id, a]),
                      ).values(),
                    ].slice(0, limit),
                  )
                }
              />
            </TabsContent>
            <TabsContent
              value="subjects"
              className="min-h-0 overflow-auto"
              data-canvas-wheel-scroll
            >
              <ToggleGroup
                type="single"
                value={subjectKind}
                onValueChange={(v) => {
                  if (v) {
                    setSubjectKind(v as API.Kind);
                    setSubject(null);
                  }
                }}
                aria-label="主体类型"
              >
                <ToggleGroupItem value="character">角色</ToggleGroupItem>
                <ToggleGroupItem value="scene">场景</ToggleGroupItem>
                <ToggleGroupItem value="prop">物品</ToggleGroupItem>
              </ToggleGroup>
              <SubjectList
                key={projectID + subjectKind}
                projectID={projectID}
                kind={subjectKind}
                onSelect={setSubject}
                selectedID={subject?.head.id}
              />
            </TabsContent>
          </Tabs>
          {confirmError && (
            <p role="alert" className="text-sm text-destructive">
              {confirmError}
            </p>
          )}
          <DialogFooter className="canvas-material-footer">
            <p
              className="mr-auto text-xs text-muted-foreground"
              aria-live="polite"
            >
              {tab === "assets"
                ? `已选择 ${selected.length} 个素材（最多 ${limit} 个）`
                : subject
                  ? `已选择：${subjectText(subject).name ?? "未命名主体"}`
                  : "请选择一个主体"}
            </p>
            {tab === "assets" && selected.length > 0 && (
              <Button
                variant="ghost"
                disabled={confirming}
                onClick={() => setSelected([])}
              >
                清空选择
              </Button>
            )}
            <Button variant="outline" disabled={confirming} onClick={onClose}>
              取消
            </Button>
            <Button
              disabled={
                confirming || (tab === "assets" ? !selected.length : !subject)
              }
              onClick={async () => {
                if (confirmLock.current) return;
                confirmLock.current = true;
                setConfirming(true);
                setConfirmError("");
                const controller = new AbortController();
                confirmRequest.current = controller;
                const scope = getScopeGeneration(),
                  session = getSessionGeneration();
                try {
                  if (tab === "assets") {
                    const verified = await Promise.all(
                      selected.map(async (asset) => {
                        const result = await getMediaPreview(
                          { pid: projectID, asset_id: asset.id! },
                          { signal: controller.signal },
                        );
                        if (
                          !result.asset ||
                          result.asset.id !== asset.id ||
                          result.asset.project_id !== projectID ||
                          result.asset.kind !== asset.kind ||
                          !Number.isSafeInteger(result.asset.revision) ||
                          result.asset.revision! < 1 ||
                          result.asset.revision !== asset.revision
                        )
                          throw new Error("素材权限或版本已变化，请重新选择。");
                        return result.asset;
                      }),
                    );
                    if (
                      controller.signal.aborted ||
                      scope !== getScopeGeneration() ||
                      session !== getSessionGeneration()
                    )
                      return;
                    await onAdd(verified);
                  } else if (subject) onSubject?.(subject);
                } catch (error) {
                  if (!controller.signal.aborted)
                    setConfirmError(
                      error instanceof Error
                        ? error.message
                        : "素材暂不可用，请重试。",
                    );
                } finally {
                  confirmLock.current = false;
                  if (!controller.signal.aborted) setConfirming(false);
                }
              }}
            >
              {confirming
                ? "正在核验…"
                : tab === "assets"
                  ? reference
                    ? "添加为参考"
                    : "添加到画布"
                  : "用主体创作"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {upload && (
        <UploadDialog
          projectID={projectID}
          onClose={() => setUpload(false)}
          onUploaded={(asset) => {
            setUpload(false);
            setRevision((v) => v + 1);
            if (
              asset &&
              ["image", "video", "audio", "model"].includes(asset.kind ?? "")
            ) {
              if (reference && asset.kind !== initialKind) {
                setConfirmError(
                  "素材已上传到项目，但类型与当前参考不符，请选择对应类型的素材。",
                );
                return;
              }
              setConfirmError("");
              setQuery("");
              setKind(asset.kind!);
              setSelected((items) =>
                reference
                  ? [asset]
                  : items.length < limit &&
                      !items.some((i) => i.id === asset.id)
                    ? [...items, asset]
                    : items,
              );
            }
          }}
        />
      )}
    </>
  );
}

function MaterialList({
  projectID,
  kind,
  selected,
  onToggle,
  query,
  onSelectPage,
  limit,
}: {
  projectID: string;
  kind: string;
  selected: API.AssetSummary[];
  onToggle: (asset: API.AssetSummary) => void;
  query: string;
  limit: number;
  onSelectPage: (assets: API.AssetSummary[]) => void;
}) {
  const [items, setItems] = useState<API.AssetSummary[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const request = useRef<AbortController | null>(null);
  const busy = useRef(false);
  function validate(page: API.AssetPage) {
    if (
      !Array.isArray(page.items) ||
      page.items.some(
        (a) =>
          !isProjectID(a.id) || a.project_id !== projectID || a.kind !== kind,
      ) ||
      (page.next_cursor != null && typeof page.next_cursor !== "string")
    )
      throw new RequestError("invalid_receipt");
    return { ...page, next_cursor: page.next_cursor ?? undefined };
  }
  useEffect(() => {
    const controller = new AbortController();
    request.current = controller;
    async function read() {
      const page = validate(
        await listMediaAssets(
          { pid: projectID, kind, limit: 40 },
          { signal: controller.signal },
        ),
      );
      if (!query.trim()) return page;
      const all = [...page.items!];
      let next = page.next_cursor;
      const seen = new Set<string>();
      while (next && !controller.signal.aborted) {
        if (seen.has(next)) throw new RequestError("invalid_receipt");
        seen.add(next);
        const more = validate(
          await listMediaAssets(
            { pid: projectID, kind, limit: 200, cursor: next },
            { signal: controller.signal },
          ),
        );
        all.push(...more.items!);
        next = more.next_cursor;
      }
      return {
        items: [...new Map(all.map((a) => [a.id, a])).values()],
        next_cursor: undefined,
      };
    }
    const timer = setTimeout(
      () => {
        void read()
          .then((page) => {
            if (controller.signal.aborted) return;
            setItems(page.items!);
            setCursor(page.next_cursor);
            setError("");
          })
          .catch((e) => {
            if (!controller.signal.aborted) setError(flowError(e));
          })
          .finally(() => {
            if (!controller.signal.aborted) setLoading(false);
          });
      },
      query ? 250 : 0,
    );
    return () => {
      clearTimeout(timer);
      controller.abort();
      request.current?.abort();
    };
  }, [projectID, kind, retry, query]); // eslint-disable-line react-hooks/exhaustive-deps
  async function more() {
    if (busy.current || loading || !cursor) return;
    busy.current = true;
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError("");
    try {
      const page = validate(
        await listMediaAssets(
          { pid: projectID, kind, limit: 40, cursor },
          { signal: controller.signal },
        ),
      );
      if (controller.signal.aborted) return;
      if (page.next_cursor === cursor)
        throw new RequestError("invalid_receipt");
      setItems((previous) => [
        ...new Map(
          [...previous, ...page.items!].map((a) => [a.id, a]),
        ).values(),
      ]);
      setCursor(page.next_cursor);
    } catch (e) {
      if (!controller.signal.aborted) setError(flowError(e));
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setLoading(false);
    }
  }
  const filtered = items.filter((a) =>
    (a.file_name ?? "")
      .toLocaleLowerCase()
      .includes(query.trim().toLocaleLowerCase()),
  );
  const Icon = mediaKinds.find((k) => k.value === kind)!.icon;
  return (
    <>
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span>
          {loading
            ? query
              ? "正在搜索当前项目…"
              : "正在读取素材…"
            : `${filtered.length} 个结果`}
        </span>
        {limit > 1 && (
          <Button
            variant="ghost"
            size="sm"
            disabled={loading || !filtered.length || selected.length >= limit}
            onClick={() => onSelectPage(filtered)}
          >
            选择本页
          </Button>
        )}
      </div>
      <div className="canvas-material-content" data-canvas-wheel-scroll>
        {error && (
          <FeedbackToast
            message={error}
            action={{
              label: "重新读取",
              onClick: () => {
                setError("");
                setLoading(true);
                setRetry((v) => v + 1);
              },
            }}
          />
        )}
        <div className="canvas-material-grid">
          {filtered.map((a) => {
            const chosen = selected.some((i) => i.id === a.id);
            return (
              <Button
                variant="ghost"
                key={a.id}
                type="button"
                className="canvas-material-card h-auto p-0 whitespace-normal"
                aria-label={`选择素材 ${a.file_name || "未命名素材"}`}
                aria-pressed={chosen}
                disabled={limit > 1 && !chosen && selected.length >= limit}
                onClick={() => onToggle(a)}
              >
                <Card variant="media" className="w-full">
                  <CardContent>
                    <div className="canvas-material-preview">
                      {kind === "image" || kind === "video" ? (
                        <ProjectMediaPreview
                          projectID={projectID}
                          id={a.id!}
                          kind={kind}
                        />
                      ) : (
                        <Icon />
                      )}
                      {chosen && (
                        <span className="canvas-material-check">
                          <Check />
                        </span>
                      )}
                    </div>
                  </CardContent>
                  <CardHeader>
                    <CardTitle>{a.file_name || "未命名素材"}</CardTitle>
                    <CardDescription>
                      {kind === "image" || kind === "video"
                        ? `${a.width ?? "—"} × ${a.height ?? "—"}`
                        : kind === "model"
                          ? "GLB 模型"
                          : "音频素材"}
                    </CardDescription>
                  </CardHeader>
                </Card>
              </Button>
            );
          })}
          {loading &&
            !items.length &&
            Array.from({ length: 8 }, (_, i) => (
              <Skeleton key={i} className="h-40 w-full" />
            ))}
        </div>
        {!loading && !error && !filtered.length && (
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FolderOpen />
              </EmptyMedia>
              <EmptyTitle>
                {query ? "没有匹配的素材" : "当前项目暂无此类素材"}
              </EmptyTitle>
              <EmptyDescription>
                {query
                  ? "换个关键词，或清除搜索查看全部素材。"
                  : "上传素材后，即可添加到画布。"}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
        {cursor && (
          <Button
            variant="ghost"
            disabled={loading}
            onClick={() => void more()}
          >
            {loading && <Spinner data-icon="inline-start" />}加载更多
          </Button>
        )}
      </div>
    </>
  );
}
