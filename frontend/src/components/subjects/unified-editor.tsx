"use client";
import { useEffect, useId, useRef, useState } from "react";
import dynamic from "next/dynamic";
import {
  ArrowLeft,
  ArrowRight,
  AudioLines,
  FolderPlus,
  ImagePlus,
  Plus,
  Upload,
  X,
} from "lucide-react";
import { createSubject, getSubject, updateSubject } from "@/api/subjects";
import { Button } from "@/components/ui/button";
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
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  RequestError,
  getScopeGeneration,
  getSessionGeneration,
} from "@/lib/request";
import { UnifiedSubjectMedia } from "./unified-media";
import { useSubjectVoiceJob } from "./unified-voice-job";
import { UnifiedSubjectVoiceControls } from "./unified-voice-controls";
import {
  requireUnifiedSubject,
  selectedSubjectMedia,
  subjectError,
  type SelectedSubjectMedia,
  type UnifiedSubject,
} from "./unified-contracts";
import "./unified-subjects.css";
const SourcePicker = dynamic(() =>
  import("./unified-source-picker").then((m) => m.UnifiedSubjectSourcePicker),
);
const SourceUpload = dynamic(() =>
  import("./unified-source-upload").then((m) => m.UnifiedSubjectSourceUpload),
);
type Target = "primary" | "reference" | "voice";
const selection = (m: SelectedSubjectMedia) => ({
  asset_id: m.asset_id,
  ...(m.bound ? { binding_id: m.bound.bindingID } : {}),
  ...(m.project_id ? { project_id: m.project_id } : {}),
});
const versionFacts = (subject: UnifiedSubject) =>
  JSON.stringify([
    subject.name,
    subject.description,
    [
      subject.primary,
      ...subject.references,
      ...(subject.voice ? [subject.voice.media] : []),
    ].map((media) => [
      media.binding_id,
      media.asset_id,
      media.asset_revision,
      media.kind,
      media.mime_type,
      media.file_name,
      media.duration_ms ?? null,
      media.width ?? null,
      media.height ?? null,
    ]),
    subject.references.length,
    subject.voice?.name ?? null,
  ]);

export function UnifiedSubjectEditor({
  actorID,
  scopeID,
  initial,
  onClose,
  onSaved,
}: {
  actorID: string;
  scopeID: string;
  initial?: UnifiedSubject;
  onClose: () => void;
  onSaved: (subject: UnifiedSubject) => void;
}) {
  const fieldID = useId();
  const [name, setName] = useState(initial?.name ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [primary, setPrimary] = useState<SelectedSubjectMedia | null>(
    initial ? selectedSubjectMedia(initial, initial.primary) : null,
  );
  const [references, setReferences] = useState<SelectedSubjectMedia[]>(
    initial?.references.map((m) => selectedSubjectMedia(initial, m)) ?? [],
  );
  const [voice, setVoice] = useState<{
    name: string;
    media: SelectedSubjectMedia;
  } | null>(
    initial?.voice
      ? {
          name: initial.voice.name,
          media: selectedSubjectMedia(initial, initial.voice.media),
        }
      : null,
  );
  const [source, setSource] = useState<{
    target: Target;
    mode: "upload" | "library";
    file?: File;
    suspended?: boolean;
  } | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [baseRevision, setBaseRevision] = useState(initial?.revision ?? 0);
  const [conflict, setConflict] = useState(false);
  const [frozen, setFrozen] = useState(false);
  const [leave, setLeave] = useState(false);
  const voiceJob = useSubjectVoiceJob({
    actorID,
    scopeID,
    onReady: (name, media) => setVoice({ name, media }),
  });
  const operation = useRef<{
    key: string;
    body: Parameters<typeof createSubject>[0];
    id?: string;
  } | null>(null);
  const request = useRef<AbortController | null>(null);
  const busy = useRef(false);
  const uploadReturnFocus = useRef<HTMLButtonElement>(null);
  useEffect(() => () => request.current?.abort(), []);
  const locked = pending || frozen || (!!source && !source.suspended);
  const mediaLocked = locked || !!source || voiceJob.active;
  const initialMedia = initial
    ? [
        initial.primary.asset_id,
        ...initial.references.map((m) => m.asset_id),
        initial.voice?.media.asset_id ?? "",
      ].join("/")
    : "";
  const currentMedia = primary
    ? [
        primary.asset_id,
        ...references.map((m) => m.asset_id),
        voice?.media.asset_id ?? "",
      ].join("/")
    : "";
  const dirty =
    name !== (initial?.name ?? "") ||
    description !== (initial?.description ?? "") ||
    initialMedia !== currentMedia;
  function close() {
    if (pending) return;
    if (frozen || dirty) setLeave(true);
    else onClose();
  }
  function choose(target: Target, media: SelectedSubjectMedia) {
    if (voiceJob.active) return "请等待当前音色任务完成。";
    const allowed =
      target === "voice"
        ? ["audio"]
        : target === "reference"
          ? ["image"]
          : ["image", "video"];
    if (!allowed.includes(media.kind)) {
      return target === "voice"
        ? "请选择音频文件。"
        : target === "reference"
          ? "补充参考仅支持图片。"
          : "主参考请选择图片或视频。";
    }
    if (target === "primary") {
      voiceJob.reset();
      setPrimary(media);
      if (media.kind === "video") setReferences([]);
    } else if (target === "voice") {
      voiceJob.reset();
      setVoice({ name: media.file_name || "上传音色", media });
    } else if (
      media.asset_id === primary?.asset_id ||
      references.some((r) => r.asset_id === media.asset_id)
    ) {
      return "此图片已经在参考主体中。";
    } else setReferences((items) => [...items, media]);
    setError("");
    setSource(null);
  }
  async function save() {
    if (conflict) return;
    if (
      busy.current ||
      voiceJob.active ||
      !primary ||
      !name.trim() ||
      name.length > 20 ||
      source
    )
      return;
    const op = operation.current ?? {
      key: crypto.randomUUID(),
      id: initial?.id,
      body: {
        expected_revision: baseRevision,
        name: name.trim(),
        description: description.trim(),
        primary: selection(primary),
        references: references.map(selection),
        ...(voice
          ? { voice: { name: voice.name, ...selection(voice.media) } }
          : {}),
      },
    };
    operation.current = op;
    busy.current = true;
    setPending(true);
    setFrozen(true);
    setError("");
    const controller = new AbortController();
    request.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    try {
      const options = {
        headers: { "Idempotency-Key": op.key },
        signal: controller.signal,
      };
      const saved = requireUnifiedSubject(
        op.id
          ? await updateSubject({ id: op.id }, op.body, options)
          : await createSubject(op.body, options),
        scopeID,
        op.id,
      );
      const expectedReferences = op.body.references ?? [];
      if (
        saved.revision !== (op.body.expected_revision ?? 0) + 1 ||
        saved.name !== op.body.name ||
        saved.description !== op.body.description ||
        saved.primary.asset_id !== op.body.primary?.asset_id ||
        saved.references.length !== expectedReferences.length ||
        saved.references.some(
          (m, index) => m.asset_id !== expectedReferences[index].asset_id,
        ) ||
        (saved.voice?.media.asset_id ?? null) !==
          (op.body.voice?.asset_id ?? null) ||
        (saved.voice?.name ?? null) !== (op.body.voice?.name ?? null)
      )
        throw new RequestError("invalid_receipt");
      const result = requireUnifiedSubject(
        await getSubject({ id: saved.id }, { signal: controller.signal }),
        scopeID,
        saved.id,
      );
      if (
        result.revision < saved.revision ||
        (result.revision === saved.revision &&
          (result.version_id !== saved.version_id ||
            versionFacts(result) !== versionFacts(saved)))
      )
        throw new RequestError("invalid_receipt");
      if (
        !controller.signal.aborted &&
        scope === getScopeGeneration() &&
        session === getSessionGeneration()
      )
        onSaved(result);
    } catch (reason) {
      if (!controller.signal.aborted) {
        setError(subjectError(reason));
        if (
          reason instanceof RequestError &&
          reason.status >= 400 &&
          reason.status < 500
        ) {
          operation.current = null;
          setFrozen(false);
          if (reason.status === 409) setConflict(true);
        }
      }
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  async function refreshConflict() {
    if (!initial || busy.current) return;
    busy.current = true;
    setPending(true);
    const controller = new AbortController();
    request.current = controller;
    try {
      const value = requireUnifiedSubject(
        await getSubject({ id: initial.id }, { signal: controller.signal }),
        scopeID,
        initial.id,
      );
      if (!controller.signal.aborted) {
        setBaseRevision(value.revision);
        setConflict(false);
        setError("已读取最新版本，当前输入仍保留。请核对后保存。");
      }
    } catch (reason) {
      if (!controller.signal.aborted) setError(subjectError(reason));
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  function sourceMenu(target: Target, label: string, icon = false) {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size={icon ? "icon" : "default"}
            disabled={mediaLocked}
            aria-label={label}
          >
            {icon ? (
              <Plus />
            ) : (
              <>
                <AudioLines />
                {label}
              </>
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="unified-subject-menu">
          <DropdownMenuItem
            onSelect={() => setSource({ target, mode: "upload" })}
          >
            <Upload />
            {target === "voice" ? "上传音色" : "从本地添加"}
          </DropdownMenuItem>
          {target !== "voice" ? (
            <DropdownMenuItem
              onSelect={() => setSource({ target, mode: "library" })}
            >
              <FolderPlus />
              从资产添加
            </DropdownMenuItem>
          ) : null}
          {target === "voice" ? (
            <DropdownMenuItem onSelect={() => void voiceJob.begin()}>
              智能配音
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <DialogContent
          className="unified-subject-settings w-[calc(100vw-32px)] sm:max-w-[560px]"
          showCloseButton={!pending}
          onInteractOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => {
            e.preventDefault();
            close();
          }}
        >
          <DialogTitle className="text-2xl leading-[34px] font-medium">
            设置主体
          </DialogTitle>
          <DialogDescription className="sr-only">
            设置主参考、名称、描述和可选音色，以便在当前空间创作中复用。
          </DialogDescription>
          <form
            className="flex min-h-0 flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <div className="unified-subject-settings-body flex min-h-0 flex-col gap-4">
              <Field>
                <FieldLabel>
                  参考主体 <span aria-hidden="true">*</span>
                </FieldLabel>
                <div
                  className="unified-subject-main"
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => {
                    e.preventDefault();
                    if (mediaLocked) return;
                    if (e.dataTransfer.files.length !== 1) {
                      setError("请每次上传一个文件。");
                      return;
                    }
                    const file = e.dataTransfer.files[0];
                    if (file)
                      setSource({ target: "primary", mode: "upload", file });
                  }}
                >
                  {primary ? (
                    <>
                      <div className="relative min-w-0 flex-1 self-stretch">
                        <UnifiedSubjectMedia
                          media={primary}
                          label={
                            primary.kind === "video" ? "主参考视频" : "主参考图"
                          }
                          interactive
                        />
                        <Button
                          type="button"
                          variant="secondary"
                          size="icon"
                          className="absolute top-1 right-1"
                          aria-label="移除主参考"
                          disabled={mediaLocked}
                          onClick={() => {
                            voiceJob.reset();
                            setPrimary(null);
                            setReferences([]);
                          }}
                        >
                          <X />
                        </Button>
                      </div>
                      {primary.kind === "image" ? (
                        <div className="flex h-full items-center px-2">
                          {sourceMenu("reference", "添加补充参考", true)}
                        </div>
                      ) : null}
                    </>
                  ) : (
                    <div className="flex h-full min-w-0 flex-col items-center justify-center gap-4 px-4 text-center">
                      <ImagePlus className="size-9 text-muted-foreground" />
                      <p className="text-sm text-muted-foreground">
                        上传主图，将素材拖拽至此处或从以下选择
                      </p>
                      <div className="flex flex-wrap justify-center gap-2">
                        <Button
                          type="button"
                          variant="secondary"
                          disabled={mediaLocked}
                          onClick={() =>
                            setSource({ target: "primary", mode: "upload" })
                          }
                        >
                          <Upload />
                          从本地添加
                        </Button>
                        <Button
                          type="button"
                          variant="secondary"
                          disabled={mediaLocked}
                          onClick={() =>
                            setSource({ target: "primary", mode: "library" })
                          }
                        >
                          <FolderPlus />
                          从资产添加
                        </Button>
                      </div>
                    </div>
                  )}
                </div>
              </Field>
              {source?.mode === "upload" && source.suspended ? (
                <div
                  className="flex flex-wrap items-center gap-2 rounded-lg border p-3"
                  aria-label="待确认上传"
                >
                  <p className="w-full text-sm">上传尚未完成确认</p>
                  <p className="w-full text-xs text-muted-foreground">
                    若文件已上传，会保留在个人素材中。
                  </p>
                  <Button
                    ref={uploadReturnFocus}
                    type="button"
                    variant="outline"
                    onClick={() =>
                      setSource((current) =>
                        current ? { ...current, suspended: false } : null,
                      )
                    }
                  >
                    继续确认上传
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => setSource(null)}
                  >
                    不采纳此文件
                  </Button>
                </div>
              ) : null}
              {references.length ? (
                <div className="flex flex-wrap gap-2" aria-label="补充参考">
                  {references.map((r, index) => (
                    <div
                      key={`${r.asset_id}/${index}`}
                      className="flex w-28 flex-col gap-1"
                    >
                      <div className="relative h-24 overflow-hidden rounded-lg bg-muted">
                        <UnifiedSubjectMedia
                          media={r}
                          label={`补充参考${index + 1}`}
                        />
                        <Button
                          type="button"
                          variant="secondary"
                          size="icon"
                          className="absolute top-0 right-0"
                          aria-label={`移除补充参考${index + 1}`}
                          disabled={mediaLocked}
                          onClick={() =>
                            setReferences((items) =>
                              items.filter((_, i) => i !== index),
                            )
                          }
                        >
                          <X />
                        </Button>
                      </div>
                      <div className="flex justify-between">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          disabled={mediaLocked || index === 0}
                          aria-label={`前移补充参考${index + 1}`}
                          onClick={() =>
                            setReferences((items) => {
                              const next = [...items];
                              [next[index - 1], next[index]] = [
                                next[index],
                                next[index - 1],
                              ];
                              return next;
                            })
                          }
                        >
                          <ArrowLeft />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          disabled={
                            mediaLocked || index === references.length - 1
                          }
                          aria-label={`后移补充参考${index + 1}`}
                          onClick={() =>
                            setReferences((items) => {
                              const next = [...items];
                              [next[index], next[index + 1]] = [
                                next[index + 1],
                                next[index],
                              ];
                              return next;
                            })
                          }
                        >
                          <ArrowRight />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}
              {primary ? (
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  {sourceMenu("voice", "添加音色")}
                  <UnifiedSubjectVoiceControls owner={voiceJob} />
                  {voice ? (
                    <div className="flex min-w-0 flex-1 items-center gap-2">
                      <div className="min-w-0 flex-1">
                        <UnifiedSubjectMedia
                          media={voice.media}
                          label={voice.name}
                          interactive
                        />
                      </div>
                      {!voiceJob.active ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label="移除音色"
                          disabled={mediaLocked}
                          onClick={() => {
                            voiceJob.reset();
                            setVoice(null);
                          }}
                        >
                          <X />
                        </Button>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              ) : null}
              <Field>
                <FieldLabel htmlFor={`${fieldID}-name`}>
                  名称 <span aria-hidden="true">*</span>
                </FieldLabel>
                <div className="relative">
                  <Input
                    id={`${fieldID}-name`}
                    aria-label="名称"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    maxLength={20}
                    required
                    disabled={locked}
                    placeholder="请输入名称"
                    className="h-10 pr-16"
                  />
                  <span
                    className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-muted-foreground"
                    aria-live="polite"
                  >
                    {name.length} / 20
                  </span>
                </div>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${fieldID}-description`}>描述</FieldLabel>
                <Textarea
                  id={`${fieldID}-description`}
                  aria-label="描述"
                  placeholder="请输入描述"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  maxLength={2000}
                  disabled={locked}
                  className="min-h-16 resize-y"
                />
              </Field>
              {error ? (
                <div
                  role="alert"
                  className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
                >
                  {error}
                  {conflict ? (
                    <Button
                      type="button"
                      variant="outline"
                      className="mt-2"
                      disabled={pending}
                      onClick={() => void refreshConflict()}
                    >
                      读取最新版本并保留输入
                    </Button>
                  ) : null}
                  {frozen ? (
                    <p className="mt-2 text-muted-foreground">
                      当前输入已冻结。重试会核对同一请求，不会重复创建主体。
                    </p>
                  ) : null}
                </div>
              ) : null}
            </div>
            <div className="flex shrink-0 justify-end">
              <Button
                type="submit"
                className="min-w-20"
                disabled={
                  pending ||
                  voiceJob.active ||
                  conflict ||
                  !!source ||
                  !primary ||
                  !name.trim() ||
                  name.length > 20
                }
              >
                {pending ? "正在保存…" : frozen ? "重试原保存" : "保存"}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
      {source?.mode === "library" ? (
        <SourcePicker
          actorID={actorID}
          scopeID={scopeID}
          allowedKinds={
            source.target === "voice"
              ? ["audio"]
              : source.target === "reference"
                ? ["image"]
                : ["image", "video"]
          }
          onClose={() => setSource(null)}
          onSelect={(media) => choose(source.target, media)}
        />
      ) : null}
      {source?.mode === "upload" ? (
        <SourceUpload
          open={!source.suspended}
          initialFile={source.file}
          allowedKinds={
            source.target === "voice"
              ? ["audio"]
              : source.target === "reference"
                ? ["image"]
                : ["image", "video"]
          }
          onClose={() => setSource(null)}
          onSuspend={() =>
            setSource((current) =>
              current ? { ...current, suspended: true } : null,
            )
          }
          onReturnFocus={() => uploadReturnFocus.current?.focus()}
          onUploaded={(media) => choose(source.target, media)}
        />
      ) : null}
      {leave ? (
        <Dialog open onOpenChange={setLeave}>
          <DialogContent role="alertdialog">
            <DialogTitle>离开主体设置？</DialogTitle>
            <DialogDescription>
              {frozen
                ? "保存结果尚未确认。离开前建议重试原保存，避免重复新建。"
                : "未保存的输入会丢失。"}
            </DialogDescription>
            <DialogFooter>
              <Button variant="outline" onClick={() => setLeave(false)}>
                继续设置
              </Button>
              <Button variant="destructive" onClick={onClose}>
                确认离开
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}
    </>
  );
}
