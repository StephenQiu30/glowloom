"use client";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { getSubjectVersion } from "@/api/subjects";
import { requireCanvasSubjectVersion } from "@/components/canvas/lib/canvas-subject-source";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import {
  ProjectPickerSubjects,
  ProjectPickerCanvases,
} from "./project-picker-sources";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";

import { useEffect, useRef, useState } from "react";
import {
  AudioLines,
  Box,
  Check,
  FolderOpen,
  FileText,
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

import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { getMediaPreview, listMediaLibrary } from "@/api/media";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { UploadDialog } from "@/components/media/upload-dialog";
import { SubjectList } from "@/components/subjects/subject-list";
import { subjectText, type Subject } from "@/components/subjects/contracts";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import "./project-asset-dialog.css";
import {
  defaultProjectPickerFilter,
  projectPickerFilterForKind,
  projectPickerQuery,
  requireProjectPickerPage,
  type ProjectPickerFilter,
} from "./project-picker";
import { ProjectPickerFilters } from "./project-picker-filters";

const mediaKinds = [
  { value: "image", label: "图片", icon: ImageIcon },
  { value: "video", label: "视频", icon: VideoIcon },
  { value: "audio", label: "音频", icon: AudioLines },
  { value: "model", label: "3D", icon: Box },
  { value: "document", label: "文档", icon: FileText },
];

export function ProjectAssetDialog({
  projectID,
  initialTab = "assets",
  initialKind = "image",
  onClose,
  onAdd,
  onSubject,
  onUnifiedSubject,
  browseSources = false,
  reference = false,
  allowedKinds,
  selectionLimit,
  confirmLabel,
  canvasPresentation = false,
  initialUpload = false,
  dialogTitle,
}: {
  projectID: string;
  initialTab?: "assets" | "subjects";
  initialKind?: string;
  onClose: () => void;
  onAdd: (assets: API.AssetSummary[]) => void | Promise<void>;
  reference?: boolean;
  allowedKinds?: readonly string[];
  selectionLimit?: number;
  confirmLabel?: string;
  canvasPresentation?: boolean;
  initialUpload?: boolean;
  dialogTitle?: string;
  onSubject?: (subject: Subject) => void;
  onUnifiedSubject?: (subject: UnifiedSubject) => void | Promise<void>;
  browseSources?: boolean;
}) {
  const sessionIdentity = useOptionalSession();
  const space = useSpace();
  const [unifiedSubject, setUnifiedSubject] = useState<UnifiedSubject | null>(
    null,
  );
  const limit = reference ? 1 : (selectionLimit ?? 24);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState(defaultProjectPickerFilter);
  const [confirming, setConfirming] = useState(false);
  const [confirmError, setConfirmError] = useState("");
  const confirmLock = useRef(false);
  const confirmRequest = useRef<AbortController | null>(null);
  useEffect(() => () => confirmRequest.current?.abort(), [projectID]);
  const [tab, setTab] = useState<"assets" | "subjects" | "canvases">(
    initialTab,
  );
  const [kind, setKind] = useState(initialKind);
  const [subjectKind, setSubjectKind] = useState<API.Kind>("character");
  const [selected, setSelected] = useState<API.AssetSummary[]>([]);
  const [subject, setSubject] = useState<Subject | null>(null);
  const [upload, setUpload] = useState(initialUpload);
  const [revision, setRevision] = useState(0);
  function toggle(asset: API.AssetSummary) {
    setSelected((items) =>
      items.some((i) => i.id === asset.id)
        ? items.filter((i) => i.id !== asset.id)
        : limit === 1
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
          data-canvas-picker={canvasPresentation || undefined}
          data-canvas-no-zoom
          onMouseDown={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <DialogHeader className="sr-only">
            <DialogTitle>{dialogTitle ?? (reference ? "选择参考素材" : "参考素材")}</DialogTitle>
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
              {(onSubject || onUnifiedSubject || browseSources) && (
                <TabsTrigger value="subjects">主体</TabsTrigger>
              )}
              {browseSources && (
                <TabsTrigger value="canvases">画布</TabsTrigger>
              )}
            </TabsList>
            <TabsContent value="assets" className="flex min-h-0 flex-col gap-4">
              <div className="canvas-material-filters flex flex-col gap-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <ToggleGroup
                    type="single"
                    value={kind}
                    onValueChange={(v) => {
                      if (v) {
                        setKind(v);
                        setFilter((previous) =>
                          projectPickerFilterForKind(previous, v),
                        );
                      }
                    }}
                    aria-label="素材类型"
                  >
                    {mediaKinds
                      .filter(
                        (k) =>
                          (!reference || k.value === initialKind) &&
                          (!allowedKinds || allowedKinds.includes(k.value)),
                      )
                      .map(({ value, label, icon: Icon }) => (
                        <ToggleGroupItem key={value} value={value}>
                          {!canvasPresentation && (
                            <Icon data-icon="inline-start" />
                          )}
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
                  <ProjectPickerFilters
                    kind={kind}
                    value={filter}
                    onChange={setFilter}
                  />
                </div>
              </div>
              <MaterialList
                key={JSON.stringify([
                  sessionIdentity?.actor.id,
                  space.selected?.id,
                  projectID,
                  kind,
                  revision,
                  query,
                  filter,
                ])}
                projectID={projectID}
                actorID={sessionIdentity?.actor.id}
                kind={kind}
                selected={selected}
                onToggle={toggle}
                query={query}
                filter={filter}
                limit={limit}
                canvasPresentation={canvasPresentation}
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
              {onUnifiedSubject || browseSources ? (
                sessionIdentity && space.selected?.id ? (
                  <ProjectPickerSubjects
                    key={`${sessionIdentity.actor.id}/${space.selected.id}`}
                    actorID={sessionIdentity.actor.id}
                    scopeID={space.selected.id}
                    selected={unifiedSubject?.id}
                    onSelect={setUnifiedSubject}
                  />
                ) : (
                  <ContentState state="empty" title="请选择创作空间" />
                )
              ) : (
                <>
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
                </>
              )}
            </TabsContent>
            {browseSources && (
              <TabsContent
                value="canvases"
                className="flex min-h-0 flex-1 flex-col"
              >
                <ProjectPickerCanvases />
              </TabsContent>
            )}
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
                : tab === "canvases"
                  ? "整张画布暂不可添加"
                  : onUnifiedSubject || browseSources
                    ? unifiedSubject
                      ? `已选择：${unifiedSubject.name}`
                      : "请选择一个主体"
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
                confirming ||
                (tab === "assets"
                  ? !selected.length
                  : tab === "canvases"
                    ? true
                    : onUnifiedSubject || browseSources
                      ? !onUnifiedSubject ||
                        !unifiedSubject ||
                        unifiedSubject.owner_scope_id !== space.selected?.id
                      : !subject)
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
                  } else if (
                    unifiedSubject &&
                    onUnifiedSubject &&
                    space.selected?.id
                  ) {
                    const verified = requireCanvasSubjectVersion(
                      await getSubjectVersion(
                        {
                          id: unifiedSubject.id,
                          version: unifiedSubject.version_id,
                        },
                        { signal: controller.signal },
                      ),
                      space.selected.id,
                      {
                        subject_id: unifiedSubject.id,
                        version_id: unifiedSubject.version_id,
                      },
                    );
                    if (
                      controller.signal.aborted ||
                      scope !== getScopeGeneration() ||
                      session !== getSessionGeneration()
                    )
                      return;
                    await onUnifiedSubject(verified);
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
                    : (confirmLabel ?? "添加到画布")
                  : tab === "canvases"
                    ? "暂不可添加"
                    : onUnifiedSubject
                      ? "添加主体"
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
              if (
                (reference && asset.kind !== initialKind) ||
                (allowedKinds && !allowedKinds.includes(asset.kind ?? ""))
              ) {
                setConfirmError(
                  "素材已上传到项目，但类型与当前参考不符，请选择对应类型的素材。",
                );
                return;
              }
              setConfirmError("");
              setQuery("");
              setKind(asset.kind!);
              setFilter((previous) =>
                projectPickerFilterForKind(previous, asset.kind!),
              );
              setSelected((items) =>
                limit === 1
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
  actorID,
  kind,
  selected,
  onToggle,
  query,
  filter,
  onSelectPage,
  limit,
  canvasPresentation,
}: {
  projectID: string;
  actorID?: string;
  kind: string;
  selected: API.AssetSummary[];
  onToggle: (asset: API.AssetSummary) => void;
  query: string;
  filter: ProjectPickerFilter;
  limit: number;
  canvasPresentation?: boolean;
  onSelectPage: (assets: API.AssetSummary[]) => void;
}) {
  const [items, setItems] = useState<API.AssetSummary[]>([]);
  const [pageIndex, setPageIndex] = useState(1);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const request = useRef<AbortController | null>(null);
  const busy = useRef(false);
  async function read(index: number, controller: AbortController) {
    return requireProjectPickerPage(
      await listMediaLibrary(
        projectPickerQuery(projectID, kind, query, index, filter),
        { signal: controller.signal },
      ),
      projectID,
      kind,
      index,
      actorID,
    );
  }
  useEffect(() => {
    const controller = new AbortController();
    request.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    const timer = setTimeout(
      () => {
        void read(1, controller)
          .then((page) => {
            if (
              controller.signal.aborted ||
              scope !== getScopeGeneration() ||
              session !== getSessionGeneration()
            )
              return;
            setItems(page.items);
            setPageIndex(page.page);
            setTotal(page.total);
            setHasMore(page.hasMore);
            setError("");
          })
          .catch((e) => {
            if (!controller.signal.aborted) setError(readErrorMessage(e));
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
  }, [projectID, actorID, kind, retry, query, filter]); // eslint-disable-line react-hooks/exhaustive-deps
  async function more() {
    if (busy.current || loading || !hasMore) return;
    busy.current = true;
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError("");
    try {
      const scope = getScopeGeneration(),
        session = getSessionGeneration();
      const page = await read(pageIndex + 1, controller);
      if (
        controller.signal.aborted ||
        scope !== getScopeGeneration() ||
        session !== getSessionGeneration()
      )
        return;
      setItems((previous) => [
        ...new Map(
          [...previous, ...page.items!].map((a) => [a.id, a]),
        ).values(),
      ]);
      setPageIndex(page.page);
      setTotal(page.total);
      setHasMore(page.hasMore);
    } catch (e) {
      if (!controller.signal.aborted) setError(readErrorMessage(e));
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setLoading(false);
    }
  }
  const filtered = items;
  const Icon = mediaKinds.find((k) => k.value === kind)!.icon;
  return (
    <>
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span>
          {loading
            ? query
              ? "正在搜索当前项目…"
              : "正在读取素材…"
            : error
              ? "读取未完成"
              : `${total} 个结果，已加载 ${filtered.length} 个`}
        </span>
        {limit > 1 && (
          <Button
            variant="ghost"
            size="sm"
            disabled={
              kind === "document" ||
              loading ||
              !filtered.length ||
              selected.length >= limit
            }
            onClick={() => onSelectPage(filtered)}
          >
            选择本页
          </Button>
        )}
      </div>
      {kind === "document" && (
        <p className="text-xs text-muted-foreground">
          可浏览项目文档；文档解析与添加到画布尚未接入。
        </p>
      )}
      <div className="canvas-material-content" data-canvas-wheel-scroll>
        {error && !loading && (
          <ContentState
            state="error"
            title="素材暂时无法读取"
            description={error}
            actions={
              <Button
                onClick={() => {
                  setError("");
                  setLoading(true);
                  setRetry((v) => v + 1);
                }}
              >
                重试
              </Button>
            }
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
                disabled={
                  kind === "document" ||
                  (limit > 1 && !chosen && selected.length >= limit)
                }
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
                          fit={canvasPresentation ? "contain" : undefined}
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
          <ContentState
            icon={FolderOpen}
            title={query ? "没有匹配的素材" : "当前项目暂无此类素材"}
            description={
              query
                ? "换个关键词，或清除搜索查看全部素材。"
                : "上传素材后，即可添加到画布。"
            }
          />
        )}
        {hasMore && (
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
