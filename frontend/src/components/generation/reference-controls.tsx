"use client";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  ProjectMediaPicker,
  ProjectMediaPreview,
} from "@/components/media/project-media";
import { SubjectList } from "@/components/subjects/subject-list";
import { subjectText, type Subject } from "@/components/subjects/contracts";
import { uploadMediaAsset } from "@/api/media";
import { validateUpload } from "@/components/media/library";
import { ComposerSession } from "./composer-session";
import {
  referenceLabel,
  type MediaKind,
  type PromptDraft,
  type ReferenceBinding,
} from "./prompt-document";
export function mediaBinding(
  asset: API.AssetSummary,
  role: string,
): ReferenceBinding {
  if (
    !asset.id ||
    !Number.isSafeInteger(asset.revision) ||
    !["image", "video", "audio"].includes(asset.kind ?? "")
  )
    throw new Error("素材身份或版本不完整");
  return {
    id: crypto.randomUUID(),
    bindingGeneration: 1,
    mediaKind: asset.kind as MediaKind,
    role,
    order: 0,
    source: {
      kind: "media",
      sourceId: asset.id,
      sourceVersion: String(asset.revision),
    },
  };
}
export function subjectBinding(
  subject: Subject,
  role: string,
): ReferenceBinding {
  const text = subjectText(subject);
  return {
    id: crypto.randomUUID(),
    bindingGeneration: 1,
    mediaKind: "image",
    role,
    order: 0,
    source: {
      kind: "subject",
      sourceId: subject.head.id,
      sourceVersion: subject.current.id!,
      description:
        text.name + (text.description ? "：" + text.description : ""),
      media:
        subject.current.character?.looks
          ?.filter((l) => l.default)
          .flatMap((l) => l.references ?? [])
          .map((r) => ({
            sourceId: r.media!.asset_id!,
            sourceVersion: String(r.media!.revision),
            mediaKind: "image" as const,
          })) ?? [],
    },
  };
}
export function ReferenceControls({
  session,
  draft,
  projectID,
  model,
  disabled = false,
}: {
  session: ComposerSession;
  draft: PromptDraft;
  projectID: string;
  model?: API.ModelResponse;
  disabled?: boolean;
}) {
  const [picker, setPicker] = useState<{
      kind: MediaKind;
      replace?: string;
    } | null>(null),
    [subjects, setSubjects] = useState(false),
    [menu, setMenu] = useState(false),
    [search, setSearch] = useState(""),
    [role, setRole] = useState("reference_image"),
    [error, setError] = useState(""),
    [remove, setRemove] = useState<string | null>(null),
    [review, setReview] = useState(false);
  const [fileIds,setFileIds]=useState<string[]>([]);
  const files = useRef(new Map<string, File>()),
    [uploading, setUploading] = useState<string[]>([]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "@" && !event.isComposing) {
        setMenu(true);
        setSearch("");
      }
    };
    session.editor.view.dom.addEventListener("keyup", onKey);
    return () => session.editor.view.dom.removeEventListener("keyup", onKey);
  }, [session]);
  const roles = [
    ...new Set([
      ...(model?.input_roles?.filter((r) => r !== "prompt") ?? []),
      ...draft.references.map((r) => r.role),
      role,
    ]),
  ];
  function add(binding: ReferenceBinding) {
    try {
      session.insertReference(binding);
      setMenu(false);
    } catch (e) {
      setError(String(e));
    }
  }
  function choose(file: File, replace?: ReferenceBinding) {
    const error = validateUpload(file);
    if (error) {
      setError(error);
      return;
    }
    const kind = file.type.split("/")[0] as MediaKind;
    if (!["image", "video", "audio"].includes(kind)) {
      setError("请选择图片、视频或音频文件。");
      return;
    }
    const pending: ReferenceBinding = {
      id: replace?.id ?? crypto.randomUUID(),
      order: 0,
      bindingGeneration: 1,
      mediaKind: kind,
      role: replace?.role ?? role,
      source: {
        kind: "pending_upload",
        clientUploadId: crypto.randomUUID(),
        uploadAttemptId: crypto.randomUUID(),
      },
    };
    if (replace) {
      if (!session.replaceReference(replace.id, pending)) {
        setError("原位重选需保持相同媒体类型。");
        return;
      }
    } else add(pending);
    files.current.set(pending.id, file);
    setFileIds(ids=>[...new Set([...ids,pending.id])]);
    setReview(false);
  }
  async function upload(id: string) {
    const snapshot = session.snapshot(),
      ref = snapshot.draft.references.find((r) => r.id === id),
      file = files.current.get(id);
    if (
      !review ||
      !file ||
      !ref ||
      ref.source.kind !== "pending_upload" ||
      uploading.includes(id)
    )
      return;
    const ticket = {
      contextKey: snapshot.contextKey,
      replicaId: snapshot.replicaId,
      referenceId: id,
      generation: ref.bindingGeneration,
      attempt: ref.source.uploadAttemptId,
    };
    setUploading((v) => [...v, id]);
    setError("");
    try {
      const result = await uploadMediaAsset(
        { pid: projectID },
        { local_review_confirmed: true },
        file,
        { headers: { "Idempotency-Key": ticket.attempt } },
      );
      if (result.asset?.project_id !== projectID)
        throw new Error("上传范围不匹配");
      const source = mediaBinding(result.asset, ref.role).source;
      if (!session.resolveUpload(ticket, source))
        setError("该上传所属引用已改变；结果保留在素材库，可重新选择。");
      files.current.delete(id);
    } catch (e) {
      setError(e instanceof Error ? e.message : "上传结果未知，请重试原上传。");
    } finally {
      setUploading((v) => v.filter((x) => x !== id));
    }
  }
  const mentioned = new Set(
    draft.document.content.flatMap(
      (p) =>
        p.content?.flatMap((n) =>
          n.type === "reference" ? [n.attrs.referenceId] : [],
        ) ?? [],
    ),
  );
  return (
    <section aria-label="参考素材" className="space-y-3">
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={disabled}
          onClick={() => setMenu((v) => !v)}
        >
          ＠ 引用
        </Button>
        {(["image", "video", "audio"] as const).map((kind) => (
          <Button
            key={kind}
            variant="outline"
            disabled={disabled}
            onClick={() => setPicker({ kind })}
          >
            添加{kind === "image" ? "图片" : kind === "video" ? "视频" : "音频"}
          </Button>
        ))}
        <Button
          variant="outline"
          disabled={disabled}
          onClick={() => setSubjects(true)}
        >
          添加主体
        </Button>
        <label className="cursor-pointer rounded border px-3 py-2 text-sm">
          上传参考
          <input
            aria-label="上传参考文件"
            type="file"
            accept="image/*,video/*,audio/*"
            className="sr-only"
            disabled={disabled}
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) choose(file);
              e.target.value = "";
            }}
          />
        </label>
      </div>
      <label className="flex items-center gap-2 text-sm">
        新参考角色
        <select
          aria-label="新参考角色"
          value={role}
          onChange={(e) => setRole(e.target.value)}
          className="rounded border bg-background p-2"
        >
          {roles.map((r) => (
            <option key={r}>{r}</option>
          ))}
        </select>
      </label>
      {menu && (
        <div className="rounded border p-3" aria-label="引用候选">
          <Input
            autoFocus
            placeholder="按编号查找引用"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className="flex flex-wrap gap-2">
            {draft.references
              .filter((r) =>
                referenceLabel(draft.labelState, r.id).includes(search),
              )
              .map((r) => (
                <Button key={r.id} variant="ghost" onClick={() => add(r)}>
                  {referenceLabel(draft.labelState, r.id)}
                </Button>
              ))}
          </div>
          <Button variant="ghost" onClick={() => setMenu(false)}>
            关闭候选
          </Button>
        </div>
      )}
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {draft.references.map((ref) => (
          <article
            key={ref.id}
            className="min-w-0 space-y-2 rounded-lg border p-3"
          >
            <div className="flex items-center justify-between">
              <strong>{referenceLabel(draft.labelState, ref.id)}</strong>
              <span className="text-xs text-muted-foreground">
                {mentioned.has(ref.id) ? "已提及" : "作为全局参考发送"}
              </span>
            </div>
            {ref.source.kind === "media" && (
              <div className="relative h-24 overflow-hidden rounded">
                <ProjectMediaPreview
                  projectID={projectID}
                  id={ref.source.sourceId}
                  kind={ref.mediaKind}
                />
              </div>
            )}
            {ref.source.kind === "subject" && (
              <p className="text-sm">
                {ref.source.description} · {ref.source.media.length} 个素材
              </p>
            )}
            <select
              aria-label={`${referenceLabel(draft.labelState, ref.id)}角色`}
              disabled={disabled}
              value={ref.role}
              className="w-full rounded border bg-background p-1"
              onChange={(e) =>
                session.replaceReference(ref.id, {
                  ...ref,
                  role: e.target.value,
                })
              }
            >
              {roles.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
            {ref.source.kind === "pending_upload" && (
              <div className="space-y-2 text-sm">
                <p>
                  {fileIds.includes(ref.id)
                    ? "待上传，草稿可保存"
                    : "请原位重选文件，引用编号会保留"}
                </p>
                <input
                  aria-label={`重选${referenceLabel(draft.labelState, ref.id)}`}
                  type="file"
                  accept={`${ref.mediaKind}/*`}
                  disabled={disabled}
                  onChange={(e) => {
                    if (e.target.files?.[0]) choose(e.target.files[0], ref);
                  }}
                />
                <Button
                  disabled={
                    disabled ||
                    !review ||
                    !fileIds.includes(ref.id) ||
                    uploading.includes(ref.id)
                  }
                  onClick={() => void upload(ref.id)}
                >
                  {uploading.includes(ref.id) ? "上传中…" : "上传 / 重试原请求"}
                </Button>
              </div>
            )}
            <div className="flex flex-wrap gap-1">
              <Button
                size="sm"
                variant="ghost"
                disabled={disabled}
                onClick={() => add(ref)}
              >
                提及
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={disabled || ref.order === 0}
                onClick={() => session.reorderReference(ref.id, -1)}
              >
                前移
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={disabled || ref.order === draft.references.length - 1}
                onClick={() => session.reorderReference(ref.id, 1)}
              >
                后移
              </Button>
              {ref.source.kind !== "subject" && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() =>
                    setPicker({ kind: ref.mediaKind, replace: ref.id })
                  }
                >
                  替换
                </Button>
              )}
              <Button
                size="sm"
                variant="ghost"
                disabled={disabled}
                onClick={() => setRemove(ref.id)}
              >
                移除
              </Button>
            </div>
          </article>
        ))}
      </div>
      {draft.references.some((r) => r.source.kind === "pending_upload") && (
        <label className="flex gap-2 text-sm">
          <input
            type="checkbox"
            checked={review}
            onChange={(e) => setReview(e.target.checked)}
          />
          我已确认这些素材可以用于本次创作
        </label>
      )}
      {error && <p role="alert">{error}</p>}
      {picker && (
        <ProjectMediaPicker
          projectID={projectID}
          kind={picker.kind}
          onClose={() => setPicker(null)}
          onSelect={(asset) => {
            try {
              const binding = mediaBinding(asset, role);
              if (picker.replace) {
                const original = draft.references.find(
                  (r) => r.id === picker.replace,
                )!;
                session.replaceReference(original.id, {
                  ...binding,
                  role: original.role,
                });
              } else add(binding);
              setPicker(null);
            } catch (e) {
              setError(String(e));
            }
          }}
        />
      )}
      {subjects && (
        <Dialog open onOpenChange={setSubjects}>
          <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-4xl">
            <DialogTitle>选择主体版本</DialogTitle>
            <DialogDescription>
              保存当前描述和默认造型参考；后续主体更新不会悄悄改变这份草稿。
            </DialogDescription>
            <SubjectList
              projectID={projectID}
              onSelect={(subject) => {
                add(subjectBinding(subject, role));
                setSubjects(false);
              }}
            />
          </DialogContent>
        </Dialog>
      )}
      {remove && (
        <Dialog open onOpenChange={() => setRemove(null)}>
          <DialogContent>
            <DialogTitle>移除这份参考？</DialogTitle>
            <DialogDescription>
              正文中的所有对应标签会一起移除，可以一次撤销恢复。
            </DialogDescription>
            <Button
              onClick={() => {
                session.removeReference(remove);
                setRemove(null);
              }}
            >
              移除参考及标签
            </Button>
            <Button variant="outline" onClick={() => setRemove(null)}>
              取消
            </Button>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
