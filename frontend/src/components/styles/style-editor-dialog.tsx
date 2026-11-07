"use client";
import { useEffect, useId, useRef, useState } from "react";
import dynamic from "next/dynamic";
import { ArrowUpToLine, Plus, Sparkles, X } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  PersonalLibraryPicker,
  PersonalMediaPreview,
  type PersonalReferenceItem,
} from "@/components/media/personal-reference-picker";
import { UploadDropzone } from "@/components/media/upload-dropzone";
import { UploadDialog } from "@/components/media/upload-dialog";
import { validateUpload } from "@/components/media/library";
import { getMediaLibraryItem } from "@/api/media";
import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import {
  createStyle,
  readStyle,
  styleError,
  updateStyle,
  type StyleCategory,
  type StyleDetail,
  type StyleInput,
} from "./style-api";
import type { StyleOwner } from "./style-library-popover";
import { parseStyleAnalysisReply } from "./style-analysis-result";
const Analysis = dynamic(() =>
  import("./style-analysis-dialog").then(
    (module) => module.StyleAnalysisDialog,
  ),
);
export type StyleEditorDialogProps = {
  value?: Readonly<StyleDetail>;
  owner: StyleOwner;
  onSaved: (value: StyleDetail) => void;
  onClose: () => void;
};
export function StyleEditorDialog(props: StyleEditorDialogProps) {
  return (
    <StyleEditor
      key={`${props.owner.actorId}/${props.owner.scopeId}/${props.value?.id ?? "new"}`}
      {...props}
    />
  );
}
function StyleEditor({
  value,
  owner,
  onSaved,
  onClose,
}: StyleEditorDialogProps) {
  const id = useId();
  const [name, setName] = useState(value?.name ?? "");
  const [description, setDescription] = useState(value?.prompt_fragment ?? "");
  const [category, setCategory] = useState<StyleCategory>(
    value?.category ?? "realistic",
  );
  const [references, setReferences] = useState([
    ...(value?.reference_items ?? []),
  ]);
  const [revision, setRevision] = useState(value?.revision ?? 0);
  const [picker, setPicker] = useState(false);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const [analysis, setAnalysis] = useState(false);
  const [error, setError] = useState(
    value && !value.references_available
      ? "旧参考图已不可用。保存时将采用当前重新选择的图片，原描述已保留。"
      : "",
  );
  const [pending, setPending] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [conflict, setConflict] = useState(false);
  const busy = useRef(false);
  const request = useRef<AbortController | null>(null);
  const command = useRef<{
    key: string;
    input: StyleInput;
    revision: number;
  } | null>(null);
  useEffect(() => () => request.current?.abort(), []);
  const writable = !value || (value.owned && value.can_edit);
  const locked = pending || unknown || !writable;
  const nameLength = Array.from(name).length;
  const descriptionLength = Array.from(description).length;
  const valid =
    !!name.trim() &&
    nameLength <= 20 &&
    !!description.trim() &&
    descriptionLength <= 2000 &&
    references.length <= 4;
  async function save() {
    if (busy.current || !writable || (!unknown && (!valid || conflict))) return;
    const operation = command.current ?? {
      key: crypto.randomUUID(),
      revision,
      input: {
        name: name.trim(),
        prompt_fragment: description.trim(),
        category,
        reference_asset_ids: references.map((item) => item.asset_id),
      },
    };
    command.current = operation;
    busy.current = true;
    setPending(true);
    setError("");
    const controller = new AbortController();
    request.current = controller;
    try {
      const receipt = value
        ? await updateStyle(
            value.id,
            { ...operation.input, expected_revision: operation.revision },
            operation.key,
            controller.signal,
          )
        : await createStyle(operation.input, operation.key, controller.signal);
      if (!controller.signal.aborted) {
        command.current = null;
        setUnknown(false);
        onSaved(receipt.preset);
      }
    } catch (cause) {
      if (!controller.signal.aborted) {
        setError(styleError(cause));
        const definite =
          cause instanceof RequestError &&
          cause.status >= 400 &&
          cause.status < 500;
        setUnknown(!definite);
        if (definite) {
          command.current = null;
          setConflict(
            cause instanceof RequestError && cause.code === "revision_conflict",
          );
        }
      }
    } finally {
      if (!controller.signal.aborted) {
        busy.current = false;
        setPending(false);
      }
    }
  }
  async function refreshVersion() {
    if (!value || busy.current) return;
    busy.current = true;
    setPending(true);
    const controller = new AbortController();
    request.current = controller;
    try {
      const detail = await readStyle(value.id, controller.signal);
      if (!controller.signal.aborted) {
        setRevision(detail.revision);
        setConflict(false);
        setError("");
      }
    } catch (cause) {
      if (!controller.signal.aborted) setError(styleError(cause));
    } finally {
      if (!controller.signal.aborted) {
        busy.current = false;
        setPending(false);
      }
    }
  }
  function add(items: PersonalReferenceItem[]) {
    if (locked) return;
    const unique = items.filter(
      (item) =>
        !references.some((reference) => reference.asset_id === item.asset_id),
    );
    if (
      unique.some((item) => item.kind !== "image") ||
      references.length + unique.length > 4
    ) {
      setError("最多添加 4 张可用图片。");
      return;
    }
    setReferences((current) => [
      ...current,
      ...unique.map((item) => ({
        id: item.id,
        revision: item.revision,
        asset_id: item.asset_id,
        asset_revision: item.media.revision,
        title: item.title,
      })),
    ]);
    setPicker(false);
  }
  function chooseFile(file: File) {
    if (locked || references.length >= 4) return;
    const issue = validateUpload(file);
    if (issue || !/\.(png|jpe?g|webp)$/i.test(file.name)) {
      setError(issue || "请选择 PNG、JPEG 或 WebP 图片。");
      return;
    }
    setError("");
    setUploadFile(file);
  }
  async function adoptUpload(asset?: API.AssetSummary) {
    if (!asset?.id || references.length >= 4)
      throw new Error("请重新读取可用参考图。");
    const controller = new AbortController();
    request.current = controller;
    const item = await getMediaLibraryItem(
      { scope: "personal", item_id: asset.id },
      { signal: controller.signal },
    );
    controller.signal.throwIfAborted();
    if (
      !isProjectID(item.id) ||
      item.asset_id !== asset.id ||
      item.media?.id !== asset.id ||
      item.kind !== "image" ||
      item.catalog_state !== "active" ||
      !Number.isSafeInteger(item.revision) ||
      Number(item.revision) < 0 ||
      !Number.isSafeInteger(item.media.revision) ||
      Number(item.media.revision) < 1
    )
      throw new RequestError("invalid_receipt");
    const reference = {
      id: item.id,
      revision: item.revision!,
      asset_id: asset.id,
      asset_revision: item.media.revision!,
      title: item.title ?? asset.file_name ?? "参考图",
    };
    setReferences((items) =>
      items.some((current) => current.asset_id === reference.asset_id)
        ? items
        : [...items, reference],
    );
    setUploadFile(null);
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !pending && !unknown) onClose();
        }}
      >
        <DialogContent
          className="overflow-x-hidden sm:max-w-[850px]"
          showCloseButton={!pending && !unknown}
          onInteractOutside={(event) => {
            if (pending || unknown) event.preventDefault();
          }}
        >
          <DialogTitle>{value ? "编辑风格" : "创建风格"}</DialogTitle>
          <DialogDescription>
            用描述与参考图保存视觉风格，第一张图片作为封面。
          </DialogDescription>
          <div className="grid min-w-0 gap-6 md:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
            <FieldGroup className="min-w-0 gap-4">
              <Field data-disabled={locked} data-invalid={nameLength > 20}>
                <FieldLabel htmlFor={`${id}-name`}>风格名称</FieldLabel>
                <Input
                  id={`${id}-name`}
                  value={name}
                  disabled={locked}
                  aria-invalid={nameLength > 20}
                  onChange={(event) => setName(event.target.value)}
                />
                <FieldDescription>{nameLength}/20 字</FieldDescription>
              </Field>
              <Field data-disabled={locked}>
                <FieldLabel>风格分类</FieldLabel>
                <ToggleGroup
                  type="single"
                  value={category}
                  disabled={locked}
                  onValueChange={(next) => {
                    if (["realistic", "2d", "3d"].includes(next))
                      setCategory(next as StyleCategory);
                  }}
                  aria-label="风格分类"
                >
                  <ToggleGroupItem value="realistic">真人</ToggleGroupItem>
                  <ToggleGroupItem value="2d">2D</ToggleGroupItem>
                  <ToggleGroupItem value="3d">3D</ToggleGroupItem>
                </ToggleGroup>
              </Field>
              <Field
                data-disabled={locked}
                data-invalid={descriptionLength > 2000}
              >
                <FieldLabel htmlFor={`${id}-description`}>风格描述</FieldLabel>
                <Textarea
                  id={`${id}-description`}
                  className="min-h-[320px] resize-y"
                  value={description}
                  disabled={locked}
                  aria-invalid={descriptionLength > 2000}
                  onChange={(event) => setDescription(event.target.value)}
                />
                <FieldDescription>{descriptionLength}/2000 字</FieldDescription>
              </Field>
            </FieldGroup>
            <FieldGroup className="min-w-0 gap-4">
              <Field data-disabled={locked}>
                <FieldLabel>参考图</FieldLabel>
                <div className="grid grid-cols-2 gap-3">
                  {references.map((reference, index) => (
                    <div
                      key={reference.asset_id}
                      className="flex min-w-0 flex-col gap-1"
                    >
                      <div className="relative aspect-square overflow-hidden rounded-lg">
                        <PersonalMediaPreview
                          itemID={reference.id}
                          assetID={reference.asset_id}
                          title={reference.title}
                        />
                      </div>
                      <div className="flex items-center justify-between gap-1">
                        <span className="text-xs text-muted-foreground">
                          {index === 0 ? "封面" : `参考图 ${index + 1}`}
                        </span>
                        <div className="flex gap-1">
                          {index > 0 ? (
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              disabled={locked}
                              aria-label={`设为封面第 ${index + 1} 张参考图`}
                              onClick={() =>
                                setReferences((items) => [
                                  items[index]!,
                                  ...items.filter((_, n) => n !== index),
                                ])
                              }
                            >
                              <ArrowUpToLine />
                            </Button>
                          ) : null}
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            disabled={locked}
                            aria-label={`移除第 ${index + 1} 张参考图`}
                            onClick={() =>
                              setReferences((items) =>
                                items.filter((_, n) => n !== index),
                              )
                            }
                          >
                            <X />
                          </Button>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
                {references.length < 4 ? (
                  <div className="flex flex-col gap-2">
                    <UploadDropzone
                      file={null}
                      disabled={locked}
                      onBrowse={() => fileInput.current?.click()}
                      onChoose={chooseFile}
                      onError={setError}
                    />
                    <Button
                      variant="outline"
                      disabled={locked}
                      onClick={() => setPicker(true)}
                    >
                      <Plus data-icon="inline-start" />
                      添加参考图
                    </Button>
                  </div>
                ) : null}
                <Input
                  ref={fileInput}
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  tabIndex={-1}
                  className="sr-only"
                  aria-label="选择风格参考图片"
                  disabled={locked}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    if (file) chooseFile(file);
                    event.target.value = "";
                  }}
                />
                <FieldDescription>
                  最多 4 张图片。可从个人素材库选择或上传。
                </FieldDescription>
              </Field>
              <Button
                variant="outline"
                disabled={locked || !references.length}
                onClick={() => setAnalysis(true)}
              >
                <Sparkles data-icon="inline-start" />
                分析风格
              </Button>
              <FieldDescription>
                先准备参考图，再由你在 Agent 中发送和采用分析结果。
              </FieldDescription>
            </FieldGroup>
          </div>
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {unknown ? (
            <p role="status">
              保存结果待确认，重试会恢复同一请求，不会重复创建。
            </p>
          ) : null}
          {conflict ? (
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => void refreshVersion()}
            >
              读取最新版本并保留输入
            </Button>
          ) : null}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={pending || unknown}
              onClick={onClose}
            >
              关闭
            </Button>
            <Button
              disabled={
                pending || !writable || (!unknown && (!valid || conflict))
              }
              onClick={() => void save()}
            >
              {pending
                ? "正在保存…"
                : unknown
                  ? "重试原保存"
                  : value
                    ? "保存风格"
                    : "保存并使用"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {picker ? (
        <PersonalLibraryPicker
          owner={owner}
          allowedKinds={["image"]}
          maxSelection={4 - references.length}
          onClose={() => setPicker(false)}
          onSelect={add}
        />
      ) : null}
      {uploadFile ? (
        <UploadDialog
          initialFile={uploadFile}
          acceptedExtensions={["png", "jpg", "jpeg", "webp"]}
          destinationLabel="个人风格参考图"
          onClose={() => setUploadFile(null)}
          onUploaded={adoptUpload}
        />
      ) : null}
      {analysis ? (
        <Analysis
          owner={owner}
          references={references}
          onClose={() => setAnalysis(false)}
          onUse={(text) => {
            const result = parseStyleAnalysisReply(text);
            if (!name.trim()) setName(result.name);
            setDescription(result.description);
            setAnalysis(false);
            setError("");
          }}
        />
      ) : null}
    </>
  );
}
