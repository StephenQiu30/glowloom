"use client";

import {
  Archive,
  ArrowDown,
  ArrowUp,
  CheckCircle2,
  Copy,
  Film,
  GripVertical,
  History,
  PencilLine,
  Plus,
  RotateCcw,
  Save,
  ShieldAlert,
  TriangleAlert,
} from "lucide-react";
import { type FormEvent, useMemo, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/class-names";

import {
  DeleteShotDialog,
  type MergePreparation,
  MergeShotsDialog,
  type ShotTransformSource,
  SplitShotDialog,
} from "./storyboard-shot-operations";

type StoryboardWorkspaceProps = {
  archivedShots: API.ShotResponse[];
  assetBible?: API.AssetBibleResponse;
  busy: boolean;
  confirmedShotCandidates: API.ExtractionCandidateResponse[];
  order: API.ShotOrderResponse;
  readiness?: API.ShotReadinessBatchResponse;
  selectedShotId: string | null;
  structure?: API.ConfirmedStructureResponse;
  versions: API.ShotSpecVersionResponse[];
  onCopy: (shot: API.ShotResponse) => Promise<void>;
  onCreate: (request: API.ShotCreateRequest) => Promise<boolean>;
  onCreateFromCandidate: (
    candidate: API.ExtractionCandidateResponse,
  ) => Promise<boolean>;
  onDelete: (shot: API.ShotResponse) => Promise<boolean>;
  onDeletePreflight: (
    shotId: string,
  ) => Promise<API.ShotDeletePreflightResponse | undefined>;
  onMerge: (request: API.MergeShotRequest) => Promise<boolean>;
  onMergePrepare: (
    source: API.ShotResponse,
    partner: API.ShotResponse,
  ) => Promise<MergePreparation | undefined>;
  onReorder: (shotIds: string[]) => Promise<void>;
  onSaveSpec: (
    shotId: string,
    request: API.ShotSpecCreateRequest,
  ) => Promise<boolean>;
  onSelectShot: (shotId: string) => void;
  onSetCurrentSpec: (
    shot: API.ShotResponse,
    version: API.ShotSpecVersionResponse,
  ) => Promise<void>;
  onSplit: (shotId: string, request: API.SplitShotRequest) => Promise<boolean>;
  onSplitPreflight: (
    shotId: string,
    request: API.SplitPreflightRequest,
  ) => Promise<API.ShotTransformPreflightResponse | undefined>;
  onToggleArchived: (shot: API.ShotResponse) => Promise<void>;
  onUpdate: (shot: API.ShotResponse, title: string) => Promise<boolean>;
};

const shotSizeLabels: Record<API.VisualSpec["shot_size"], string> = {
  extreme_wide: "大远景",
  wide: "远景",
  full: "全景",
  medium: "中景",
  medium_close_up: "中近景",
  close_up: "近景",
  extreme_close_up: "特写",
};

const angleLabels: Record<API.VisualSpec["camera_angle"], string> = {
  eye_level: "平视",
  high: "俯拍",
  low: "仰拍",
  bird_eye: "鸟瞰",
  dutch: "荷兰角",
};

const movementLabels: Record<API.VisualSpec["camera_movement"], string> = {
  static: "固定",
  pan: "横摇",
  tilt: "纵摇",
  dolly: "推拉",
  truck: "横移",
  pedestal: "升降",
  zoom: "变焦",
  handheld: "手持",
  orbit: "环绕",
};

const generationModeLabels: Record<API.GenerationIntent["mode"], string> = {
  keyframe_then_video: "关键帧转视频",
  reference_to_video: "参考图转视频",
  text_to_video: "文本生成视频",
};

const assetKindLabels: Record<API.AssetResponse["kind"], string> = {
  character: "角色",
  location: "场景",
  prop: "道具",
  costume: "服装",
  visual_style: "视觉风格",
  voice: "声音",
};

const readinessLabels: Record<API.ShotReadinessResponse["status"], string> = {
  ready: "可生成",
  blocked: "待完善",
  unavailable: "依赖不可用",
};

const readinessIssueLabels: Record<API.ShotReadinessIssue["code"], string> = {
  CURRENT_SPEC_MISSING: "尚未保存镜头规格",
  SPEC_FIELD_MISSING: "镜头规格字段不完整",
  DURATION_OUT_OF_RANGE: "镜头时长超出支持范围",
  SCRIPT_VERSION_UNAVAILABLE: "已确认剧本版本不可用",
  SCRIPT_REVISION_NOT_CURRENT: "镜头引用的剧本已不是当前版本",
  SOURCE_SCENE_INVALID: "来源场次与已确认剧本不一致",
  SOURCE_DIALOGUE_INVALID: "对白引用与已确认场次不一致",
  LOCATION_REFERENCE_MISSING: "需要且只能固定一个场景资产版本",
  CHARACTER_REFERENCE_MISSING: "画面角色尚未固定对应的角色资产版本",
  VOICE_REFERENCE_MISSING: "启用语音的对白尚未固定声音资产版本",
  ASSET_KIND_MISMATCH: "资产类型与镜头引用槽位不一致",
  ASSET_VERSION_UNAVAILABLE: "固定的资产版本不可用",
  ASSET_NOT_READY: "固定资产尚未达到生产就绪状态",
  MEDIA_REFERENCE_UNAVAILABLE: "资产所需的媒体版本不可用",
  RIGHTS_BLOCKED: "资产授权尚未满足本次生产用途",
  DEPENDENCY_UNAVAILABLE: "生产依赖暂时不可用",
};

const readinessWarningLabels: Record<API.ShotReadinessWarning["code"], string> = {
  DURATION_ABOVE_RECOMMENDED: "镜头时长超过 8 秒建议值",
  ACTION_DENSITY_HIGH: "镜头动作密度较高",
  STYLE_REFERENCE_MISSING: "尚未固定可选的视觉风格参考",
};

function readinessIssueSummary(issue: API.ShotReadinessIssue): string {
  return readinessIssueLabels[issue.code];
}

function readinessWarningSummary(warning: API.ShotReadinessWarning): string {
  return readinessWarningLabels[warning.code];
}

function readinessClass(status: API.ShotReadinessResponse["status"]): string {
  if (status === "ready") return "border-emerald-200 bg-emerald-50 text-emerald-700";
  if (status === "unavailable") return "border-rose-200 bg-rose-50 text-rose-700";
  return "border-amber-200 bg-amber-50 text-amber-700";
}

function reorderShotIds(
  shots: API.ShotResponse[],
  draggedShotId: string,
  targetShotId: string,
): string[] {
  const shotIds = shots.map((shot) => shot.id);
  const draggedIndex = shotIds.indexOf(draggedShotId);
  const targetIndex = shotIds.indexOf(targetShotId);
  if (draggedIndex < 0 || targetIndex < 0 || draggedIndex === targetIndex) {
    return shotIds;
  }
  const [draggedId] = shotIds.splice(draggedIndex, 1);
  shotIds.splice(targetIndex, 0, draggedId);
  return shotIds;
}

function defaultSpec(
  shot: API.ShotResponse,
  structure?: API.ConfirmedStructureResponse,
): API.ShotSpec {
  const scene = structure?.scenes.find((item) => item.id === shot.source_scene_id);
  return {
    schema_version: 1,
    script_reference: {
      confirmed_script_version_id: shot.source_script_version_id,
      scene_id: shot.source_scene_id,
      dialogue_ids: [],
    },
    narrative: { purpose: shot.title, continuity_note: null },
    visual: {
      shot_size: "medium",
      camera_angle: "eye_level",
      camera_movement: "static",
      composition: "主体位于画面视觉中心，保留明确的前后景层次",
      environment: scene
        ? `${scene.location} · ${scene.time_of_day} · ${scene.summary}`
        : "待补充镜头环境",
      subject_placements: [],
      mood_lighting: "延续项目视觉风格与场景光线",
    },
    action_beats: [
      { beat_key: "beat-1", order: 1, description: "完成本镜头的主要动作" },
    ],
    dialogue_or_narration: [],
    duration_ms: 3_000,
    audio_intent: { ambient: null, sound_effects: [] },
    generation_intent: {
      mode: "text_to_video",
      first_frame: null,
      last_frame: null,
      keyframe_notes: null,
    },
  };
}

function NewShotDialog({
  busy,
  confirmedShotCandidates,
  structure,
  onCreate,
  onCreateFromCandidate,
}: Pick<
  StoryboardWorkspaceProps,
  | "busy"
  | "confirmedShotCandidates"
  | "structure"
  | "onCreate"
  | "onCreateFromCandidate"
>) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [sceneId, setSceneId] = useState(structure?.scenes[0]?.id ?? "");
  const scene = structure?.scenes.find((item) => item.id === sceneId);
  const canCreate = Boolean(structure?.scenes.length);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!structure || !scene || !title.trim()) return;
    const succeeded = await onCreate({
      title: title.trim(),
      source_script_version_id: structure.script_version_id,
      source_scene_id: scene.id,
      creation_key: `studio-shot:${scene.id}:${crypto.randomUUID()}`,
    });
    if (succeeded) {
      setTitle("");
      setOpen(false);
    }
  }

  async function createFromCandidate(candidate: API.ExtractionCandidateResponse) {
    if (await onCreateFromCandidate(candidate)) setOpen(false);
  }

  return (
    <div className="grid justify-items-end gap-1.5">
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>
          <Button disabled={!canCreate || busy}>
            <Plus aria-hidden="true" />新建镜头
          </Button>
        </DialogTrigger>
        <DialogContent className="sm:max-w-lg">
          <form onSubmit={submit}>
            <DialogHeader>
              <DialogTitle>建立镜头</DialogTitle>
              <DialogDescription>
                优先复用已确认的镜头候选，也可以从场次手工建立稳定镜头身份。
              </DialogDescription>
            </DialogHeader>
            <div className="mt-5 grid gap-4">
              {confirmedShotCandidates.length ? (
                <section className="grid gap-2" aria-labelledby="confirmed-shot-candidates">
                  <div>
                    <h3 className="text-sm font-medium" id="confirmed-shot-candidates">
                      已确认的镜头候选
                    </h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      来源与场次已经确认，加入后仍需在工作台完善规格。
                    </p>
                  </div>
                  {confirmedShotCandidates.map((candidate) => {
                    if (candidate.proposal.kind !== "shot") return null;
                    return (
                      <div
                        className="flex items-start justify-between gap-3 rounded-xl border p-3"
                        key={candidate.id}
                      >
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">
                            {candidate.proposal.title}
                          </p>
                          <p className="mt-1 text-xs leading-5 text-muted-foreground">
                            {candidate.proposal.purpose}
                          </p>
                        </div>
                        <Button
                          aria-label={`从候选建立 ${candidate.proposal.title}`}
                          disabled={busy}
                          onClick={() => void createFromCandidate(candidate)}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          加入清单
                        </Button>
                      </div>
                    );
                  })}
                </section>
              ) : null}
              {confirmedShotCandidates.length ? <Separator /> : null}
              <p className="text-sm font-medium">手工镜头</p>
              <div className="grid gap-2">
                <Label htmlFor="newShotTitle">镜头标题</Label>
                <Input
                  id="newShotTitle"
                  aria-label="新镜头标题"
                  maxLength={200}
                  onChange={(event) => setTitle(event.target.value)}
                  placeholder="例如：远处灯箱闪烁"
                  required
                  value={title}
                />
              </div>
              <div className="grid gap-2">
                <Label>来源场次</Label>
                <Select value={sceneId} onValueChange={setSceneId}>
                  <SelectTrigger aria-label="来源场次" className="h-10 w-full">
                    <SelectValue placeholder="选择已确认场次" />
                  </SelectTrigger>
                  <SelectContent>
                    {(structure?.scenes ?? []).map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        第 {item.position} 场 · {item.heading}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {scene ? (
                  <p className="text-xs leading-5 text-muted-foreground">
                    {scene.location} · {scene.time_of_day} · {scene.summary}
                  </p>
                ) : null}
              </div>
            </div>
            <DialogFooter className="mt-5">
              <Button disabled={busy || !scene || !title.trim()} type="submit">
                创建空镜头
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      {!canCreate ? (
        <p className="max-w-xs text-right text-xs leading-5 text-muted-foreground">
          需先确认剧本结构并设为当前版本，才能建立镜头。
        </p>
      ) : null}
    </div>
  );
}

function ShotTitleDialog({
  busy,
  shot,
  onUpdate,
}: Pick<StoryboardWorkspaceProps, "busy" | "onUpdate"> & {
  shot: API.ShotResponse;
}) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState(shot.title);

  function handleOpenChange(nextOpen: boolean) {
    if (nextOpen) setTitle(shot.title);
    setOpen(nextOpen);
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextTitle = title.trim();
    if (!nextTitle || nextTitle === shot.title) return;
    if (await onUpdate(shot, nextTitle)) setOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button
          aria-label="修改镜头标题"
          disabled={busy}
          size="icon-sm"
          type="button"
          variant="ghost"
        >
          <PencilLine aria-hidden="true" />
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>修改镜头标题</DialogTitle>
            <DialogDescription>
              标题用于清单识别，不改变已保存的镜头规格和生产证据。
            </DialogDescription>
          </DialogHeader>
          <div className="mt-5 grid gap-2">
            <Label htmlFor="shotTitle">镜头标题</Label>
            <Input
              id="shotTitle"
              maxLength={200}
              onChange={(event) => setTitle(event.target.value)}
              required
              value={title}
            />
          </div>
          <DialogFooter className="mt-5">
            <Button
              disabled={busy || !title.trim() || title.trim() === shot.title}
              type="submit"
            >
              保存标题
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ShotSpecEditor({
  assetBible,
  busy,
  currentVersion,
  readiness,
  shot,
  structure,
  onSaveSpec,
}: {
  assetBible?: API.AssetBibleResponse;
  busy: boolean;
  currentVersion?: API.ShotSpecVersionResponse;
  readiness?: API.ShotReadinessResponse;
  shot: API.ShotResponse;
  structure?: API.ConfirmedStructureResponse;
  onSaveSpec: StoryboardWorkspaceProps["onSaveSpec"];
}) {
  const initial = currentVersion?.spec ?? defaultSpec(shot, structure);
  const [purpose, setPurpose] = useState(initial.narrative.purpose);
  const [continuity, setContinuity] = useState(
    initial.narrative.continuity_note ?? "",
  );
  const [shotSize, setShotSize] = useState(initial.visual.shot_size);
  const [angle, setAngle] = useState(initial.visual.camera_angle);
  const [movement, setMovement] = useState(initial.visual.camera_movement);
  const [composition, setComposition] = useState(initial.visual.composition);
  const [environment, setEnvironment] = useState(initial.visual.environment);
  const [lighting, setLighting] = useState(initial.visual.mood_lighting);
  const [beats, setBeats] = useState(
    initial.action_beats.map((beat) => beat.description).join("\n"),
  );
  const [duration, setDuration] = useState(initial.duration_ms ?? 3_000);
  const [ambient, setAmbient] = useState(initial.audio_intent?.ambient ?? "");
  const [soundEffects, setSoundEffects] = useState(
    (initial.audio_intent?.sound_effects ?? []).join("，"),
  );
  const [generationMode, setGenerationMode] = useState(
    initial.generation_intent.mode,
  );
  const [keyframeNotes, setKeyframeNotes] = useState(
    initial.generation_intent.keyframe_notes ?? "",
  );
  const [dialogueIds, setDialogueIds] = useState(
    initial.script_reference.dialogue_ids ?? [],
  );
  const [references, setReferences] = useState<API.AssetReferenceRequest[]>(
    currentVersion?.asset_references.map((reference) => ({ ...reference })) ?? [],
  );
  const [subjectPlacements, setSubjectPlacements] = useState<
    API.SubjectPlacement[]
  >(initial.visual.subject_placements ?? []);
  const [dialogueVoiceById, setDialogueVoiceById] = useState<
    Record<string, string | null>
  >(() =>
    Object.fromEntries(
      (initial.dialogue_or_narration ?? []).map((dialogue) => [
        dialogue.source_dialogue_id,
        dialogue.render_as_audio ? dialogue.speaker_subject_key : null,
      ]),
    ),
  );
  const [dialoguePerformanceById, setDialoguePerformanceById] = useState<
    Record<string, string>
  >(() =>
    Object.fromEntries(
      (initial.dialogue_or_narration ?? []).map((dialogue) => [
        dialogue.source_dialogue_id,
        dialogue.performance_note ?? "",
      ]),
    ),
  );
  const [firstFrame, setFirstFrame] = useState(
    initial.generation_intent.first_frame ?? "",
  );
  const [lastFrame, setLastFrame] = useState(
    initial.generation_intent.last_frame ?? "",
  );
  const scene = structure?.scenes.find(
    (item) => item.id === initial.script_reference.scene_id,
  );
  const assetChoices = (assetBible?.items ?? []).flatMap(({ asset, states }) =>
    asset.status === "active"
      ? states.flatMap(({ state, current_version: version }) =>
          state.status === "active" && version ? [{ asset, state, version }] : [],
        )
      : [],
  );
  const assetsByKind = Object.groupBy(
    assetChoices,
    ({ asset }) => asset.kind,
  );
  const assetsByVersion = new Map(
    assetChoices.map((choice) => [choice.version.id, choice] as const),
  );
  const characterReferences = references.filter(
    (reference) => reference.role === "character" && reference.subject_key,
  );
  const voiceReferences = references.filter(
    (reference) => reference.role === "voice" && reference.subject_key,
  );
  const initialDialogueById = new Map(
    (initial.dialogue_or_narration ?? []).map((dialogue) => [
      dialogue.source_dialogue_id,
      dialogue,
    ]),
  );

  function referenceName(reference: API.AssetReferenceRequest): string {
    return (
      assetsByVersion.get(reference.asset_version_id)?.asset.name ??
      `${assetKindLabels[reference.role]} ${reference.asset_version_id.slice(-8)}`
    );
  }

  function toggleDialogue(id: string) {
    setDialogueIds((current) =>
      current.includes(id)
        ? current.filter((item) => item !== id)
        : [...current, id],
    );
  }

  function toggleAsset(choice: (typeof assetChoices)[number]) {
    const { asset, version } = choice;
    const versionId = version.id;
    const existing = references.find(
      (reference) => reference.asset_version_id === versionId,
    );
    if (existing) {
      setReferences((current) =>
        current.filter((reference) => reference.asset_version_id !== versionId),
      );
      if (existing.role === "character" && existing.subject_key) {
        setSubjectPlacements((current) =>
          current.filter(
            (placement) => placement.subject_key !== existing.subject_key,
          ),
        );
      }
      if (existing.role === "voice" && existing.subject_key) {
        setDialogueVoiceById((current) =>
          Object.fromEntries(
            Object.entries(current).map(([dialogueId, subjectKey]) => [
              dialogueId,
              subjectKey === existing.subject_key ? null : subjectKey,
            ]),
          ),
        );
      }
      return;
    }
    const suffix = asset.id.slice(-8);
    const subjectKey =
      asset.kind === "character" || asset.kind === "voice"
        ? `subject-${suffix}`
        : null;
    setReferences((current) => [
      ...current,
      {
        slot_key: `${asset.kind}-${suffix}`,
        role: asset.kind,
        asset_version_id: versionId,
        subject_key: subjectKey,
      },
    ]);
    if (asset.kind === "character" && subjectKey) {
      setSubjectPlacements((current) => [
        ...current.filter((placement) => placement.subject_key !== subjectKey),
        { subject_key: subjectKey, placement: "主体位于画面视觉中心" },
      ]);
    }
  }

  function setSubjectPlacement(subjectKey: string, placement: string) {
    setSubjectPlacements((current) =>
      current.some((item) => item.subject_key === subjectKey)
        ? current.map((item) =>
            item.subject_key === subjectKey ? { ...item, placement } : item,
          )
        : [...current, { subject_key: subjectKey, placement }],
    );
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const beatDescriptions = beats
      .split("\n")
      .map((item) => item.trim())
      .filter(Boolean)
      .slice(0, 8);
    const actionBeats = (beatDescriptions.length
      ? beatDescriptions
      : ["完成本镜头的主要动作"]
    ).map((description, index) => ({
      beat_key: `beat-${index + 1}`,
      order: index + 1,
      description,
    }));
    await onSaveSpec(shot.id, {
      expected_current_spec_version_id: shot.current_spec_version_id,
      spec: {
        schema_version: 1,
        script_reference: {
          confirmed_script_version_id: shot.source_script_version_id,
          scene_id: shot.source_scene_id,
          dialogue_ids: dialogueIds,
        },
        narrative: {
          purpose: purpose.trim(),
          continuity_note: continuity.trim() || null,
        },
        visual: {
          shot_size: shotSize,
          camera_angle: angle,
          camera_movement: movement,
          composition: composition.trim(),
          environment: environment.trim(),
          subject_placements: characterReferences.flatMap((reference) => {
            if (!reference.subject_key) return [];
            const placement = subjectPlacements.find(
              (item) => item.subject_key === reference.subject_key,
            );
            return placement
              ? [{ ...placement, placement: placement.placement.trim() }]
              : [];
          }),
          mood_lighting: lighting.trim(),
        },
        action_beats: actionBeats,
        dialogue_or_narration: dialogueIds.map((dialogueId) => {
          const existing = initialDialogueById.get(dialogueId);
          const speakerSubjectKey = dialogueVoiceById[dialogueId] ?? null;
          const sceneDialogue = scene?.dialogues.find(
            (item) => item.id === dialogueId,
          );
          const performanceNote =
            dialoguePerformanceById[dialogueId] ??
            existing?.performance_note ??
            sceneDialogue?.performance_note ??
            "";
          const existingBeatKey = existing?.beat_key;
          return {
            source_dialogue_id: dialogueId,
            beat_key:
              existingBeatKey &&
              actionBeats.some((beat) => beat.beat_key === existingBeatKey)
                ? existingBeatKey
                : actionBeats[0].beat_key,
            speaker_subject_key: speakerSubjectKey,
            render_as_audio: Boolean(speakerSubjectKey),
            performance_note: performanceNote.trim() || null,
          };
        }),
        duration_ms: duration,
        audio_intent: {
          ambient: ambient.trim() || null,
          sound_effects: soundEffects
            .split(/[，,]/)
            .map((item) => item.trim())
            .filter(Boolean)
            .slice(0, 8),
        },
        generation_intent: {
          mode: generationMode,
          first_frame: firstFrame.trim() || null,
          last_frame: lastFrame.trim() || null,
          keyframe_notes: keyframeNotes.trim() || null,
        },
      },
      asset_references: references,
    });
  }

  return (
    <form className="grid gap-6" onSubmit={submit}>
      {readiness?.blocking_reasons.length ? (
        <Alert className="border-amber-200 bg-amber-50 text-amber-800">
          <ShieldAlert aria-hidden="true" />
          <AlertTitle>当前规格仍有生产阻塞</AlertTitle>
          <AlertDescription>
            {readiness.blocking_reasons
              .map(readinessIssueSummary)
              .join("；")}
          </AlertDescription>
        </Alert>
      ) : readiness?.ready ? (
        <Alert className="border-emerald-200 bg-emerald-50 text-emerald-800">
          <CheckCircle2 aria-hidden="true" />
          <AlertTitle>当前规格可进入生产预检</AlertTitle>
          <AlertDescription>
            准备度由服务端按固定结构、资产版本、媒体与授权实时计算。
          </AlertDescription>
        </Alert>
      ) : null}

      {readiness?.warnings.length ? (
        <Alert className="border-border bg-muted text-foreground">
          <TriangleAlert aria-hidden="true" />
          <AlertTitle>进入生产前需要确认</AlertTitle>
          <AlertDescription>
            <ul className="list-disc space-y-1 pl-4">
              {readiness.warnings.map((warning) => (
                <li key={`${warning.code}:${warning.field_path ?? "general"}`}>
                  {readinessWarningSummary(warning)}
                </li>
              ))}
            </ul>
            <p className="mt-2">
              警告不阻止保存；后续进入生产预检时需要逐项确认。
            </p>
          </AlertDescription>
        </Alert>
      ) : null}

      <section className="grid gap-4" aria-labelledby="narrative-title">
        <div>
          <h3 className="font-medium" id="narrative-title">叙事与时长</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            固定本镜头要完成的叙事任务，不复制整段剧本文本。
          </p>
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="grid gap-2 lg:col-span-2">
            <Label htmlFor="shotPurpose">镜头目的</Label>
            <Textarea
              id="shotPurpose"
              maxLength={500}
              onChange={(event) => setPurpose(event.target.value)}
              required
              value={purpose}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="shotDuration">时长（毫秒）</Label>
            <Input
              id="shotDuration"
              max={15_000}
              min={500}
              onChange={(event) => setDuration(Number(event.target.value))}
              required
              step={100}
              type="number"
              value={duration}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="continuityNote">连续性备注</Label>
            <Input
              id="continuityNote"
              maxLength={500}
              onChange={(event) => setContinuity(event.target.value)}
              placeholder="可选：与前后镜头的方向、动作或光线关系"
              value={continuity}
            />
          </div>
        </div>
      </section>

      <Separator />

      <section className="grid gap-4" aria-labelledby="visual-title">
        <div>
          <h3 className="font-medium" id="visual-title">画面设计</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            使用受控枚举形成供应商无关的镜头语言。
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <div className="grid gap-2">
            <Label>景别</Label>
            <Select
              value={shotSize}
              onValueChange={(value) =>
                setShotSize(value as API.VisualSpec["shot_size"])
              }
            >
              <SelectTrigger aria-label="景别" className="h-10 w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(shotSizeLabels).map(([value, label]) => (
                  <SelectItem key={value} value={value}>{label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label>机位</Label>
            <Select
              value={angle}
              onValueChange={(value) =>
                setAngle(value as API.VisualSpec["camera_angle"])
              }
            >
              <SelectTrigger aria-label="机位" className="h-10 w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(angleLabels).map(([value, label]) => (
                  <SelectItem key={value} value={value}>{label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label>运镜</Label>
            <Select
              value={movement}
              onValueChange={(value) =>
                setMovement(value as API.VisualSpec["camera_movement"])
              }
            >
              <SelectTrigger aria-label="运镜" className="h-10 w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(movementLabels).map(([value, label]) => (
                  <SelectItem key={value} value={value}>{label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2 md:col-span-3">
            <Label htmlFor="shotComposition">构图</Label>
            <Textarea
              id="shotComposition"
              maxLength={1_000}
              onChange={(event) => setComposition(event.target.value)}
              required
              value={composition}
            />
          </div>
          <div className="grid gap-2 md:col-span-2">
            <Label htmlFor="shotEnvironment">环境</Label>
            <Textarea
              id="shotEnvironment"
              maxLength={1_000}
              onChange={(event) => setEnvironment(event.target.value)}
              required
              value={environment}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="shotLighting">情绪与光线</Label>
            <Textarea
              id="shotLighting"
              maxLength={1_000}
              onChange={(event) => setLighting(event.target.value)}
              required
              value={lighting}
            />
          </div>
        </div>
      </section>

      <Separator />

      <section className="grid gap-4" aria-labelledby="beats-title">
        <div>
          <h3 className="font-medium" id="beats-title">动作与对白</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            每行一个动作节拍，最多 8 个；对白引用保持原文稳定 ID。
          </p>
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="actionBeats">动作节拍</Label>
            <Textarea
              className="min-h-32"
              id="actionBeats"
              onChange={(event) => setBeats(event.target.value)}
              value={beats}
            />
          </div>
          <div className="grid content-start gap-2">
            <Label>场次对白 / 旁白</Label>
            {scene?.dialogues.length ? (
              <div className="grid gap-2">
                {scene.dialogues.map((dialogue) => {
                  const selected = dialogueIds.includes(dialogue.id);
                  return (
                    <Button
                      aria-pressed={selected}
                      className="h-auto justify-start whitespace-normal px-3 py-2 text-left"
                      key={dialogue.id}
                      onClick={() => toggleDialogue(dialogue.id)}
                      type="button"
                      variant={selected ? "secondary" : "outline"}
                    >
                      <span>
                        <span className="block font-medium">{dialogue.speaker_candidate}</span>
                        <span className="mt-0.5 block text-xs text-muted-foreground">
                          {dialogue.text}
                        </span>
                      </span>
                    </Button>
                  );
                })}
              </div>
            ) : (
              <p className="rounded-lg bg-muted p-3 text-xs text-muted-foreground">
                当前场次没有可引用对白。
              </p>
            )}
            {dialogueIds.length ? (
              <div className="mt-2 grid gap-3 border-t pt-3">
                {dialogueIds.map((dialogueId) => {
                  const dialogue = scene?.dialogues.find(
                    (item) => item.id === dialogueId,
                  );
                  if (!dialogue) return null;
                  const selectedVoiceSubject =
                    dialogueVoiceById[dialogueId] ?? null;
                  const performanceNote =
                    dialoguePerformanceById[dialogueId] ??
                    initialDialogueById.get(dialogueId)?.performance_note ??
                    dialogue.performance_note ??
                    "";
                  return (
                    <div className="grid gap-2 rounded-xl bg-muted/50 p-3" key={dialogueId}>
                      <div>
                        <p className="text-sm font-medium">
                          {dialogue.speaker_candidate}
                        </p>
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          {dialogue.text}
                        </p>
                      </div>
                      <div className="grid gap-1.5">
                        <Label>对白声音</Label>
                        <div className="flex flex-wrap gap-2">
                          <Button
                            aria-label={`对白 ${dialogue.speaker_candidate} 仅文本`}
                            aria-pressed={!selectedVoiceSubject}
                            onClick={() =>
                              setDialogueVoiceById((current) => ({
                                ...current,
                                [dialogueId]: null,
                              }))
                            }
                            size="sm"
                            type="button"
                            variant={!selectedVoiceSubject ? "secondary" : "outline"}
                          >
                            仅文本
                          </Button>
                          {voiceReferences.map((reference) => {
                            const subjectKey = reference.subject_key;
                            if (!subjectKey) return null;
                            const voiceName = referenceName(reference);
                            const selected = selectedVoiceSubject === subjectKey;
                            return (
                              <Button
                                aria-label={`为对白 ${dialogue.speaker_candidate} 选择声音 ${voiceName}`}
                                aria-pressed={selected}
                                key={reference.slot_key}
                                onClick={() =>
                                  setDialogueVoiceById((current) => ({
                                    ...current,
                                    [dialogueId]: subjectKey,
                                  }))
                                }
                                size="sm"
                                type="button"
                                variant={selected ? "secondary" : "outline"}
                              >
                                {voiceName}
                              </Button>
                            );
                          })}
                        </div>
                        {voiceReferences.length === 0 ? (
                          <p className="text-xs text-muted-foreground">
                            选择声音资产后，才能把这句对白标记为有声。
                          </p>
                        ) : null}
                      </div>
                      <div className="grid gap-1.5">
                        <Label htmlFor={`dialogue-performance-${dialogueId}`}>
                          {dialogue.speaker_candidate}表演提示
                        </Label>
                        <Input
                          id={`dialogue-performance-${dialogueId}`}
                          maxLength={1_000}
                          onChange={(event) =>
                            setDialoguePerformanceById((current) => ({
                              ...current,
                              [dialogueId]: event.target.value,
                            }))
                          }
                          value={performanceNote}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : null}
          </div>
        </div>
      </section>

      <Separator />

      <section className="grid gap-4" aria-labelledby="asset-reference-title">
        <div>
          <h3 className="font-medium" id="asset-reference-title">固定资产版本</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            选择会固定当前 AssetVersion；后续资产升级会创建新的镜头规格版本。
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {(Object.keys(assetKindLabels) as API.AssetResponse["kind"][]).map(
            (kind) => (
              <div className="grid content-start gap-2" key={kind}>
                <Label>{assetKindLabels[kind]}</Label>
                <div className="flex flex-wrap gap-2">
                  {(assetsByKind[kind] ?? []).map((choice) => {
                    const selected = references.some(
                      (reference) =>
                        reference.asset_version_id === choice.version.id,
                    );
                    return (
                      <Button
                        aria-pressed={selected}
                        key={choice.state.id}
                        onClick={() => toggleAsset(choice)}
                        size="sm"
                        type="button"
                        variant={selected ? "secondary" : "outline"}
                      >
                        {choice.asset.name} · {choice.state.label}
                      </Button>
                    );
                  })}
                  {(assetsByKind[kind] ?? []).length === 0 ? (
                    <span className="text-xs text-muted-foreground">暂无可选当前版本</span>
                  ) : null}
                </div>
              </div>
            ),
          )}
        </div>
        {characterReferences.length ? (
          <div className="grid gap-3 rounded-xl border p-4">
            <div>
              <p className="text-sm font-medium">主体站位</p>
              <p className="mt-1 text-xs text-muted-foreground">
                每个画面主体都绑定固定角色版本与稳定 subject key。
              </p>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              {characterReferences.map((reference) => {
                const subjectKey = reference.subject_key;
                if (!subjectKey) return null;
                const characterName = referenceName(reference);
                const placement =
                  subjectPlacements.find(
                    (item) => item.subject_key === subjectKey,
                  )?.placement ?? "";
                return (
                  <div className="grid gap-2" key={reference.slot_key}>
                    <Label htmlFor={`subject-placement-${subjectKey}`}>
                      {characterName}画面位置
                    </Label>
                    <Input
                      id={`subject-placement-${subjectKey}`}
                      maxLength={500}
                      onChange={(event) =>
                        setSubjectPlacement(subjectKey, event.target.value)
                      }
                      placeholder="例如：画面左侧，面向镜头外"
                      required
                      value={placement}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        ) : null}
      </section>

      <Separator />

      <section className="grid gap-4" aria-labelledby="generation-title">
        <div>
          <h3 className="font-medium" id="generation-title">声音与生成意图</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            只描述镜头意图，不暴露模型、供应商参数或费用字段。
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="ambient">环境声</Label>
            <Input
              id="ambient"
              maxLength={1_000}
              onChange={(event) => setAmbient(event.target.value)}
              placeholder="例如：雨声、风声、远处列车声"
              value={ambient}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="soundEffects">音效（逗号分隔）</Label>
            <Input
              id="soundEffects"
              onChange={(event) => setSoundEffects(event.target.value)}
              placeholder="脚步声，灯箱电流声"
              value={soundEffects}
            />
          </div>
          <div className="grid gap-2">
            <Label>生成方式</Label>
            <Select
              value={generationMode}
              onValueChange={(value) =>
                setGenerationMode(value as API.GenerationIntent["mode"])
              }
            >
              <SelectTrigger aria-label="生成方式" className="h-10 w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(generationModeLabels).map(([value, label]) => (
                  <SelectItem key={value} value={value}>{label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="keyframeNotes">关键帧备注</Label>
            <Input
              id="keyframeNotes"
              maxLength={2_000}
              onChange={(event) => setKeyframeNotes(event.target.value)}
              value={keyframeNotes}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="firstFrame">首帧意图</Label>
            <Input
              id="firstFrame"
              maxLength={1_000}
              onChange={(event) => setFirstFrame(event.target.value)}
              placeholder="可选：首帧画面与构图约束"
              value={firstFrame}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="lastFrame">尾帧意图</Label>
            <Input
              id="lastFrame"
              maxLength={1_000}
              onChange={(event) => setLastFrame(event.target.value)}
              placeholder="可选：尾帧落点与连续性约束"
              value={lastFrame}
            />
          </div>
        </div>
      </section>

      <div className="sticky bottom-4 z-10 flex items-center justify-between gap-4 rounded-xl border bg-background/95 p-3 shadow-lg backdrop-blur">
        <div className="text-xs text-muted-foreground">
          {currentVersion
            ? `当前 v${currentVersion.version_no} · 输入 ${currentVersion.input_hash.slice(0, 10)}…`
            : "尚未保存规格版本"}
        </div>
        <Button disabled={busy} type="submit">
          <Save aria-hidden="true" />保存为新版本
        </Button>
      </div>
    </form>
  );
}

export function StoryboardWorkspace({
  archivedShots,
  assetBible,
  busy,
  confirmedShotCandidates,
  order,
  readiness,
  selectedShotId,
  structure,
  versions,
  onCopy,
  onCreate,
  onCreateFromCandidate,
  onDelete,
  onDeletePreflight,
  onMerge,
  onMergePrepare,
  onReorder,
  onSaveSpec,
  onSelectShot,
  onSetCurrentSpec,
  onSplit,
  onSplitPreflight,
  onToggleArchived,
  onUpdate,
}: StoryboardWorkspaceProps) {
  const [draggedShotId, setDraggedShotId] = useState<string | null>(null);
  const [dragOverShotId, setDragOverShotId] = useState<string | null>(null);
  const selectedShot =
    order.items.find((shot) => shot.id === selectedShotId) ?? order.items[0];
  const currentVersion = versions.find(
    (version) => version.id === selectedShot?.current_spec_version_id,
  );
  const readinessByShot = useMemo(
    () => new Map(readiness?.items.map((item) => [item.shot_id, item]) ?? []),
    [readiness?.items],
  );
  const selectedReadiness = selectedShot
    ? readinessByShot.get(selectedShot.id)
    : undefined;
  const transformSource: ShotTransformSource | undefined =
    selectedShot && currentVersion
      ? { shot: selectedShot, version: currentVersion }
      : undefined;
  const mergeCandidates = selectedShot
    ? order.items.filter(
        (shot) =>
          Math.abs(shot.position - selectedShot.position) === 1 &&
          Boolean(shot.current_spec_version_id),
      )
    : [];

  async function moveSelected(direction: -1 | 1) {
    if (!selectedShot) return;
    const index = order.items.findIndex((shot) => shot.id === selectedShot.id);
    const target = index + direction;
    if (target < 0 || target >= order.items.length) return;
    const ids = order.items.map((shot) => shot.id);
    [ids[index], ids[target]] = [ids[target], ids[index]];
    await onReorder(ids);
  }

  async function dropShot(targetShotId: string) {
    const sourceShotId = draggedShotId;
    setDraggedShotId(null);
    setDragOverShotId(null);
    if (busy || !sourceShotId || sourceShotId === targetShotId) return;
    onSelectShot(sourceShotId);
    await onReorder(reorderShotIds(order.items, sourceShotId, targetShotId));
  }

  return (
    <div className="grid gap-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium text-primary">
            <Film className="size-4" aria-hidden="true" />
            S3 · 稳定镜头规格
          </div>
          <h2 className="mt-2 text-2xl font-semibold tracking-[-0.025em]">分镜设计</h2>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
            以已确认剧本结构建立镜头，固定叙事、画面、动作、声音和资产版本。
          </p>
        </div>
        <NewShotDialog
          busy={busy}
          confirmedShotCandidates={confirmedShotCandidates}
          key={structure?.script_version_id ?? "no-structure"}
          structure={structure}
          onCreate={onCreate}
          onCreateFromCandidate={onCreateFromCandidate}
        />
      </div>

      <section className="grid gap-3 sm:grid-cols-4" aria-label="分镜准备度摘要">
        <Card size="sm"><CardHeader><CardDescription>镜头清单</CardDescription><CardTitle>{order.items.length} 个镜头</CardTitle></CardHeader></Card>
        <Card size="sm"><CardHeader><CardDescription>可进入生产</CardDescription><CardTitle className="text-emerald-700">{readiness?.summary.ready ?? 0} 可生成</CardTitle></CardHeader></Card>
        <Card size="sm"><CardHeader><CardDescription>需要完善</CardDescription><CardTitle className="text-amber-700">{readiness?.summary.blocked ?? 0} 个阻塞</CardTitle></CardHeader></Card>
        <Card size="sm"><CardHeader><CardDescription>依赖故障</CardDescription><CardTitle className="text-rose-700">{readiness?.summary.unavailable ?? 0} 个不可用</CardTitle></CardHeader></Card>
      </section>

      <div className="grid min-w-0 items-start gap-5 xl:grid-cols-[320px_minmax(0,1fr)]">
        <div className="grid gap-4 xl:sticky xl:top-24">
          <Card>
            <CardHeader>
              <CardTitle>镜头顺序</CardTitle>
              <CardDescription>
                拖动把手调整；键盘可使用右侧上移/下移。顺序由服务端 order hash 并发保护。
              </CardDescription>
            </CardHeader>
            <CardContent aria-label="镜头顺序列表" className="grid gap-2" role="list">
              {order.items.length ? (
                order.items.map((shot) => {
                  const shotReadiness = readinessByShot.get(shot.id);
                  const active = shot.id === selectedShot?.id;
                  return (
                    <div
                      aria-label={`镜头 ${shot.title} 顺序项`}
                      className={cn(
                        "flex items-stretch gap-1 rounded-xl [content-visibility:auto]",
                        dragOverShotId === shot.id &&
                          draggedShotId !== shot.id &&
                          "bg-primary/5 ring-2 ring-primary/30",
                      )}
                      key={shot.id}
                      onDragOver={(event) => {
                        if (busy || !draggedShotId) return;
                        event.preventDefault();
                        if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
                        setDragOverShotId(shot.id);
                      }}
                      onDrop={(event) => {
                        event.preventDefault();
                        void dropShot(shot.id);
                      }}
                      role="listitem"
                    >
                      <Button
                        aria-label={`拖动镜头 ${shot.title}`}
                        className="h-auto w-8 cursor-grab self-stretch rounded-lg px-0 active:cursor-grabbing"
                        disabled={busy}
                        draggable={!busy}
                        onDragEnd={() => {
                          setDraggedShotId(null);
                          setDragOverShotId(null);
                        }}
                        onDragStart={(event) => {
                          if (event.dataTransfer) {
                            event.dataTransfer.effectAllowed = "move";
                            event.dataTransfer.setData("text/plain", shot.id);
                          }
                          setDraggedShotId(shot.id);
                        }}
                        size="icon-sm"
                        title="拖动排序"
                        type="button"
                        variant="ghost"
                      >
                        <GripVertical aria-hidden="true" />
                      </Button>
                      <Button
                        className={cn(
                          "h-auto min-w-0 flex-1 justify-start gap-3 whitespace-normal px-3 py-3 text-left",
                          active && "ring-2 ring-primary/25",
                        )}
                        onClick={() => onSelectShot(shot.id)}
                        type="button"
                        variant={active ? "secondary" : "ghost"}
                      >
                        <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-background text-xs font-semibold ring-1 ring-foreground/10">
                          {String(shot.position).padStart(2, "0")}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="block truncate font-medium">{shot.title}</span>
                          <span className="mt-1 flex items-center gap-2">
                            <Badge
                              className={shotReadiness ? readinessClass(shotReadiness.status) : undefined}
                              variant="outline"
                            >
                              {shotReadiness ? readinessLabels[shotReadiness.status] : "读取中"}
                            </Badge>
                            <span className="text-xs text-muted-foreground">
                              {shot.current_spec_version_id ? "已有规格" : "空镜头"}
                            </span>
                          </span>
                          {shotReadiness?.blocking_reasons[0] ? (
                            <span className="mt-1.5 block text-xs text-amber-700">
                              {readinessIssueSummary(
                                shotReadiness.blocking_reasons[0],
                              )}
                            </span>
                          ) : null}
                        </span>
                      </Button>
                    </div>
                  );
                })
              ) : (
                <div className="grid min-h-40 place-items-center rounded-xl border border-dashed p-5 text-center">
                  <div>
                    <Film className="mx-auto size-6 text-muted-foreground" aria-hidden="true" />
                    <p className="mt-2 text-sm font-medium">还没有镜头</p>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      从已确认场次建立第一个手工镜头。
                    </p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          {archivedShots.length ? (
            <Card size="sm">
              <CardHeader>
                <CardTitle className="flex items-center gap-2"><History className="size-4" aria-hidden="true" />已归档</CardTitle>
                <CardDescription>历史规格与证据仍保留</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-2">
                {archivedShots.map((shot) => (
                  <div className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2" key={shot.id}>
                    <span className="min-w-0 truncate text-sm">{shot.title}</span>
                    <Button
                      aria-label={`恢复${shot.title}`}
                      disabled={busy}
                      onClick={() => void onToggleArchived(shot)}
                      size="icon-sm"
                      type="button"
                      variant="ghost"
                    >
                      <RotateCcw aria-hidden="true" />
                    </Button>
                  </div>
                ))}
              </CardContent>
            </Card>
          ) : null}
        </div>

        {selectedShot ? (
          <Card className="min-w-0">
            <CardHeader className="border-b">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <CardDescription>镜头 {String(selectedShot.position).padStart(2, "0")}</CardDescription>
                  <div className="mt-1 flex items-center gap-1">
                    <CardTitle className="text-xl">{selectedShot.title}</CardTitle>
                    <ShotTitleDialog
                      busy={busy}
                      key={`${selectedShot.id}:${selectedShot.revision}`}
                      shot={selectedShot}
                      onUpdate={onUpdate}
                    />
                  </div>
                  <div className="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground">
                    <span>revision {selectedShot.revision}</span>
                    <span>·</span>
                    <span>{versions.length} 个历史规格</span>
                    {selectedReadiness ? (
                      <>
                        <span>·</span>
                        <span>{readinessLabels[selectedReadiness.status]}</span>
                      </>
                    ) : null}
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    aria-label="上移镜头"
                    disabled={busy || selectedShot.position === 1}
                    onClick={() => void moveSelected(-1)}
                    size="icon"
                    type="button"
                    variant="outline"
                  >
                    <ArrowUp aria-hidden="true" />
                  </Button>
                  <Button
                    aria-label="下移镜头"
                    disabled={busy || selectedShot.position === order.items.length}
                    onClick={() => void moveSelected(1)}
                    size="icon"
                    type="button"
                    variant="outline"
                  >
                    <ArrowDown aria-hidden="true" />
                  </Button>
                  <Button
                    disabled={busy || !selectedShot.current_spec_version_id}
                    onClick={() => void onCopy(selectedShot)}
                    type="button"
                    variant="outline"
                  >
                    <Copy aria-hidden="true" />复制镜头
                  </Button>
                  {transformSource ? (
                    <>
                      <SplitShotDialog
                        busy={busy}
                        key={`split:${selectedShot.id}:${transformSource.version.id}:${order.order_hash}`}
                        orderHash={order.order_hash}
                        source={transformSource}
                        onApply={onSplit}
                        onPreflight={onSplitPreflight}
                      />
                      <MergeShotsDialog
                        busy={busy}
                        candidates={mergeCandidates}
                        key={`merge:${selectedShot.id}:${transformSource.version.id}:${order.order_hash}`}
                        source={transformSource}
                        onApply={onMerge}
                        onPrepare={onMergePrepare}
                      />
                    </>
                  ) : null}
                  <Button
                    disabled={busy}
                    onClick={() => void onToggleArchived(selectedShot)}
                    type="button"
                    variant="outline"
                  >
                    <Archive aria-hidden="true" />归档镜头
                  </Button>
                  <DeleteShotDialog
                    busy={busy}
                    key={`delete:${selectedShot.id}:${selectedShot.revision}:${order.order_hash}`}
                    shot={selectedShot}
                    onDelete={onDelete}
                    onPreflight={onDeletePreflight}
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent className="grid gap-6">
              {versions.length ? (
                <section aria-labelledby="shot-history-title">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <h3 className="font-medium" id="shot-history-title">
                        历史规格
                      </h3>
                      <p className="mt-1 text-xs text-muted-foreground">
                        切换 current 只改变后续入口，不覆盖任何历史版本。
                      </p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {versions.map((version) => {
                        const current = version.id === currentVersion?.id;
                        return (
                          <Button
                            disabled={busy || current}
                            key={version.id}
                            onClick={() =>
                              void onSetCurrentSpec(selectedShot, version)
                            }
                            size="sm"
                            type="button"
                            variant={current ? "secondary" : "outline"}
                          >
                            v{version.version_no}
                            {current ? " · 当前" : " · 设为当前"}
                          </Button>
                        );
                      })}
                    </div>
                  </div>
                  <Separator className="mt-5" />
                </section>
              ) : null}
              <ShotSpecEditor
                assetBible={assetBible}
                busy={busy}
                currentVersion={currentVersion}
                key={`${selectedShot.id}:${currentVersion?.id ?? "draft"}`}
                readiness={selectedReadiness}
                shot={selectedShot}
                structure={structure}
                onSaveSpec={onSaveSpec}
              />
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardContent className="grid min-h-96 place-items-center text-center">
              <div>
                <Film className="mx-auto size-8 text-muted-foreground" aria-hidden="true" />
                <h3 className="mt-3 font-medium">建立镜头后开始设计</h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  规格将以不可变版本保存，历史版本不会被覆盖。
                </p>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
