"use client";

import Link from "next/link";
import dynamic from "next/dynamic";
import { useEffect, useRef, useState } from "react";
import useSWR from "swr";
import { listMediaAssets } from "@/api/media";
import { isProjectID, type Project } from "@/components/projects/project";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { UploadDialog } from "@/components/media/upload-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { mediaBinding } from "@/components/generation/reference-controls";
import { useScriptWrite } from "./use-script-write";
import { redrawMarkets } from "./studio-input";

const ConversationWorkspace = dynamic(
  () =>
    import("@/components/conversations/conversation-workspace").then(
      (m) => m.ConversationWorkspace,
    ),
  { loading: () => <p role="status">正在读取单镜制作…</p> },
);
const singleShotGoal = {
  kind: "production",
  required_deliveries: 1,
} as const satisfies API.GoalContract;
const noProjectChange = () => {};

const RichGeneration = dynamic(
  () =>
    import("@/components/generation/rich-generation").then(
      (m) => m.RichGeneration,
    ),
  {
    loading: () => <p role="status">正在读取模型和生成配置…</p>,
  },
);

const referenceInstruction =
  "根据参考视频制作新版本。尽量保持主体、动作和叙事顺序，请按以下要求调整：";
const eligibleVideo = (asset: API.AssetSummary, projectID: string) =>
  isProjectID(asset.id) &&
  asset.project_id === projectID &&
  asset.kind === "video" &&
  ["video/mp4", "video/quicktime"].includes(asset.mime_type ?? "") &&
  Number.isSafeInteger(asset.revision) &&
  asset.revision! > 0 &&
  typeof asset.duration_ms === "number" &&
  asset.duration_ms >= 5000 &&
  asset.duration_ms <= 300000 &&
  typeof asset.byte_size === "number" &&
  asset.byte_size <= 500 * 1024 * 1024;

export function VideoPanel({
  project,
  owner,
  canvas,
  writable,
  initialPrompt,
  productionSessionID,
  onProductionAccepted,
  initialInput,
  styleAssets = [],
  onLock,
}: {
  project: Project;
  owner: string;
  canvas: string;
  writable: boolean;
  initialPrompt?: string;
  productionSessionID?: string;
  onProductionAccepted?: (session: string) => void;
  initialInput?: { file?: File; style: string; market: string };
  styleAssets?: readonly API.AssetSummary[];
  onLock: (locked: boolean) => void;
}) {
  const [production, setProduction] = useState(
    Boolean(productionSessionID || initialPrompt),
  );
  const [agentLocked, setAgentLocked] = useState(false);
  const [productionDraft, setProductionDraft] = useState(() =>
    prompt(initialPrompt ?? ""),
  );
  const [selected, setSelected] = useState("");
  const [style, setStyle] = useState(initialInput?.style ?? "");
  const [market, setMarket] = useState(initialInput?.market || "原片语言");
  const [upload, setUpload] = useState(Boolean(writable && initialInput?.file));
  const [initialFile, setInitialFile] = useState(initialInput?.file);
  const [draft, setDraft] = useState<PromptDraft | null>(() =>
    initialPrompt ? prompt(initialPrompt) : null,
  );
  const [result, setResult] = useState<string>();
  const [locked, setLocked] = useState(false);
  const closeGuard = useRef<(() => Promise<boolean>) | null>(null);
  const [draftID, setDraftID] = useState<string>();
  const change = useScriptWrite();
  const media = useSWR([owner, project.id, "production-videos"], () =>
    listMediaAssets({ pid: project.id, kind: "video", limit: 100 }),
  );
  const choices =
    media.data?.items?.filter((asset) => eligibleVideo(asset, project.id)) ??
    [];
  const source = choices.find((asset) => asset.id === selected);
  useEffect(() => {
    onLock(locked || agentLocked || change.pending);
    return () => onLock(false);
  }, [onLock, locked, agentLocked, change.pending]);

  function selectProduction(next: boolean) {
    if (locked || agentLocked || change.pending || production === next) return;
    void change.submit(async () => {
      if (closeGuard.current && !(await closeGuard.current())) return;
      setProduction(next);
    });
  }

  function selectSource(asset: API.AssetSummary) {
    if (locked || !eligibleVideo(asset, project.id)) return;
    void change.submit(async () => {
      if (closeGuard.current && !(await closeGuard.current())) return;
      const next = prompt(
        [
          referenceInstruction,
          `目标市场：${market}`,
          style.trim() && `风格：${style.trim()}`,
        ]
          .filter(Boolean)
          .join("\n"),
      );
      const binding = mediaBinding(asset, "reference_video");
      next.mode = "omni_reference";
      const images = styleAssets
        .filter(
          (image) => image.project_id === project.id && image.kind === "image",
        )
        .map((image) => mediaBinding(image, "reference_image"));
      next.references = [binding, ...images].map((reference, order) => ({
        ...reference,
        order,
      }));
      next.labelState.highWater.video = 1;
      next.labelState.allocations[binding.id] = {
        labelKind: "video",
        ordinal: 1,
      };
      for (const image of images) {
        const ordinal = ++next.labelState.highWater.image;
        next.labelState.allocations[image.id] = { labelKind: "image", ordinal };
      }
      setDraftID(undefined);
      setDraft(next);
    });
  }

  return (
    <section className="flex flex-col gap-4">
      <div className="flex gap-2" aria-label="制作方式">
        <Button
          variant={production ? "default" : "outline"}
          disabled={locked || agentLocked || change.pending}
          onClick={() => selectProduction(true)}
        >
          单镜制作
        </Button>
        <Button
          variant={production ? "outline" : "default"}
          disabled={locked || agentLocked || change.pending}
          onClick={() => selectProduction(false)}
        >
          视频转绘
        </Button>
      </div>
      {production ? (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold">单镜制作</h2>
          <p className="text-sm text-muted-foreground">
            描述镜头，确认制作方案与费用后开始。成片和剪辑工程检查通过后交付。
          </p>
          <ConversationWorkspace
            embedded
            ownerKey={owner}
            projectID={project.id}
            canvasID={project.canvas_id ?? undefined}
            sessionID={productionSessionID}
            enabled={writable}
            goalContract={singleShotGoal}
            initialDraft={productionDraft}
            onDraftChange={setProductionDraft}
            onAccepted={onProductionAccepted}
            onLock={setAgentLocked}
            onChooseProject={noProjectChange}
          />
        </section>
      ) : (
        <>
          <header>
            <h2 className="text-lg font-semibold">转绘与后期</h2>
            <p className="text-sm text-muted-foreground">
              使用支持视频参考的模型制作新版本。项目内可预览结果，并在短剧画布的时间线中剪辑和导出。
            </p>
          </header>
          {writable && (
            <div className="flex flex-wrap items-end gap-2">
              <Field className="max-w-sm">
                <FieldLabel htmlFor="short-drama-video">
                  转绘源视频（MP4 / MOV，5–300 秒）
                </FieldLabel>
                <NativeSelect
                  id="short-drama-video"
                  value={selected}
                  onChange={(event) => setSelected(event.target.value)}
                  disabled={locked || change.pending}
                >
                  <NativeSelectOption value="">
                    选择已上传的项目视频
                  </NativeSelectOption>
                  {choices.map((asset) => (
                    <NativeSelectOption key={asset.id} value={asset.id}>
                      {asset.file_name ?? "项目视频"}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </Field>
              <Field className="max-w-52">
                <FieldLabel htmlFor="short-drama-market">目标市场</FieldLabel>
                <NativeSelect
                  id="short-drama-market"
                  value={market}
                  disabled={locked || change.pending}
                  onChange={(event) => setMarket(event.target.value)}
                >
                  {redrawMarkets.map(([value, label]) => (
                    <NativeSelectOption key={value} value={value}>
                      {label}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </Field>
              <Field className="max-w-sm">
                <FieldLabel htmlFor="short-drama-style">转绘风格</FieldLabel>
                <Input
                  id="short-drama-style"
                  value={style}
                  maxLength={200}
                  disabled={locked || change.pending}
                  onChange={(event) => setStyle(event.target.value)}
                  placeholder="如复古胶片、二维动画"
                />
              </Field>
              <Button
                variant="outline"
                disabled={locked || change.pending}
                onClick={() => setUpload(true)}
              >
                上传视频素材
              </Button>
              <Button
                disabled={!source || locked || change.pending}
                onClick={() => {
                  if (source) selectSource(source);
                }}
              >
                准备转绘
              </Button>
            </div>
          )}
          {media.error && (
            <div role="alert">
              无法读取项目视频。{" "}
              <Button variant="outline" onClick={() => void media.mutate()}>
                重试
              </Button>
            </div>
          )}
          {change.error && <p role="alert">{change.error}</p>}
          {draft && writable && (
            <RichGeneration
              key={draft.references[0]?.id ?? initialPrompt}
              projectID={project.id}
              presentation="home"
              seed={{
                project,
                capability: "video",
                creationType: "video",
                prompt: "",
                composer: draft,
              }}
              draftID={draftID}
              onDraftReady={setDraftID}
              onLock={setLocked}
              onCloseGuard={(guard) => {
                closeGuard.current = guard;
              }}
              onAdopt={(id, kind) => {
                if (isProjectID(id) && kind === "video") {
                  setResult(id);
                  void media.mutate();
                }
              }}
            />
          )}
          {result && (
            <div className="flex max-w-xl flex-col gap-2">
              <h3 className="font-medium">生成视频</h3>
              <div className="aspect-video">
                <ProjectMediaPreview
                  projectID={project.id}
                  id={result}
                  kind="video"
                  large
                />
              </div>
            </div>
          )}
        </>
      )}
      <div className="rounded-lg border p-4">
        <h3 className="mb-2 font-medium">剪辑和导出</h3>
        <p className="mb-3 text-sm text-muted-foreground">
          在同一短剧画布添加时间线节点，导入项目素材，编排片段并导出。3D
          导演台也可从画布添加。
        </p>
        <Button asChild variant="outline">
          <Link href={canvas}>打开短剧画布</Link>
        </Button>
      </div>
      {upload && (
        <UploadDialog
          projectID={project.id}
          initialFile={initialFile}
          onClose={() => {
            setUpload(false);
            setInitialFile(undefined);
          }}
          onUploaded={async (asset) => {
            await media.mutate();
            if (asset && eligibleVideo(asset, project.id))
              setSelected(asset.id!);
          }}
        />
      )}
    </section>
  );
}

function prompt(text: string): PromptDraft {
  const value = emptyPromptDraft();
  value.document = {
    type: "doc",
    content: [{ type: "paragraph", content: [{ type: "text", text }] }],
  };
  return value;
}
