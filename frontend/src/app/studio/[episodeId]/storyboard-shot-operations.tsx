"use client";

import {
  GitMerge,
  Scissors,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
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

import { toAssetReferenceRequest } from "./asset-reference";

type SplitTargetOptions = {
  firstTitle: string;
  secondTitle: string;
  firstDurationMs: number;
};

type MergeTargetOptions = {
  baseVersionId: string;
  title: string;
};

export type ShotTransformSource = {
  shot: API.ShotResponse;
  version: API.ShotSpecVersionResponse;
};

export type MergePreparation = {
  preflight: API.ShotTransformPreflightResponse;
  sources: [ShotTransformSource, ShotTransformSource];
};

function clip(value: string, maxLength: number): string {
  return value.slice(0, maxLength);
}

function clonedReferences(
  references: API.AssetReferenceResponse[],
): API.AssetReferenceRequest[] {
  return references.map(toAssetReferenceRequest);
}

function uniqueStrings(values: Array<string | null | undefined>): string[] {
  return [...new Set(values.filter((value): value is string => Boolean(value)))];
}

function joinedText(
  values: Array<string | null | undefined>,
  maxLength: number,
): string | null {
  const joined = uniqueStrings(values).join("；");
  return joined ? clip(joined, maxLength) : null;
}

export function buildSplitTargets(
  source: API.ShotSpecVersionResponse,
  options: SplitTargetOptions,
): [API.TargetShotSpecRequest, API.TargetShotSpecRequest] {
  const firstSpec = structuredClone(source.spec);
  const secondSpec = structuredClone(source.spec);
  const sourceDuration = source.spec.duration_ms ?? 3_000;
  firstSpec.duration_ms = options.firstDurationMs;
  firstSpec.narrative.purpose = clip(
    `${source.spec.narrative.purpose}（前段）`,
    500,
  );
  secondSpec.duration_ms = sourceDuration - options.firstDurationMs;
  secondSpec.narrative.purpose = clip(
    `${source.spec.narrative.purpose}（后段）`,
    500,
  );
  secondSpec.script_reference.dialogue_ids = [];
  secondSpec.dialogue_or_narration = [];
  return [
    {
      title: options.firstTitle.trim(),
      spec: firstSpec,
      asset_references: clonedReferences(source.asset_references),
    },
    {
      title: options.secondTitle.trim(),
      spec: secondSpec,
      asset_references: clonedReferences(source.asset_references),
    },
  ];
}

function mergedAssetReferences(
  base: API.ShotSpecVersionResponse,
  other: API.ShotSpecVersionResponse,
  sameScene: boolean,
): API.AssetReferenceRequest[] {
  const result = clonedReferences(base.asset_references);
  if (!sameScene) return result;
  const slotKeys = new Set(result.map((reference) => reference.slot_key));
  const uniqueRoles = new Set(
    result
      .filter((reference) =>
        ["location", "visual_style"].includes(reference.role),
      )
      .map((reference) => reference.role),
  );
  for (const reference of other.asset_references) {
    if (slotKeys.has(reference.slot_key)) continue;
    if (
      ["location", "visual_style"].includes(reference.role) &&
      uniqueRoles.has(reference.role)
    ) {
      continue;
    }
    result.push({ ...reference });
    slotKeys.add(reference.slot_key);
    uniqueRoles.add(reference.role);
  }
  return result;
}

export function buildMergeTarget(
  first: API.ShotSpecVersionResponse,
  second: API.ShotSpecVersionResponse,
  options: MergeTargetOptions,
): API.TargetShotSpecRequest {
  const base = options.baseVersionId === second.id ? second : first;
  const other = base.id === first.id ? second : first;
  const sameScene =
    first.spec.script_reference.scene_id ===
    second.spec.script_reference.scene_id;
  const spec = structuredClone(base.spec);
  const actionDescriptions = [first, second]
    .flatMap((version) =>
      version.spec.action_beats.map((beat) => beat.description),
    )
    .slice(0, 8);
  spec.action_beats = actionDescriptions.map((description, index) => ({
    beat_key: `beat-${index + 1}`,
    order: index + 1,
    description,
  }));

  const dialogueIds = sameScene
    ? uniqueStrings([
        ...(first.spec.script_reference.dialogue_ids ?? []),
        ...(second.spec.script_reference.dialogue_ids ?? []),
      ])
    : [...(base.spec.script_reference.dialogue_ids ?? [])];
  const dialogueById = new Map(
    [first, second].flatMap((version) =>
      (version.spec.dialogue_or_narration ?? []).map((dialogue) => [
        dialogue.source_dialogue_id,
        dialogue,
      ] as const),
    ),
  );
  spec.script_reference.dialogue_ids = dialogueIds;
  spec.dialogue_or_narration = dialogueIds.map((dialogueId, index) => {
    const dialogue = dialogueById.get(dialogueId);
    return {
      source_dialogue_id: dialogueId,
      beat_key: spec.action_beats[Math.min(index, spec.action_beats.length - 1)]
        .beat_key,
      speaker_subject_key: dialogue?.speaker_subject_key ?? null,
      render_as_audio: dialogue?.render_as_audio ?? false,
      performance_note: dialogue?.performance_note ?? null,
    };
  });
  spec.duration_ms =
    (first.spec.duration_ms ?? 3_000) + (second.spec.duration_ms ?? 3_000);
  spec.narrative.purpose = clip(
    `${first.spec.narrative.purpose}；${second.spec.narrative.purpose}`,
    500,
  );
  spec.narrative.continuity_note = joinedText(
    [
      first.spec.narrative.continuity_note,
      second.spec.narrative.continuity_note,
    ],
    500,
  );
  spec.visual.composition = clip(
    `${first.spec.visual.composition}；${second.spec.visual.composition}`,
    1_000,
  );
  spec.visual.mood_lighting = clip(
    `${first.spec.visual.mood_lighting}；${second.spec.visual.mood_lighting}`,
    1_000,
  );
  const placementBySubject = new Map(
    [
      ...(base.spec.visual.subject_placements ?? []),
      ...(sameScene ? (other.spec.visual.subject_placements ?? []) : []),
    ].map((placement) => [placement.subject_key, placement]),
  );
  spec.visual.subject_placements = [...placementBySubject.values()].slice(0, 16);
  spec.audio_intent = {
    ambient: joinedText(
      [first.spec.audio_intent?.ambient, second.spec.audio_intent?.ambient],
      1_000,
    ),
    sound_effects: uniqueStrings([
      ...(first.spec.audio_intent?.sound_effects ?? []),
      ...(second.spec.audio_intent?.sound_effects ?? []),
    ]).slice(0, 8),
  };
  spec.generation_intent.keyframe_notes = joinedText(
    [
      first.spec.generation_intent.keyframe_notes,
      second.spec.generation_intent.keyframe_notes,
    ],
    2_000,
  );
  return {
    title: options.title.trim(),
    spec,
    asset_references: mergedAssetReferences(base, other, sameScene),
  };
}

function evidenceCount(evidence: API.DownstreamEvidenceResponse): number {
  return (
    (evidence.generation_request_ids ?? []).length +
    (evidence.candidate_ids ?? []).length +
    (evidence.review_ids ?? []).length +
    (evidence.issue_ids ?? []).length +
    (evidence.timeline_source_ids ?? []).length
  );
}

export function SplitShotDialog({
  busy,
  orderHash,
  source,
  onApply,
  onPreflight,
}: {
  busy: boolean;
  orderHash: string;
  source: ShotTransformSource;
  onApply: (shotId: string, request: API.SplitShotRequest) => Promise<boolean>;
  onPreflight: (
    shotId: string,
    request: API.SplitPreflightRequest,
  ) => Promise<API.ShotTransformPreflightResponse | undefined>;
}) {
  const [open, setOpen] = useState(false);
  const [preflight, setPreflight] =
    useState<API.ShotTransformPreflightResponse>();
  const [firstTitle, setFirstTitle] = useState(
    clip(`${source.shot.title} · 前段`, 200),
  );
  const [secondTitle, setSecondTitle] = useState(
    clip(`${source.shot.title} · 后段`, 200),
  );
  const [firstDuration, setFirstDuration] = useState(
    Math.max(
      500,
      Math.floor((source.version.spec.duration_ms ?? 3_000) / 2 / 100) * 100,
    ),
  );
  const totalDuration = source.version.spec.duration_ms ?? 3_000;
  const secondDuration = totalDuration - firstDuration;
  const durationValid =
    Number.isInteger(firstDuration) &&
    firstDuration >= 500 &&
    secondDuration >= 500;

  function changeOpen(nextOpen: boolean) {
    setOpen(nextOpen);
    if (!nextOpen) setPreflight(undefined);
  }

  async function prepare() {
    const result = await onPreflight(source.shot.id, {
      expected_source_spec_version_id: source.version.id,
      expected_order_hash: orderHash,
    });
    setPreflight(result);
  }

  async function apply() {
    if (!preflight || !durationValid) return;
    const succeeded = await onApply(source.shot.id, {
      expected_source_spec_version_id: source.version.id,
      expected_order_hash: preflight.order_hash,
      impact_hash: preflight.impact_hash,
      idempotency_key: `studio-split:${source.shot.id}:${crypto.randomUUID()}`,
      targets: buildSplitTargets(source.version, {
        firstTitle,
        secondTitle,
        firstDurationMs: firstDuration,
      }),
    });
    if (succeeded) changeOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button disabled={busy || !source.version} type="button" variant="outline">
          <Scissors aria-hidden="true" />拆分
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>拆分镜头</DialogTitle>
          <DialogDescription>
            生成两个新镜头并归档来源镜头；历史规格和下游证据不会迁移。
          </DialogDescription>
        </DialogHeader>
        {totalDuration < 1_000 ? (
          <Alert variant="destructive">
            <AlertTitle>当前时长不能拆分</AlertTitle>
            <AlertDescription>两个目标镜头都必须至少为 500 毫秒。</AlertDescription>
          </Alert>
        ) : preflight ? (
          <div className="grid gap-4">
            <Alert className="border-border bg-muted text-foreground">
              <ShieldCheck aria-hidden="true" />
              <AlertTitle>影响已固定</AlertTitle>
              <AlertDescription>
                检出 {evidenceCount(preflight.downstream_evidence)} 条下游证据；确认后只保留历史，不迁移到新镜头。
              </AlertDescription>
            </Alert>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="splitFirstTitle">前段标题</Label>
                <Input
                  id="splitFirstTitle"
                  maxLength={200}
                  onChange={(event) => setFirstTitle(event.target.value)}
                  value={firstTitle}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="splitSecondTitle">后段标题</Label>
                <Input
                  id="splitSecondTitle"
                  maxLength={200}
                  onChange={(event) => setSecondTitle(event.target.value)}
                  value={secondTitle}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="splitFirstDuration">前段时长（毫秒）</Label>
                <Input
                  id="splitFirstDuration"
                  max={totalDuration - 500}
                  min={500}
                  onChange={(event) => setFirstDuration(Number(event.target.value))}
                  step={100}
                  type="number"
                  value={firstDuration}
                />
              </div>
              <div className="grid gap-2">
                <Label>后段时长</Label>
                <div className="flex h-8 items-center rounded-lg border bg-muted/40 px-2.5 text-sm">
                  {secondDuration} 毫秒
                </div>
              </div>
            </div>
          </div>
        ) : (
          <Alert>
            <ShieldCheck aria-hidden="true" />
            <AlertTitle>先检查影响</AlertTitle>
            <AlertDescription>
              预检会固定当前规格、顺序和下游证据；任何变化都会要求重新检查。
            </AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          {preflight ? (
            <Button
              disabled={busy || !durationValid || !firstTitle.trim() || !secondTitle.trim()}
              onClick={() => void apply()}
              type="button"
            >
              确认拆分
            </Button>
          ) : (
            <Button
              disabled={busy || totalDuration < 1_000}
              onClick={() => void prepare()}
              type="button"
            >
              检查拆分影响
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function MergeShotsDialog({
  busy,
  candidates,
  source,
  onApply,
  onPrepare,
}: {
  busy: boolean;
  candidates: API.ShotResponse[];
  source: ShotTransformSource;
  onApply: (request: API.MergeShotRequest) => Promise<boolean>;
  onPrepare: (
    source: API.ShotResponse,
    partner: API.ShotResponse,
  ) => Promise<MergePreparation | undefined>;
}) {
  const [open, setOpen] = useState(false);
  const [partnerId, setPartnerId] = useState(candidates[0]?.id ?? "");
  const [preparation, setPreparation] = useState<MergePreparation>();
  const [title, setTitle] = useState(`${source.shot.title} · 合并镜头`);
  const [baseVersionId, setBaseVersionId] = useState(source.version.id);
  const partner = candidates.find((candidate) => candidate.id === partnerId);
  const totalDuration = preparation
    ? preparation.sources.reduce(
        (total, item) => total + (item.version.spec.duration_ms ?? 3_000),
        0,
      )
    : 0;

  function changeOpen(nextOpen: boolean) {
    setOpen(nextOpen);
    if (!nextOpen) setPreparation(undefined);
  }

  async function prepare() {
    if (!partner) return;
    const result = await onPrepare(source.shot, partner);
    if (!result) return;
    setPreparation(result);
    setBaseVersionId(result.sources[0].version.id);
    setTitle(
      clip(
        `${result.sources[0].shot.title} + ${result.sources[1].shot.title}`,
        200,
      ),
    );
  }

  async function apply() {
    if (!preparation) return;
    const [first, second] = preparation.sources;
    const succeeded = await onApply({
      shot_ids: preparation.preflight.source_shot_ids,
      expected_spec_version_ids:
        preparation.preflight.source_spec_version_ids,
      expected_order_hash: preparation.preflight.order_hash,
      impact_hash: preparation.preflight.impact_hash,
      idempotency_key: `studio-merge:${first.shot.id}:${second.shot.id}:${crypto.randomUUID()}`,
      target: buildMergeTarget(first.version, second.version, {
        baseVersionId,
        title,
      }),
    });
    if (succeeded) changeOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button
          disabled={busy || candidates.length === 0}
          title={candidates.length ? undefined : "需要相邻且已有规格的镜头"}
          type="button"
          variant="outline"
        >
          <GitMerge aria-hidden="true" />合并
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>合并相邻镜头</DialogTitle>
          <DialogDescription>
            新镜头使用完整目标规格；两个来源会归档，历史和下游证据保持原位。
          </DialogDescription>
        </DialogHeader>
        {preparation ? (
          <div className="grid gap-4">
            <Alert className="border-border bg-muted text-foreground">
              <ShieldCheck aria-hidden="true" />
              <AlertTitle>影响已固定</AlertTitle>
              <AlertDescription>
                合并时长 {totalDuration} 毫秒，检出 {evidenceCount(preparation.preflight.downstream_evidence)} 条下游证据。
              </AlertDescription>
            </Alert>
            {totalDuration > 15_000 ? (
              <Alert variant="destructive">
                <AlertTitle>合并后时长超限</AlertTitle>
                <AlertDescription>目标镜头不能超过 15,000 毫秒。</AlertDescription>
              </Alert>
            ) : null}
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2 sm:col-span-2">
                <Label htmlFor="mergeTitle">合并后标题</Label>
                <Input
                  id="mergeTitle"
                  maxLength={200}
                  onChange={(event) => setTitle(event.target.value)}
                  value={title}
                />
              </div>
              <div className="grid gap-2 sm:col-span-2">
                <Label>场次与资产基础</Label>
                <Select value={baseVersionId} onValueChange={setBaseVersionId}>
                  <SelectTrigger aria-label="合并规格基础" className="h-10 w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {preparation.sources.map((item) => (
                      <SelectItem key={item.version.id} value={item.version.id}>
                        {item.shot.title} · v{item.version.version_no}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs leading-5 text-muted-foreground">
                  跨场次时只保留所选规格的场次、对白和资产；同场次会合并动作与对白。
                </p>
              </div>
            </div>
          </div>
        ) : (
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label>相邻镜头</Label>
              <Select value={partnerId} onValueChange={setPartnerId}>
                <SelectTrigger aria-label="相邻镜头" className="h-10 w-full">
                  <SelectValue placeholder="选择相邻镜头" />
                </SelectTrigger>
                <SelectContent>
                  {candidates.map((candidate) => (
                    <SelectItem key={candidate.id} value={candidate.id}>
                      {String(candidate.position).padStart(2, "0")} · {candidate.title}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Alert>
              <ShieldCheck aria-hidden="true" />
              <AlertTitle>先检查影响</AlertTitle>
              <AlertDescription>
                只有同一单集内相邻、启用且都已保存规格的两个镜头可以合并。
              </AlertDescription>
            </Alert>
          </div>
        )}
        <DialogFooter>
          {preparation ? (
            <Button
              disabled={busy || totalDuration > 15_000 || !title.trim()}
              onClick={() => void apply()}
              type="button"
            >
              确认合并
            </Button>
          ) : (
            <Button
              disabled={busy || !partner}
              onClick={() => void prepare()}
              type="button"
            >
              检查合并影响
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const deleteBlockerLabels: Record<API.ShotDeleteBlocker["code"], string> = {
  SOURCE_CANDIDATE_EVIDENCE: "镜头保留已确认候选来源，只能归档",
  SPEC_VERSION_EVIDENCE: "镜头已有规格历史，只能归档",
};

export function DeleteShotDialog({
  busy,
  shot,
  onDelete,
  onPreflight,
}: {
  busy: boolean;
  shot: API.ShotResponse;
  onDelete: (shot: API.ShotResponse) => Promise<boolean>;
  onPreflight: (
    shotId: string,
  ) => Promise<API.ShotDeletePreflightResponse | undefined>;
}) {
  const [open, setOpen] = useState(false);
  const [preflight, setPreflight] = useState<API.ShotDeletePreflightResponse>();
  const blockerLabels = preflight?.blockers.map(
    (blocker) => deleteBlockerLabels[blocker.code],
  );

  function changeOpen(nextOpen: boolean) {
    setOpen(nextOpen);
    if (!nextOpen) setPreflight(undefined);
  }

  async function remove() {
    if (!preflight?.allowed) return;
    if (await onDelete(shot)) changeOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button disabled={busy} type="button" variant="outline">
          <Trash2 aria-hidden="true" />删除检查
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>检查是否能永久删除</DialogTitle>
          <DialogDescription>
            只有没有规格、变换或生产证据的空手工镜头才能永久删除。
          </DialogDescription>
        </DialogHeader>
        {preflight ? (
          preflight.allowed ? (
            <Alert className="border-amber-200 bg-amber-50 text-amber-900">
              <Trash2 aria-hidden="true" />
              <AlertTitle>可以永久删除</AlertTitle>
              <AlertDescription>
                删除后镜头身份无法恢复；本操作只适用于空手工镜头。
              </AlertDescription>
            </Alert>
          ) : (
            <Alert>
              <ShieldCheck aria-hidden="true" />
              <AlertTitle>必须保留历史证据</AlertTitle>
              <AlertDescription>{blockerLabels?.join("；")}</AlertDescription>
            </Alert>
          )
        ) : (
          <Alert>
            <ShieldCheck aria-hidden="true" />
            <AlertTitle>服务端预检</AlertTitle>
            <AlertDescription>
              预检会读取当前引用，防止并发新增证据时误删镜头。
            </AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          {preflight?.allowed ? (
            <Button
              disabled={busy}
              onClick={() => void remove()}
              type="button"
              variant="destructive"
            >
              永久删除空镜头
            </Button>
          ) : preflight ? (
            <Button onClick={() => changeOpen(false)} type="button" variant="outline">
              知道了
            </Button>
          ) : (
            <Button
              disabled={busy}
              onClick={() => void onPreflight(shot.id).then(setPreflight)}
              type="button"
            >
              检查删除条件
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
