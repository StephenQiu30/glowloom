"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { cn } from "cn";
import {
  Plus,
  ImageIcon,
  Video,
  AudioLines,
  UserRound,
  Upload,
  FolderOpen,
  Scan,
  Link2,
  ArrowLeft,
  Search,
  Copy,
  ClipboardPaste,
} from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  PopoverAnchor,
  PopoverHeader,
  PopoverTitle,
} from "@/components/ui/popover";
import {
  Command,
  CommandInput,
  CommandList,
  CommandGroup,
  CommandItem,
  CommandEmpty,
  CommandSeparator,
} from "@/components/ui/command";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "@/components/ui/tooltip";
import { getMediaPreview } from "@/api/media";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import {
  referenceRoleLabel,
  type CanvasReferenceSource,
} from "./reference-source";
import { Field, FieldLabel } from "@/components/ui/field";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  useId,
  useMemo,
} from "react";
import {
  resolvePromptUpload,
  validatePromptFragment,
} from "@/api/promptEditor";
import { draftRequest } from "./draft-save";
import { validatePromptDraft } from "./prompt-document";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import { ReferenceSearch } from "./reference-search";
import { ReferenceMentionMenu } from "./reference-mention-menu";
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
  authorized = false,
  compact = false,
  home = false,
  canvasSources,
  incomingSources,
}: {
  session: ComposerSession;
  draft: PromptDraft;
  projectID: string;
  model?: API.ModelResponse;
  disabled?: boolean;
  authorized?: boolean;
  compact?: boolean;
  home?: boolean;
  canvasSources?: CanvasReferenceSource[];
  incomingSources?: (CanvasReferenceSource & { edgeID: string })[];
}) {
  const displayLabel = (id: string) =>
    session.referencePending(id)
      ? "待确认引用"
      : referenceLabel(draft.labelState, id);
  const formFieldId = useId();

  const [picker, setPicker] = useState<{
      kind: MediaKind;
      replace?: string;
      inline?: boolean;
    } | null>(null),
    [subjects, setSubjects] = useState(false),
    [subjectReplace, setSubjectReplace] = useState<string | null>(null),
    [menu, setMenu] = useState(false),
    [search, setSearch] = useState(""),
    [role, setRole] = useState("reference_image"),
    [error, setError] = useState(""),
    [remove, setRemove] = useState<string | null>(null),
    [review, setReview] = useState(false);
  const [lookup, setLookup] = useState<{
    query: string;
    seq: number;
    range?: { from: number; to: number };
  } | null>(null);
  const [fileIds, setFileIds] = useState<string[]>([]);
  const [unavailablePaste, setUnavailablePaste] = useState<string | null>(null);
  const [detailsID, setDetailsID] = useState<string | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [sourceKind, setSourceKind] = useState<"canvas" | "library" | null>(
    null,
  );
  const [sourceQuery, setSourceQuery] = useState("");
  const [readingSource, setReadingSource] = useState<string | null>(null);
  const sourceRequest = useRef<AbortController | null>(null);
  const currentSources = useRef({ canvasSources, incomingSources });
  useEffect(() => {
    currentSources.current = { canvasSources, incomingSources };
  }, [canvasSources, incomingSources]);
  useEffect(() => () => sourceRequest.current?.abort(), []);
  const referenceUploadInput = useRef<HTMLInputElement>(null);
  const inlineSource = useRef(false);
  const files = useRef(new Map<string, File>()),
    [uploading, setUploading] = useState<string[]>([]);
  const [mention, setMention] = useState<{
    range: { from: number; to: number };
    index: number;
  } | null>(null);
  const mentionFrom = mention?.range.from;
  const mentionAnchor = useMemo(
    () => ({
      current: {
        contextElement: session.editor.view.dom,
        getBoundingClientRect: () => {
          try {
            const from = mentionFrom ?? session.editor.state.selection.from;
            const rect = session.editor.view.coordsAtPos(from);
            return new DOMRect(
              rect.left,
              rect.top,
              1,
              Math.max(1, rect.bottom - rect.top),
            );
          } catch {
            return session.editor.view.dom.getBoundingClientRect();
          }
        },
      },
    }),
    [session, mentionFrom],
  );
  useEffect(() => {
    const changed = (
      value: {
        query: string;
        range: { from: number; to: number };
        index: number;
      } | null,
    ) => {
      setMention(value);
      if (value) {
        setAddOpen(false);
        setSourceKind(null);
        setSourceQuery("");
        setMenu(true);
        setSearch(value.query);
      } else setMenu(false);
    };
    session.editor.on("composerMention", changed);
    return () => {
      session.editor.off("composerMention", changed);
    };
  }, [session]);
  const panel = useRef<HTMLElement>(null);
  useEffect(() => {
    const reveal = ({ referenceId }: { referenceId: string }) => {
      if (compact) {
        setDetailsID(referenceId);
        return;
      }
      const card = panel.current?.querySelector<HTMLElement>(
        `[data-reference-card="${CSS.escape(referenceId)}"]`,
      );
      card?.scrollIntoView({ block: "nearest" });
      card?.focus();
    };
    session.editor.on("composerReferenceSelected", reveal);
    return () => {
      session.editor.off("composerReferenceSelected", reveal);
    };
  }, [session, compact]);
  const roles = [
    ...new Set([
      ...(model?.input_roles?.filter((r) => r !== "prompt") ?? []),
      ...draft.references.map((r) => r.role),
      role,
    ]),
  ];
  function add(
    binding: ReferenceBinding,
    inline = false,
    range = inline ? mention?.range : undefined,
  ) {
    try {
      session.insertReference(binding, inline, range);
      setMenu(false);
      setError("");
      if (inline) {
        setDetailsID(null);
        session.editor.commands.focus(undefined, { scrollIntoView: false });
      }
    } catch (e) {
      setError(String(e));
    }
  }
  async function addCanvasSource(
    source: CanvasReferenceSource,
    inline: boolean,
  ) {
    if (!source.assetID || disabled || sourceRequest.current) return;
    const controller = new AbortController();
    const seq = session.snapshot().localSeq;
    const range = inline ? mention?.range : undefined;
    const connection = incomingSources?.find(
      (item) =>
        item.nodeID === source.nodeID && item.assetID === source.assetID,
    );
    const sessionGeneration = getSessionGeneration(),
      scopeGeneration = getScopeGeneration();
    sourceRequest.current = controller;
    setReadingSource(source.nodeID);
    try {
      const preview = await getMediaPreview(
        { pid: projectID, asset_id: source.assetID },
        { signal: controller.signal },
      );
      if (
        controller.signal.aborted ||
        session.editor.isDestroyed ||
        getSessionGeneration() !== sessionGeneration ||
        getScopeGeneration() !== scopeGeneration
      )
        return;
      if (
        preview.asset?.project_id !== projectID ||
        preview.asset.id !== source.assetID ||
        preview.asset.kind !== source.kind
      )
        throw new Error("参考素材范围不匹配，请重新选择。");
      if (session.snapshot().localSeq !== seq)
        throw new Error("正文已变化，请重新选择参考。");
      const latestSources = connection
        ? currentSources.current.incomingSources
        : currentSources.current.canvasSources;
      if (
        !latestSources?.some(
          (item) =>
            item.nodeID === source.nodeID &&
            item.assetID === source.assetID &&
            item.kind === source.kind &&
            (!connection ||
              ("edgeID" in item && item.edgeID === connection.edgeID)),
        )
      )
        throw new Error("参考连接或来源已变化，请重新选择。");
      const binding = mediaBinding(preview.asset, `reference_${source.kind}`);
      if (
        connection &&
        !session.snapshot().draft.labelState.allocations[connection.edgeID]
      )
        binding.id = connection.edgeID;
      add(binding, inline, range);
      setAddOpen(false);
      setSourceKind(null);
    } catch (error) {
      if (!controller.signal.aborted)
        setError(
          error instanceof Error ? error.message : "参考来源暂时无法核验。",
        );
    } finally {
      sourceRequest.current = null;
      if (!controller.signal.aborted) setReadingSource(null);
    }
  }
  function choose(file: File, replace?: ReferenceBinding, inline = false) {
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
      role: replace?.role ?? (compact ? `reference_${kind}` : role),
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
    } else add(pending, inline);
    files.current.set(pending.id, file);
    setFileIds((ids) => [...new Set([...ids, pending.id])]);
    setReview(false);
    if (compact) setDetailsID(pending.id);
  }
  const acceptFiles = useEffectEvent((values: File[]) => {
    if (!disabled) values.forEach((file) => choose(file, undefined, true));
  });
  useEffect(() => {
    const accept = (value: { files: File[] }) => acceptFiles(value.files);
    session.editor.on("composerFiles", accept);
    return () => {
      session.editor.off("composerFiles", accept);
    };
  }, [session]);
  const checkedUploads = useRef(new Set<string>());
  async function recoverUpload(ref: ReferenceBinding) {
    if (ref.source.kind !== "pending_upload") return;
    const current = session.snapshot();
    const ticket = {
      contextKey: current.contextKey,
      replicaId: current.replicaId,
      referenceId: ref.id,
      generation: ref.bindingGeneration,
      attempt: ref.source.uploadAttemptId,
    };
    try {
      const result = await resolvePromptUpload({
        pid: projectID,
        attempt: ticket.attempt,
      });
      if (
        result.found &&
        result.source &&
        result.media_kind === ref.mediaKind
      ) {
        session.resolveUpload(ticket, {
          kind: "media",
          sourceId: result.source.sourceId!,
          sourceVersion: result.source.sourceVersion!,
        });
      }
    } catch {
      setError("原上传结果暂时无法核验；保留占位，请重试查询或原位重选文件。");
    }
  }
  const recoverOnMount = useEffectEvent(recoverUpload);
  useEffect(() => {
    for (const ref of draft.references) {
      if (ref.source.kind !== "pending_upload") continue;
      const key = `${ref.id}:${ref.bindingGeneration}:${ref.source.uploadAttemptId}`;
      if (!checkedUploads.current.has(key)) {
        checkedUploads.current.add(key);
        void recoverOnMount(ref);
      }
    }
  }, [draft.references]);
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
        setError("该上传所属引用已改变；结果保留在参考素材，可重新选择。");
      files.current.delete(id);
    } catch (e) {
      setError(e instanceof Error ? e.message : "上传结果未知，请重试原上传。");
    } finally {
      setUploading((v) => v.filter((x) => x !== id));
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(
        "glowloom-prompt-v1\n" + JSON.stringify(session.snapshot().draft),
      );
    } catch {
      setError("剪贴板不可用，请检查浏览器权限。");
    }
  }
  async function paste() {
    try {
      const seq = session.snapshot().localSeq;
      const text = await navigator.clipboard.readText();
      if (!text.startsWith("glowloom-prompt-v1\n"))
        throw new Error(
          "请使用站内“复制正文与引用”获取片段；普通文字可直接粘贴到正文。",
        );
      const fragment: unknown = JSON.parse(
        text.slice("glowloom-prompt-v1\n".length),
      );
      validatePromptDraft(fragment);
      try {
        await validatePromptFragment(
          { pid: projectID },
          draftRequest(fragment),
        );
      } catch {
        setUnavailablePaste(
          fragment.document.content
            .map((paragraph) =>
              (paragraph.content ?? [])
                .map((node) =>
                  node.type === "text"
                    ? node.text
                    : node.type === "hardBreak"
                      ? "\n"
                      : "[不可用引用]",
                )
                .join(""),
            )
            .join("\n"),
        );
        throw new Error(
          "引用未通过目标项目授权，可选择仅保留正文和不可用占位。素材需先明确导入目标项目。",
        );
      }
      if (!session.importFragment(fragment, seq))
        throw new Error("校验期间正文已变化，请重新粘贴；当前输入未被覆盖。");
    } catch (e) {
      setError(e instanceof Error ? e.message : "引用未通过授权校验");
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
  const referenceActions = (
    <>
      {draft.references.some((r) => r.role === "first_frame") &&
        draft.references.some((r) => r.role === "last_frame") && (
          <Button
            variant="outline"
            disabled={disabled}
            onClick={() => {
              const current = session.snapshot();
              session.applyConversion(
                current.localSeq,
                current.draft.params,
                current.draft.references.map((ref) =>
                  ["first_frame", "last_frame"].includes(ref.role)
                    ? {
                        ...ref,
                        role:
                          ref.role === "first_frame"
                            ? "last_frame"
                            : "first_frame",
                        bindingGeneration: ref.bindingGeneration + 1,
                      }
                    : ref,
                ),
              );
            }}
          >
            交换首尾帧
          </Button>
        )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="ghost"
          disabled={!authorized}
          onClick={() => void copy()}
        >
          复制正文与引用
        </Button>
        <Button
          variant="ghost"
          disabled={disabled}
          onClick={() => void paste()}
        >
          粘贴站内引用
        </Button>
      </div>
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
          onClick={() => {
            setSubjectReplace(null);
            setSubjects(true);
          }}
        >
          添加主体
        </Button>
        <div>
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            onClick={() => referenceUploadInput.current?.click()}
          >
            上传参考
          </Button>
        </div>
      </div>
      <Field>
        <FieldLabel htmlFor={`${formFieldId}-field-2`}>
          <span>新参考角色</span>
        </FieldLabel>
        <Select
          value={role}
          onValueChange={(selectedValue) => setRole(selectedValue)}
        >
          <SelectTrigger
            id={`${formFieldId}-field-2`}
            aria-label="新参考角色"
            className="w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {roles.map((r) => (
                <SelectItem key={r} value={r}>
                  {r}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
    </>
  );
  const referenceMenu = (
    <div className="rounded border p-3" aria-label="引用候选">
      <Input
        placeholder="按编号查找引用"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      <div className="flex flex-wrap gap-2">
        {draft.references
          .filter((r) => displayLabel(r.id).includes(search))
          .map((r) => (
            <Button key={r.id} variant="ghost" onClick={() => add(r, true)}>
              {displayLabel(r.id)}
            </Button>
          ))}
      </div>
      <Button
        variant="outline"
        onClick={() => {
          setLookup({
            query: search,
            seq: session.snapshot().localSeq,
            range: mention?.range,
          });
          setMenu(false);
        }}
      >
        搜索项目素材与主体
      </Button>
      <Button
        variant="ghost"
        onClick={() => {
          setMenu(false);
          session.editor.commands.focus();
        }}
      >
        关闭候选
      </Button>
    </div>
  );
  const referenceCards = (
    <div className={cn("grid grid-cols-1 gap-2", !compact && "sm:grid-cols-2")}>
      {draft.references
        .filter((ref) => !compact || ref.id === detailsID)
        .map((ref) => (
          <article
            key={ref.id}
            data-reference-card={ref.id}
            tabIndex={-1}
            className="flex min-w-0 flex-col gap-3"
          >
            <div className="flex items-center justify-between">
              {!compact ? <strong>{displayLabel(ref.id)}</strong> : null}
              <span className="text-xs text-muted-foreground">
                {mentioned.has(ref.id) ? "已提及" : "全局参考"}
              </span>
            </div>
            {authorized && ref.source.kind === "media" && (
              <div className="relative h-24 overflow-hidden rounded">
                <ProjectMediaPreview
                  projectID={projectID}
                  id={ref.source.sourceId}
                  kind={ref.mediaKind}
                  interactive
                />
              </div>
            )}
            {authorized && ref.source.kind === "subject" && (
              <p className="text-sm">
                {ref.source.description} · {ref.source.media.length} 个素材
              </p>
            )}
            {!authorized && ref.source.kind !== "pending_upload" && (
              <p className="text-sm">来源待核验；确认权限前隐藏内容。</p>
            )}
            <Select
              disabled={disabled}
              value={ref.role}
              onValueChange={(selectedValue) =>
                session.replaceReference(ref.id, {
                  ...ref,
                  role: selectedValue,
                })
              }
            >
              <SelectTrigger
                aria-label={`${displayLabel(ref.id)}角色`}
                className="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {roles.map((r) => (
                    <SelectItem key={r} value={r}>
                      {referenceRoleLabel(r)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            {ref.source.kind === "pending_upload" && (
              <div className="space-y-2 text-sm">
                <p>
                  {fileIds.includes(ref.id)
                    ? "待上传，草稿可保存"
                    : "请原位重选文件，引用编号会保留"}
                </p>
                <Button
                  disabled={disabled}
                  variant="ghost"
                  onClick={() => void recoverUpload(ref)}
                >
                  查询原上传结果
                </Button>
                <Input
                  aria-label={`重选${displayLabel(ref.id)}`}
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
                onClick={() => add(ref, true)}
              >
                提及
              </Button>
              {mentioned.has(ref.id) && (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    let target: number | undefined;
                    session.editor.state.doc.descendants((node, position) => {
                      if (
                        target === undefined &&
                        node.type.name === "reference" &&
                        node.attrs.referenceId === ref.id
                      )
                        target = position;
                    });
                    if (target !== undefined)
                      session.editor
                        .chain()
                        .focus()
                        .setNodeSelection(target)
                        .scrollIntoView()
                        .run();
                  }}
                >
                  定位正文
                </Button>
              )}
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
              {ref.source.kind === "subject" && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => {
                    setSubjectReplace(ref.id);
                    setSubjects(true);
                  }}
                >
                  更新 / 替换主体版本
                </Button>
              )}
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
  );
  const uploadConsent = draft.references.some(
    (ref) => ref.source.kind === "pending_upload",
  ) ? (
    <Field orientation="horizontal">
      <Checkbox
        id={`${formFieldId}-field-3`}

        checked={review}
        onCheckedChange={(e) => setReview(e === true)}
      />
      <FieldLabel htmlFor={`${formFieldId}-field-3`}>
        <span>我已确认这些素材可以用于本次创作</span>
      </FieldLabel>
    </Field>
  ) : null;
  const kinds = [
    { kind: "image" as const, label: "图片", icon: ImageIcon },
    { kind: "video" as const, label: "视频", icon: Video },
    { kind: "audio" as const, label: "音频", icon: AudioLines },
  ];
  const matchingReferences = draft.references.filter((ref) =>
    displayLabel(ref.id).includes(search),
  );
  const compactMenu = (inline: boolean) => (
    <Command
      className="h-auto"
      shouldFilter={false}
      value={
        inline && mention ? matchingReferences[mention.index]?.id : undefined
      }
    >
      {inline && mention ? (
        <p className="px-2 py-1 text-xs text-muted-foreground">
          @{search || "引用参考"}
        </p>
      ) : (
        <CommandInput
          aria-label="搜索参考"
          placeholder="搜索参考…"
          value={sourceQuery}
          onValueChange={setSourceQuery}
        />
      )}
      <CommandList className="max-h-[min(20rem,calc(var(--radix-popover-content-available-height)_-_3rem))] min-h-0">
        <CommandEmpty>暂无可用参考</CommandEmpty>
        {sourceKind ? (
          <CommandGroup>
            <CommandItem value="back" onSelect={() => setSourceKind(null)}>
              <ArrowLeft />
              返回
            </CommandItem>
          </CommandGroup>
        ) : null}
        {!sourceKind && draft.references.length ? (
          <CommandGroup heading="本次参考">
            {draft.references
              .filter((ref) =>
                displayLabel(ref.id).includes(inline ? search : sourceQuery),
              )
              .map((ref) => (
                <CommandItem
                  key={ref.id}
                  value={ref.id}
                  onSelect={() => {
                    add(ref, true);
                    setAddOpen(false);
                  }}
                >
                  {ref.mediaKind === "video" ? (
                    <Video />
                  ) : ref.mediaKind === "audio" ? (
                    <AudioLines />
                  ) : (
                    <ImageIcon />
                  )}
                  {displayLabel(ref.id)}
                </CommandItem>
              ))}
          </CommandGroup>
        ) : null}
        {sourceKind === "canvas" ? (
          <CommandGroup heading="当前画布">
            {canvasSources
              ?.filter((source) => source.title.includes(sourceQuery))
              .map((source) => (
                <CommandItem
                  key={source.nodeID}
                  value={source.nodeID}
                  disabled={!source.assetID || !!readingSource}
                  onSelect={() => void addCanvasSource(source, inline)}
                >
                  {source.kind === "video" ? (
                    <Video />
                  ) : source.kind === "audio" ? (
                    <AudioLines />
                  ) : (
                    <ImageIcon />
                  )}
                  <span className="truncate">{source.title}</span>
                  {!source.assetID ? (
                    <span className="ml-auto text-xs text-muted-foreground">
                      内容为空
                    </span>
                  ) : readingSource === source.nodeID ? (
                    <span className="ml-auto">正在核验…</span>
                  ) : null}
                </CommandItem>
              ))}
          </CommandGroup>
        ) : sourceKind === "library" ? (
          <CommandGroup heading="素材类型">
            {kinds.map(({ kind, label, icon: Icon }) => (
              <CommandItem
                key={kind}
                value={kind}
                onSelect={() => {
                  setPicker({ kind, inline });
                  setAddOpen(false);
                  setMenu(false);
                }}
              >
                <Icon />
                {label}
              </CommandItem>
            ))}
          </CommandGroup>
        ) : (
          <>
            <CommandSeparator />
            <CommandGroup heading="添加参考">
              {canvasSources ? (
                <CommandItem
                  value="canvas"
                  onSelect={() => setSourceKind("canvas")}
                >
                  <Scan />
                  从画布选择
                </CommandItem>
              ) : null}
              <CommandItem
                value="library"
                onSelect={() => setSourceKind("library")}
              >
                <FolderOpen />
                从参考素材添加
              </CommandItem>
              <CommandItem
                value="upload"
                onSelect={() => {
                  setAddOpen(false);
                  setMenu(false);
                  inlineSource.current = inline;
                  referenceUploadInput.current?.click();
                }}
              >
                <Upload />
                上传参考内容
              </CommandItem>
              <CommandItem
                value="subject"
                onSelect={() => {
                  setSubjectReplace(null);
                  inlineSource.current = inline;
                  setSubjects(true);
                  setAddOpen(false);
                  setMenu(false);
                }}
              >
                <UserRound />
                添加主体
              </CommandItem>
              <CommandItem
                value="search"
                onSelect={() => {
                  setLookup({
                    query: inline ? search : sourceQuery,
                    seq: session.snapshot().localSeq,
                    range: inline ? mention?.range : undefined,
                  });
                  setMenu(false);
                  setAddOpen(false);
                }}
              >
                <Search />
                搜索项目素材与主体
              </CommandItem>
            </CommandGroup>
            {!inline ? (
              <CommandGroup heading="更多操作">
                <CommandItem
                  value="copy"
                  disabled={!authorized}
                  onSelect={() => {
                    void copy();
                    setAddOpen(false);
                  }}
                >
                  <Copy />
                  复制正文与引用
                </CommandItem>
                <CommandItem
                  value="paste"
                  onSelect={() => {
                    void paste();
                    setAddOpen(false);
                  }}
                >
                  <ClipboardPaste />
                  粘贴站内引用
                </CommandItem>
              </CommandGroup>
            ) : null}
          </>
        )}
      </CommandList>
    </Command>
  );
  return (
    <TooltipProvider>
      <Popover
        open={compact && menu}
        onOpenChange={(open) => {
          setMenu(open);
        }}
      >
        <section
          ref={panel}
          aria-label="参考素材"
          className={cn(
            "flex w-full min-w-0 flex-col gap-3",
            compact && "gap-0",
          )}
        >
          <p
            className={cn(
              "text-xs text-muted-foreground",
              compact && "sr-only",
            )}
          >
            本次参考 {draft.references.length} 项 · 正文提及{" "}
            {draft.document.content.reduce(
              (count, p) =>
                count +
                (p.content?.filter((n) => n.type === "reference").length ?? 0),
              0,
            )}{" "}
            处
          </p>
          {compact ? (
            <>
              <PopoverAnchor virtualRef={mentionAnchor} />
              <div
                className="flex w-full min-w-0 items-start gap-2 overflow-x-auto pb-1"
                aria-label="参考内容"
              >
                {draft.references.map((ref) => {
                  const Icon =
                    ref.source.kind === "subject"
                      ? UserRound
                      : ref.mediaKind === "video"
                        ? Video
                        : ref.mediaKind === "audio"
                          ? AudioLines
                          : ImageIcon;
                  const automatic = incomingSources?.some(
                    (source) => source.edgeID === ref.id,
                  );
                  return (
                    <Popover
                      key={ref.id}
                      open={detailsID === ref.id}
                      onOpenChange={(open) =>
                        setDetailsID(open ? ref.id : null)
                      }
                    >
                      <PopoverTrigger asChild>
                        <Button
                          type="button"
                          variant="outline"
                          className="relative h-16 w-16 shrink-0 flex-col gap-1 overflow-hidden"
                          aria-label={`查看${displayLabel(ref.id)}详情`}
                        >
                          {authorized &&
                          ref.source.kind === "media" &&
                          ref.mediaKind !== "audio" ? (
                            <span className="absolute inset-0">
                              <ProjectMediaPreview
                                projectID={projectID}
                                id={ref.source.sourceId}
                                kind={ref.mediaKind}
                                compact
                              />
                            </span>
                          ) : (
                            <Icon data-icon="inline-start" />
                          )}
                          <Badge variant="secondary" className="relative">
                            {displayLabel(ref.id)}
                          </Badge>
                          {ref.source.kind === "pending_upload" ? (
                            <span className="relative text-[10px] text-muted-foreground">
                              待上传
                            </span>
                          ) : null}
                          {automatic ? (
                            <Badge
                              variant="secondary"
                              className="absolute top-0 right-0 p-0.5"
                              aria-label="自动引用"
                            >
                              <Link2 data-icon="inline-start" />
                            </Badge>
                          ) : null}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent
                        side="top"
                        align="start"
                        collisionPadding={16}
                        className="max-h-[min(32rem,var(--radix-popover-content-available-height))] w-80 overflow-y-auto"
                        aria-label="参考详情"
                        onClick={(event) => event.stopPropagation()}
                        onCloseAutoFocus={(event) => {
                          if (
                            !session
                              .snapshot()
                              .draft.references.some(
                                (item) => item.id === ref.id,
                              )
                          ) {
                            event.preventDefault();
                            session.editor.commands.focus();
                          }
                        }}
                      >
                        <PopoverHeader>
                          <PopoverTitle>
                            {displayLabel(ref.id)}
                            {automatic ? (
                              <Badge variant="secondary" className="ml-2">
                                <Link2 data-icon="inline-start" />
                                自动引用
                              </Badge>
                            ) : null}
                          </PopoverTitle>
                        </PopoverHeader>
                        {referenceCards}
                        {uploadConsent}
                      </PopoverContent>
                    </Popover>
                  );
                })}
                {incomingSources
                  ?.filter(
                    (source) =>
                      !draft.references.some(
                        (ref) =>
                          ref.source.kind === "media" &&
                          ref.source.sourceId === source.assetID &&
                          ref.mediaKind === source.kind,
                      ),
                  )
                  .map((source) => (
                    <Tooltip key={source.edgeID}>
                      <TooltipTrigger asChild>
                        <Button
                          variant="outline"
                          className="h-14 w-16 shrink-0 flex-col gap-1"
                          aria-label={`${source.title}：${source.assetID ? "未引用" : "内容为空"}`}
                          disabled={
                            disabled || !source.assetID || !!readingSource
                          }
                          onClick={() => void addCanvasSource(source, false)}
                        >
                          {source.kind === "video" ? (
                            <Video />
                          ) : source.kind === "audio" ? (
                            <AudioLines />
                          ) : (
                            <ImageIcon />
                          )}
                          <span className="text-xs">
                            {source.assetID ? "未引用" : "内容为空"}
                          </span>
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>
                        {source.title} ·{" "}
                        {source.assetID ? "点击加入参考" : "节点内容为空"}
                      </TooltipContent>
                    </Tooltip>
                  ))}
                <Popover
                  open={addOpen}
                  onOpenChange={(open) => {
                    setAddOpen(open);
                    setSourceKind(null);
                    setSourceQuery("");
                  }}
                >
                  <PopoverTrigger asChild>
                    <Button
                      type="button"
                      variant={home ? "reference" : "outline"}
                      size={home ? "reference" : "icon"}
                      disabled={disabled}
                      aria-label="添加参考"
                      className={home ? "shrink-0" : "size-14 shrink-0"}
                    >
                      <Plus />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent
                    side="top"
                    align="start"
                    collisionPadding={16}
                    sideOffset={8}
                    className="max-h-(--radix-popover-content-available-height) w-72 max-w-[calc(100vw-32px)] overflow-hidden p-0"
                    aria-label="添加参考素材"
                    onClick={(event) => event.stopPropagation()}
                    onCloseAutoFocus={(event) => {
                      if (menu || session.editor.isFocused)
                        event.preventDefault();
                    }}
                  >
                    {compactMenu(false)}
                  </PopoverContent>
                </Popover>
              </div>
            </>
          ) : (
            referenceActions
          )}
          {menu &&
            (compact ? (
              <PopoverContent
                side="bottom"
                align="start"
                sideOffset={8}
                collisionPadding={16}
                className="max-h-(--radix-popover-content-available-height) w-60 overflow-visible p-0"
                aria-label="引用候选"
                onClick={(event) => event.stopPropagation()}
                onOpenAutoFocus={(event) => {
                  event.preventDefault();
                  session.editor.view.focus();
                }}
                onCloseAutoFocus={(event) => event.preventDefault()}
                onInteractOutside={(event) => {
                  if (session.editor.view.dom.contains(event.target as Node))
                    event.preventDefault();
                }}
              >
                <ReferenceMentionMenu
                  session={session}
                  draft={draft}
                  sources={incomingSources ?? canvasSources}
                  query={mention ? search : ""}
                  busy={!!readingSource}
                  disabled={disabled}
                  onReference={(ref) => add(ref, true)}
                  onSource={(source) => void addCanvasSource(source, true)}
                />
              </PopoverContent>
            ) : (
              referenceMenu
            ))}
          {lookup && (
            <Dialog
              open
              onOpenChange={(open) => {
                if (!open) {
                  setLookup(null);
                  session.editor.commands.focus(undefined, {
                    scrollIntoView: false,
                  });
                }
              }}
            >
              <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl">
                <DialogTitle>选择项目参考</DialogTitle>
                <DialogDescription>
                  选择当前项目的素材或主体。
                </DialogDescription>
                <ReferenceSearch
                  projectID={projectID}
                  initialQuery={lookup.query}
                  onClose={() => {
                    setLookup(null);
                    session.editor.commands.focus();
                  }}
                  onSelect={(candidates) => {
                    if (session.snapshot().localSeq !== lookup.seq) {
                      setError("检索期间草稿已变化，请重新打开候选后插入。");
                      setLookup(null);
                      return;
                    }
                    try {
                      const bindings = candidates.map((candidate) =>
                        candidate.media
                          ? mediaBinding(
                              candidate.media,
                              compact
                                ? `reference_${candidate.media.kind}`
                                : role,
                            )
                          : subjectBinding(
                              candidate.subject!,
                              compact ? "subject" : role,
                            ),
                      );
                      session.insertReferences(bindings, lookup.range);
                      setError("");
                      setLookup(null);
                      session.editor.commands.focus();
                    } catch (e) {
                      setError(String(e));
                    }
                  }}
                />
              </DialogContent>
            </Dialog>
          )}
          {!compact ? (
            <>
              {referenceCards}
              {uploadConsent}
            </>
          ) : null}
          <Input
            ref={referenceUploadInput}
            aria-label="上传参考文件"
            type="file"
            accept="image/*,video/*,audio/*"
            className="hidden"
            disabled={disabled}
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) choose(file, undefined, inlineSource.current);
              e.target.value = "";
            }}
          />
          {error && <FeedbackToast message={error} />}
          {unavailablePaste !== null && (
            <div className="rounded border p-3">
              <p>
                无法访问的引用将转为“[不可用引用]”文字，不携带素材身份、地址或主体描述。
              </p>
              <Button
                disabled={disabled}
                onClick={() => {
                  session.editor.commands.insertContent(
                    unavailablePaste.split("\n").map((text) => ({
                      type: "paragraph",
                      content: text ? [{ type: "text", text }] : [],
                    })),
                  );
                  setUnavailablePaste(null);
                }}
              >
                仅粘贴文字与不可用占位
              </Button>
              <Button variant="ghost" onClick={() => setUnavailablePaste(null)}>
                取消粘贴
              </Button>
            </div>
          )}
          {picker && (
            <ProjectMediaPicker
              projectID={projectID}
              kind={picker.kind}
              onClose={() => setPicker(null)}
              onSelect={(asset) => {
                try {
                  const binding = mediaBinding(
                    asset,
                    compact ? `reference_${asset.kind}` : role,
                  );
                  if (picker.replace) {
                    const original = draft.references.find(
                      (r) => r.id === picker.replace,
                    )!;
                    session.replaceReference(original.id, {
                      ...binding,
                      role: original.role,
                    });
                  } else add(binding, picker.inline);
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
                    const original = draft.references.find(
                      (r) => r.id === subjectReplace,
                    );
                    if (original)
                      session.replaceReference(
                        original.id,
                        subjectBinding(subject, original.role),
                      );
                    else
                      add(
                        subjectBinding(subject, compact ? "subject" : role),
                        compact && inlineSource.current,
                      );
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
      </Popover>
    </TooltipProvider>
  );
}
