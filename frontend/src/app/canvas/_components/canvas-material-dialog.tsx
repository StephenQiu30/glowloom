"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";

import { useEffect, useRef, useState } from "react";
import {
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
import { listMediaAssets } from "@/api/media";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { UploadDialog } from "@/components/media/upload-dialog";
import { SubjectList } from "@/components/subjects/subject-list";
import { subjectText, type Subject } from "@/components/subjects/contracts";
import { RequestError } from "@/lib/request";
import { flowError } from "@/components/generation/contracts";
import { isProjectID } from "@/components/projects/project";

const mediaKinds = [
  { value: "image", label: "图片", icon: ImageIcon },
  { value: "video", label: "视频", icon: VideoIcon },
  { value: "audio", label: "音频", icon: AudioLines },
  { value: "model", label: "3D", icon: Box },
];

export function CanvasMaterialDialog({
  projectID,
  initialTab = "assets",
  initialKind = "image",
  onClose,
  onAdd,
  onSubject,
}: {
  projectID: string;
  initialTab?: "assets" | "subjects";
  initialKind?: string;
  onClose: () => void;
  onAdd: (assets: API.AssetSummary[]) => void;
  onSubject: (subject: Subject) => void;
}) {
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
        : items.length < 24
          ? [...items, asset]
          : items,
    );
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
      >
        <DialogContent className="canvas-material-dialog sm:max-w-[800px]">
          <DialogHeader>
            <DialogTitle>选择素材</DialogTitle>
            <DialogDescription>
              从当前项目添加素材，或选择主体开始创作。
            </DialogDescription>
          </DialogHeader>
          <Tabs
            value={tab}
            onValueChange={(value) => setTab(value as typeof tab)}
            className="min-h-0 flex-1"
          >
            <TabsList variant="line">
              <TabsTrigger value="assets">资产</TabsTrigger>
              <TabsTrigger value="subjects">主体</TabsTrigger>
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
                  {mediaKinds.map(({ value, label, icon: Icon }) => (
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
              <MaterialList
                key={projectID + kind + revision}
                projectID={projectID}
                kind={kind}
                selected={selected}
                onToggle={toggle}
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
          <DialogFooter className="canvas-material-footer">
            <p
              className="mr-auto text-xs text-muted-foreground"
              aria-live="polite"
            >
              {tab === "assets"
                ? `已选择 ${selected.length} 个素材（最多 24 个）`
                : subject
                  ? `已选择：${subjectText(subject).name ?? "未命名主体"}`
                  : "请选择一个主体"}
            </p>
            <Button variant="outline" onClick={onClose}>
              取消
            </Button>
            <Button
              disabled={tab === "assets" ? !selected.length : !subject}
              onClick={() =>
                tab === "assets"
                  ? onAdd(selected)
                  : subject && onSubject(subject)
              }
            >
              {tab === "assets" ? "添加到画布" : "用主体创作"}
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
              setKind(asset.kind!);
              setSelected((items) =>
                items.length < 24 && !items.some((i) => i.id === asset.id)
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
}: {
  projectID: string;
  kind: string;
  selected: API.AssetSummary[];
  onToggle: (asset: API.AssetSummary) => void;
}) {
  const [items, setItems] = useState<API.AssetSummary[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [query, setQuery] = useState("");
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
      (page.next_cursor !== undefined && typeof page.next_cursor !== "string")
    )
      throw new RequestError("invalid_receipt");
    return page;
  }
  useEffect(() => {
    const controller = new AbortController();
    request.current = controller;
    listMediaAssets(
      { pid: projectID, kind, limit: 40 },
      { signal: controller.signal },
    )
      .then(validate)
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
    return () => {
      controller.abort();
      request.current?.abort();
    };
  }, [projectID, kind, retry]); // eslint-disable-line react-hooks/exhaustive-deps
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
      <InputGroup>
        <InputGroupAddon>
          <Search />
        </InputGroupAddon>
        <InputGroupInput
          aria-label="搜索已加载素材"
          placeholder="搜索已加载素材名称"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </InputGroup>
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
                disabled={!chosen && selected.length >= 24}
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
                  ? "换个关键词，或加载更多素材继续搜索。"
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
