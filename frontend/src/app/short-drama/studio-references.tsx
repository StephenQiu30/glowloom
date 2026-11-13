"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useEffect, useRef, useState } from "react";
import { AtSign, FolderOpen, Link2, Plus, Upload, X } from "lucide-react";
import {
  getMediaLibraryItem,
  downloadLibraryMediaAsset,
  importPersonalVideoLink,
} from "@/api/media";
import {
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import { mediaError, type LibraryItem } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";
import { UploadDialog } from "@/components/media/upload-dialog";
import { DownloadButton } from "@/components/media/download-button";
import {
  PersonalLibraryPicker,
  PersonalMediaPreview,
  isPersonalReferenceItem,
  type PersonalReferenceItem,
  type ReferenceOwner,
} from "@/components/media/personal-reference-picker";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function validateStudioReferences(
  items: readonly PersonalReferenceItem[],
) {
  for (const kind of ["image", "video", "document"])
    if (items.filter((item) => item.kind === kind).length > 30)
      return `最多添加30${kind === "image" ? "张图片" : kind === "video" ? "个视频" : "份小说文档"}。`;
  return "";
}

export function StudioReferences({
  owner,
  value,
  onChange,
  disabled,
  onAuthenticate,
  compact = false,
  libraryOnly = false,
}: {
  owner?: ReferenceOwner;
  value: PersonalReferenceItem[];
  onChange: (value: PersonalReferenceItem[]) => void;
  disabled: boolean;
  onAuthenticate?: () => void;
  compact?: boolean;
  libraryOnly?: boolean;
}) {
  const [open, setOpen] = useState<"upload" | "library" | "link" | null>(null);
  const [preview, setPreview] = useState<PersonalReferenceItem>();
  const [error, setError] = useState("");
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  function choose(action: typeof open) {
    if (!owner) onAuthenticate?.();
    else setOpen(action);
  }
  function add(items: PersonalReferenceItem[]) {
    const merged = [
      ...new Map([...value, ...items].map((i) => [i.id, i])).values(),
    ];
    const problem = validateStudioReferences(merged);
    if (problem) {
      setError(problem);
      throw new Error(problem);
    }
    onChange(merged);
    setError("");
  }
  async function uploaded(asset?: API.AssetSummary) {
    if (!owner || !isProjectID(asset?.id))
      throw new Error("上传回执尚未确认。");
    const c = new AbortController();
    controller.current?.abort();
    controller.current = c;
    const session = getSessionGeneration(),
      scope = getScopeGeneration();
    // The media library's canonical projection uses asset IDs for uploaded items.
    const item = await getMediaLibraryItem(
      { scope: "personal", item_id: asset.id },
      { signal: c.signal },
    );
    if (
      c.signal.aborted ||
      session !== getSessionGeneration() ||
      scope !== getScopeGeneration()
    )
      return;
    if (
      !isPersonalReferenceItem(item as LibraryItem) ||
      item.media?.id !== asset.id ||
      item.media.revision !== asset.revision
    )
      throw new Error("上传素材身份或版本不匹配，请从资产库重新选择。");
    add([item as PersonalReferenceItem]);
    setOpen(null);
  }
  const trigger = compact ? (
    <Button
      variant="ghost"
      size="sm"
      className="px-2"
      disabled={disabled}
      aria-label="添加参考素材"
    >
      <Plus aria-hidden="true" />
    </Button>
  ) : (
    <Button
      variant="ghost"
      disabled={disabled}
      className="h-auto min-h-24 w-full min-w-0 flex-col gap-2 rounded-xl border border-dashed border-border px-3 py-4 md:min-h-40"
    >
      <Plus aria-hidden="true" />
      <span>{value.length ? "继续添加参考" : "上传小说/参考图/视频/链接"}</span>
      <span className="text-xs font-normal text-muted-foreground">
        图片/文档 ≤20MB · 视频 ≤500MB
      </span>
    </Button>
  );
  return (
    <>
      <div className="min-w-0 space-y-2">
        {libraryOnly ? (
          <Button
            variant="ghost"
            size="sm"
            className="px-2"
            disabled={disabled}
            onClick={() => choose("library")}
          >
            <AtSign aria-hidden="true" />
            参考素材
          </Button>
        ) : (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuItem onSelect={() => choose("upload")}>
                <Upload />
                从设备上传
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => choose("library")}>
                <FolderOpen />
                资产库
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => choose("link")}>
                <Link2 />
                视频链接上传
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
        {!compact && value.length > 0 && (
          <div
            aria-label="已选创作参考"
            className="grid max-h-64 grid-cols-2 gap-2 overflow-y-auto"
          >
            {value.map((item) => (
              <div key={item.id} className="relative min-w-0">
                <Button
                  variant="ghost"
                  className="relative h-24 w-full overflow-hidden rounded-lg bg-muted p-0"
                  aria-label={`预览 · ${item.title}`}
                  onClick={() => setPreview(item)}
                >
                  <PersonalMediaPreview
                    itemID={item.id}
                    assetID={item.asset_id}
                    kind={item.kind}
                    title={item.title}
                  />
                </Button>
                <Button
                  size="icon-sm"
                  variant="secondary"
                  className="absolute top-1 right-1"
                  disabled={disabled}
                  aria-label={`移除 · ${item.title}`}
                  onClick={() => {
                    onChange(value.filter((i) => i.id !== item.id));
                    setError("");
                  }}
                >
                  <X />
                </Button>
                <p className="truncate text-xs">{item.title}</p>
              </div>
            ))}
          </div>
        )}
        {error && <FeedbackToast message={error} />}
      </div>
      {open === "library" && owner && (
        <PersonalLibraryPicker
          owner={owner}
          maxSelection={90}
          onClose={() => setOpen(null)}
          onSelect={add}
        />
      )}
      {open === "upload" && owner && (
        <UploadDialog
          acceptedExtensions={[
            "png",
            "jpg",
            "jpeg",
            "webp",
            "mp4",
            "mov",
            "webm",
            "txt",
            "docx",
          ]}
          onClose={() => setOpen(null)}
          onUploaded={uploaded}
          destinationLabel="个人参考素材"
        />
      )}
      {open === "link" && owner && (
        <VideoLinkDialog onClose={() => setOpen(null)} onUploaded={uploaded} />
      )}
      {preview && (
        <Dialog
          open
          onOpenChange={(v) => {
            if (!v) setPreview(undefined);
          }}
        >
          <DialogContent className="sm:max-w-3xl">
            <DialogTitle>{preview.title}</DialogTitle>
            <div className="relative h-[min(60dvh,500px)] overflow-hidden rounded-xl bg-muted">
              <PersonalMediaPreview
                itemID={preview.id}
                assetID={preview.asset_id}
                kind={preview.kind}
                title={preview.title}
                large
              />
            </div>
            {preview.kind === "document" && (
              <DownloadButton
                filename={preview.title}
                load={() =>
                  downloadLibraryMediaAsset({
                    scope: "personal",
                    item_id: preview.id,
                  })
                }
              >
                下载小说原件
              </DownloadButton>
            )}
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}

export function VideoLinkDialog({
  onClose,
  onUploaded,
}: {
  onClose: () => void;
  onUploaded: (asset?: API.AssetSummary) => Promise<void>;
}) {
  const [url, setURL] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [pending, setPending] = useState(false);
  const [started, setStarted] = useState(false);
  const [error, setError] = useState("");
  const frozen = useRef<
    { url: string; key: string; asset?: API.AssetSummary } | undefined
  >(undefined);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  let valid = false;
  try {
    const u = new URL(url);
    valid =
      u.protocol === "https:" &&
      !u.username &&
      !u.password &&
      !u.hash &&
      (!u.port || u.port === "443") &&
      [
        "douyin.com",
        "www.douyin.com",
        "v.douyin.com",
        "iesdouyin.com",
        "www.iesdouyin.com",
        "toutiao.com",
        "www.toutiao.com",
        "m.toutiao.com",
        "v.toutiao.com",
        "ixigua.com",
        "www.ixigua.com",
        "m.ixigua.com",
        "v.ixigua.com",
      ].includes(u.hostname);
  } catch {
    /* Keep empty/invalid input disabled. */
  }
  async function submit() {
    if (pending || !valid || !confirmed) return;
    const c = new AbortController();
    controller.current = c;
    setPending(true);
    setError("");
    const session = getSessionGeneration(),
      scope = getScopeGeneration();
    frozen.current ??= { url, key: crypto.randomUUID() };
    setStarted(true);
    try {
      const command = frozen.current;
      if (!command.asset) {
        const r = await importPersonalVideoLink(
          { url: command.url, local_review_confirmed: true },
          {
            headers: { "Idempotency-Key": command.key },
            signal: c.signal,
            timeout: 600000,
          },
        );
        if (
          c.signal.aborted ||
          session !== getSessionGeneration() ||
          scope !== getScopeGeneration()
        )
          return;
        if (!isProjectID(r.asset?.id) || r.asset?.kind !== "video")
          throw new Error("视频解析尚未返回可用素材。");
        command.asset = {
          ...r.asset,
          duration_ms: r.asset.duration_ms ?? undefined,
          width: r.asset.width ?? undefined,
          height: r.asset.height ?? undefined,
        };
      }
      await onUploaded(command.asset);
    } catch (e) {
      if (!c.signal.aborted)
        setError(
          e instanceof RequestError && e.code === "video_link_unresolvable"
            ? "无法公开读取这个视频。请关闭后改用从设备上传，或重新提供可读取的公开链接。"
            : e instanceof RequestError && e.code === "video_link_busy"
              ? "当前导入较多，请稍后重试原导入。"
              : mediaError(e),
        );
    } finally {
      if (!c.signal.aborted) setPending(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v && !pending) onClose();
      }}
    >
      <DialogContent showCloseButton={!pending}>
        <DialogTitle>视频链接上传</DialogTitle>
        <DialogDescription>
          支持抖音、头条、西瓜的公开链接。需要登录或无法读取的页面可改用从设备上传。
        </DialogDescription>
        <Field>
          <FieldLabel htmlFor="studio-video-link">参考内容链接</FieldLabel>
          <Input
            id="studio-video-link"
            type="url"
            placeholder="请输入参考内容的链接"
            value={url}
            disabled={pending || started}
            onChange={(e) => setURL(e.target.value)}
          />
        </Field>
        <Field orientation="horizontal">
          <Checkbox
            checked={confirmed}
            id="studio-video-review"
            disabled={pending || started}
            onCheckedChange={(v) => setConfirmed(v === true)}
          />
          <FieldLabel htmlFor="studio-video-review">
            我有使用权限，且无待授权真人内容。
          </FieldLabel>
        </Field>
        {pending && <p role="status">正在解析并保存视频…</p>}
        {error && <FeedbackToast message={error} />}
        <DialogFooter>
          <Button variant="secondary" disabled={pending} onClick={onClose}>
            关闭
          </Button>
          <Button
            disabled={!valid || !confirmed || pending}
            onClick={() => void submit()}
          >
            {started ? "重试原导入" : "确认"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
