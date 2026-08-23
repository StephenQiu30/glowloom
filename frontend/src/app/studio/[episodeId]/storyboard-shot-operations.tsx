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
import {
  narrativeReferenceKey,
  toNarrativeReferenceInput,
} from "./narrative-reference";

type SplitTargetOptions = {
  firstTitle: string;
  secondTitle: string;
  firstDurationMs: number;
  firstActionCount: number;
  firstDialogueCount: number;
  firstNarrativeReferenceIds: string[];
};

type MergeTargetOptions = {
  baseVersionId: string;
  title: string;
};

export type ShotTransformSource = {
  narrativeReferences: API.NarrativeReferenceResponse[];
  narrativeUnits: API.UnitCoverageResponse[];
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

function splitBoundaryError(
  source: API.ShotSpecVersionResponse,
  firstActionCount: number,
  firstDialogueCount: number,
): string | null {
  const actionCount = source.spec.action_beats.length;
  const dialogueIds = source.spec.script_reference.dialogue_ids ?? [];
  if (
    !Number.isInteger(firstActionCount) ||
    firstActionCount < 1 ||
    firstActionCount >= actionCount
  ) {
    return "来源至少需要两个动作，并明确前段包含几个动作";
  }
  if (
    !Number.isInteger(firstDialogueCount) ||
    firstDialogueCount < 0 ||
    firstDialogueCount > dialogueIds.length
  ) {
    return "前段对白数超出来源对白范围";
  }
  const firstBeatKeys = new Set(
    source.spec.action_beats
      .slice(0, firstActionCount)
      .map((beat) => beat.beat_key),
  );
  const firstDialogueIds = new Set(dialogueIds.slice(0, firstDialogueCount));
  for (const item of source.spec.dialogue_or_narration ?? []) {
    if (!item.beat_key) continue;
    if (
      firstDialogueIds.has(item.source_dialogue_id) !==
      firstBeatKeys.has(item.beat_key)
    ) {
      return "对白必须与它引用的动作分配到同一段";
    }
  }
  return null;
}

function suggestedDialogueBoundary(
  source: API.ShotSpecVersionResponse,
  firstActionCount: number,
): number {
  const dialogueCount =
    source.spec.script_reference.dialogue_ids?.length ?? 0;
  const actionCount = source.spec.action_beats.length;
  const preferred = Math.round(
    dialogueCount * (firstActionCount / Math.max(actionCount, 1)),
  );
  return Array.from({ length: dialogueCount + 1 }, (_, value) => value)
    .sort(
      (left, right) =>
        Math.abs(left - preferred) - Math.abs(right - preferred),
    )
    .find(
      (candidate) =>
        splitBoundaryError(source, firstActionCount, candidate) === null,
    ) ?? preferred;
}

function suggestedSplitBoundary(source: API.ShotSpecVersionResponse): {
  firstActionCount: number;
  firstDialogueCount: number;
} {
  const actionCount = source.spec.action_beats.length;
  const preferredAction = Math.max(1, Math.floor(actionCount / 2));
  const candidates = Array.from(
    { length: Math.max(0, actionCount - 1) },
    (_, index) => index + 1,
  ).sort(
    (left, right) =>
      Math.abs(left - preferredAction) - Math.abs(right - preferredAction),
  );
  for (const firstActionCount of candidates) {
    const firstDialogueCount = suggestedDialogueBoundary(
      source,
      firstActionCount,
    );
    if (
      splitBoundaryError(
        source,
        firstActionCount,
        firstDialogueCount,
      ) === null
    ) {
      return { firstActionCount, firstDialogueCount };
    }
  }
  return { firstActionCount: preferredAction, firstDialogueCount: 0 };
}

export function buildSplitTargets(
  source: API.ShotSpecVersionResponse,
  narrativeReferences: API.NarrativeReferenceResponse[],
  options: SplitTargetOptions,
): [API.TargetShotSpecRequest, API.TargetShotSpecRequest] {
  const firstSpec = structuredClone(source.spec);
  const secondSpec = structuredClone(source.spec);
  const sourceDuration = source.spec.duration_ms ?? 3_000;
  const boundaryError = splitBoundaryError(
    source,
    options.firstActionCount,
    options.firstDialogueCount,
  );
  if (boundaryError) throw new Error(boundaryError);
  const sourceDialogueIds = source.spec.script_reference.dialogue_ids ?? [];
  const firstDialogueIds = sourceDialogueIds.slice(
    0,
    options.firstDialogueCount,
  );
  const secondDialogueIds = sourceDialogueIds.slice(
    options.firstDialogueCount,
  );
  const firstDialogueSet = new Set(firstDialogueIds);
  const secondDialogueSet = new Set(secondDialogueIds);
  firstSpec.duration_ms = options.firstDurationMs;
  firstSpec.action_beats = source.spec.action_beats
    .slice(0, options.firstActionCount)
    .map((beat, index) => ({ ...beat, order: index + 1 }));
  secondSpec.action_beats = source.spec.action_beats
    .slice(options.firstActionCount)
    .map((beat, index) => ({ ...beat, order: index + 1 }));
  firstSpec.script_reference.dialogue_ids = firstDialogueIds;
  secondSpec.script_reference.dialogue_ids = secondDialogueIds;
  firstSpec.dialogue_or_narration = (
    source.spec.dialogue_or_narration ?? []
  ).filter((item) => firstDialogueSet.has(item.source_dialogue_id));
  secondSpec.dialogue_or_narration = (
    source.spec.dialogue_or_narration ?? []
  ).filter((item) => secondDialogueSet.has(item.source_dialogue_id));
  const recombinedDialogueIds = [
    ...firstSpec.dialogue_or_narration,
    ...secondSpec.dialogue_or_narration,
  ].map((item) => item.source_dialogue_id);
  if (
    JSON.stringify(recombinedDialogueIds) !==
    JSON.stringify(
      (source.spec.dialogue_or_narration ?? []).map(
        (item) => item.source_dialogue_id,
      ),
    )
  ) {
    throw new Error("来源对白顺序不能由一个安全分界表达");
  }
  secondSpec.duration_ms = sourceDuration - options.firstDurationMs;
  const knownReferenceIds = new Set(
    narrativeReferences.map((reference) => reference.id),
  );
  const firstReferenceIds = new Set(options.firstNarrativeReferenceIds);
  if (
    firstReferenceIds.size !== options.firstNarrativeReferenceIds.length ||
    [...firstReferenceIds].some((id) => !knownReferenceIds.has(id))
  ) {
    throw new Error("拆分叙事来源分配包含未知或重复关系");
  }
  const firstNarrativeReferences = narrativeReferences
    .filter((reference) => firstReferenceIds.has(reference.id))
    .map(toNarrativeReferenceInput);
  const secondNarrativeReferences = narrativeReferences
    .filter((reference) => !firstReferenceIds.has(reference.id))
    .map(toNarrativeReferenceInput);
  return [
    {
      title: options.firstTitle.trim(),
      spec: firstSpec,
      asset_references: clonedReferences(source.asset_references),
      narrative_references: firstNarrativeReferences,
    },
    {
      title: options.secondTitle.trim(),
      spec: secondSpec,
      asset_references: clonedReferences(source.asset_references),
      narrative_references: secondNarrativeReferences,
    },
  ];
}

function uniqueSlotKey(
  preferred: string,
  occupied: Set<string>,
): string {
  if (!occupied.has(preferred)) return preferred;
  for (let suffix = 2; suffix <= 100; suffix += 1) {
    const ending = `-${suffix}`;
    const candidate = `${preferred.slice(0, 100 - ending.length)}${ending}`;
    if (!occupied.has(candidate)) return candidate;
  }
  throw new Error("合并资产槽位无法生成唯一名称");
}

function referenceIdentity(
  reference: API.AssetReferenceRequest,
): string {
  return JSON.stringify([
    reference.role,
    reference.asset_version_id,
    reference.subject_key ?? null,
  ]);
}

function mergedAssetReferences(
  base: API.ShotSpecVersionResponse,
  other: API.ShotSpecVersionResponse,
): API.AssetReferenceRequest[] {
  const result: API.AssetReferenceRequest[] = [];
  const occupied = new Set<string>();
  const identities = new Set<string>();
  for (const reference of [base, other].flatMap((version) =>
    clonedReferences(version.asset_references),
  )) {
    const identity = referenceIdentity(reference);
    if (identities.has(identity)) continue;
    if (result.length >= 100) {
      throw new Error("合并后不能表示超过 100 个资产引用");
    }
    const slotKey = uniqueSlotKey(reference.slot_key, occupied);
    result.push({ ...reference, slot_key: slotKey });
    occupied.add(slotKey);
    identities.add(identity);
  }
  return result;
}

function mergeSourceError(
  first: API.ShotSpecVersionResponse,
  second: API.ShotSpecVersionResponse,
): string | null {
  if (
    first.spec.script_reference.confirmed_script_version_id !==
    second.spec.script_reference.confirmed_script_version_id
  ) {
    return "只能合并同一确认剧本版本的镜头";
  }
  if (
    first.spec.script_reference.scene_id !==
    second.spec.script_reference.scene_id
  ) {
    return "只能合并同一场次的镜头";
  }
  const duration =
    (first.spec.duration_ms ?? 3_000) +
    (second.spec.duration_ms ?? 3_000);
  if (duration > 15_000) return "合并后的镜头不能超过 15,000 毫秒";
  const actionCount =
    first.spec.action_beats.length + second.spec.action_beats.length;
  if (actionCount > 8) return "合并后不能超过 8 个动作";
  const dialogueIds = [
    ...(first.spec.script_reference.dialogue_ids ?? []),
    ...(second.spec.script_reference.dialogue_ids ?? []),
  ];
  const dialogueCount =
    (first.spec.dialogue_or_narration?.length ?? 0) +
    (second.spec.dialogue_or_narration?.length ?? 0);
  if (dialogueIds.length > 8 || dialogueCount > 8) {
    return "合并后不能超过 8 条对白或旁白";
  }
  if (new Set(dialogueIds).size !== dialogueIds.length) {
    return "两个来源包含重复对白，无法无损合并";
  }
  return null;
}

export function buildMergeTarget(
  first: API.ShotSpecVersionResponse,
  second: API.ShotSpecVersionResponse,
  narrativeReferences: API.NarrativeReferenceResponse[],
  options: MergeTargetOptions,
): API.TargetShotSpecRequest {
  const sourceError = mergeSourceError(first, second);
  if (sourceError) throw new Error(sourceError);
  const base = options.baseVersionId === second.id ? second : first;
  const other = base.id === first.id ? second : first;
  const spec = structuredClone(base.spec);
  const sources = [first, second];
  const beatKeyMaps = sources.map(() => new Map<string, string>());
  const actionEntries = sources.flatMap((version, sourceIndex) =>
    version.spec.action_beats.map((beat) => ({ beat, sourceIndex })),
  );
  spec.action_beats = actionEntries.map(({ beat, sourceIndex }, index) => {
    const beatKey = `beat-${index + 1}`;
    beatKeyMaps[sourceIndex].set(beat.beat_key, beatKey);
    return {
      beat_key: beatKey,
      order: index + 1,
      description: beat.description,
    };
  });

  spec.script_reference.dialogue_ids = sources.flatMap(
    (version) => version.spec.script_reference.dialogue_ids ?? [],
  );
  spec.dialogue_or_narration = sources.flatMap((version, sourceIndex) =>
    (version.spec.dialogue_or_narration ?? []).map((dialogue) => ({
      ...dialogue,
      beat_key: dialogue.beat_key
        ? (beatKeyMaps[sourceIndex].get(dialogue.beat_key) ?? null)
        : null,
    })),
  );
  spec.duration_ms =
    (first.spec.duration_ms ?? 3_000) +
    (second.spec.duration_ms ?? 3_000);
  const narrativeKeys = narrativeReferences.map(narrativeReferenceKey);
  if (new Set(narrativeKeys).size !== narrativeKeys.length) {
    throw new Error("两个来源包含冲突的叙事关系，需先分别修正映射");
  }
  return {
    title: options.title.trim(),
    spec,
    asset_references: mergedAssetReferences(base, other),
    narrative_references: narrativeReferences.map(toNarrativeReferenceInput),
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
  const initialBoundary = suggestedSplitBoundary(source.version);
  const [firstActionCount, setFirstActionCount] = useState(
    initialBoundary.firstActionCount,
  );
  const [firstDialogueCount, setFirstDialogueCount] = useState(
    initialBoundary.firstDialogueCount,
  );
  const [narrativeAllocation, setNarrativeAllocation] = useState<
    Record<string, "first" | "second">
  >({});
  const totalDuration = source.version.spec.duration_ms ?? 3_000;
  const secondDuration = totalDuration - firstDuration;
  const durationValid =
    Number.isInteger(firstDuration) &&
    firstDuration >= 500 &&
    secondDuration >= 500;
  const contentError = splitBoundaryError(
    source.version,
    firstActionCount,
    firstDialogueCount,
  );
  const narrativeAllocationComplete = source.narrativeReferences.every(
    (reference) => Boolean(narrativeAllocation[reference.id]),
  );

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
    if (
      !preflight ||
      !durationValid ||
      contentError ||
      !narrativeAllocationComplete
    ) {
      return;
    }
    const succeeded = await onApply(source.shot.id, {
      expected_source_spec_version_id: source.version.id,
      expected_order_hash: preflight.order_hash,
      impact_hash: preflight.impact_hash,
      idempotency_key: `studio-split:${source.shot.id}:${crypto.randomUUID()}`,
      targets: buildSplitTargets(source.version, source.narrativeReferences, {
        firstTitle,
        secondTitle,
        firstDurationMs: firstDuration,
        firstActionCount,
        firstDialogueCount,
        firstNarrativeReferenceIds: source.narrativeReferences
          .filter((reference) => narrativeAllocation[reference.id] === "first")
          .map((reference) => reference.id),
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
              <div className="grid gap-2">
                <Label htmlFor="splitFirstActionCount">前段动作数</Label>
                <Input
                  id="splitFirstActionCount"
                  max={source.version.spec.action_beats.length - 1}
                  min={1}
                  onChange={(event) => {
                    const nextValue = Number(event.target.value);
                    setFirstActionCount(nextValue);
                    setFirstDialogueCount(
                      suggestedDialogueBoundary(source.version, nextValue),
                    );
                  }}
                  type="number"
                  value={firstActionCount}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="splitFirstDialogueCount">前段对白数</Label>
                <Input
                  id="splitFirstDialogueCount"
                  max={
                    source.version.spec.script_reference.dialogue_ids?.length ??
                    0
                  }
                  min={0}
                  onChange={(event) =>
                    setFirstDialogueCount(Number(event.target.value))
                  }
                  type="number"
                  value={firstDialogueCount}
                />
              </div>
            </div>
            {source.narrativeReferences.length ? (
              <section className="grid gap-2" aria-labelledby="split-narrative-title">
                <div>
                  <h3 className="text-sm font-medium" id="split-narrative-title">
                    分配叙事来源
                  </h3>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    每条现有关系必须明确进入前段或后段，不能遗漏或重复。
                  </p>
                </div>
                {source.narrativeReferences.map((reference) => {
                  const unit = source.narrativeUnits.find(
                    (item) => item.unit_version_id === reference.unit_version_id,
                  );
                  const label = unit?.exact_text ?? reference.unit_version_id.slice(-8);
                  return (
                    <div
                      className="grid gap-2 rounded-xl border p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                      key={reference.id}
                    >
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{label}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {reference.channel} · {reference.role} · {reference.coverage_mode}
                        </p>
                      </div>
                      <div className="flex gap-2">
                        <Button
                          aria-label={`叙事来源 ${label} 分配到前段`}
                          aria-pressed={narrativeAllocation[reference.id] === "first"}
                          onClick={() =>
                            setNarrativeAllocation((current) => ({
                              ...current,
                              [reference.id]: "first",
                            }))
                          }
                          size="sm"
                          type="button"
                          variant={
                            narrativeAllocation[reference.id] === "first"
                              ? "secondary"
                              : "outline"
                          }
                        >
                          前段
                        </Button>
                        <Button
                          aria-label={`叙事来源 ${label} 分配到后段`}
                          aria-pressed={narrativeAllocation[reference.id] === "second"}
                          onClick={() =>
                            setNarrativeAllocation((current) => ({
                              ...current,
                              [reference.id]: "second",
                            }))
                          }
                          size="sm"
                          type="button"
                          variant={
                            narrativeAllocation[reference.id] === "second"
                              ? "secondary"
                              : "outline"
                          }
                        >
                          后段
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </section>
            ) : null}
            {contentError ? (
              <Alert variant="destructive">
                <AlertTitle>当前分界不能守恒</AlertTitle>
                <AlertDescription>{contentError}</AlertDescription>
              </Alert>
            ) : (
              <p className="text-xs leading-5 text-muted-foreground">
                前段包含 {firstActionCount} 个动作和 {firstDialogueCount} 条对白；其余内容全部进入后段。
              </p>
            )}
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
              disabled={
                busy ||
                !durationValid ||
                Boolean(contentError) ||
                !narrativeAllocationComplete ||
                !firstTitle.trim() ||
                !secondTitle.trim()
              }
              onClick={() => void apply()}
              type="button"
            >
              确认拆分
            </Button>
          ) : (
            <Button
              disabled={
                busy || totalDuration < 1_000 || Boolean(contentError)
              }
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
  const mergeError = preparation
    ? mergeSourceError(
        preparation.sources[0].version,
        preparation.sources[1].version,
      )
    : null;

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
    if (!preparation || mergeError) return;
    const [first, second] = preparation.sources;
    const succeeded = await onApply({
      shot_ids: preparation.preflight.source_shot_ids,
      expected_spec_version_ids:
        preparation.preflight.source_spec_version_ids,
      expected_order_hash: preparation.preflight.order_hash,
      impact_hash: preparation.preflight.impact_hash,
      idempotency_key: `studio-merge:${first.shot.id}:${second.shot.id}:${crypto.randomUUID()}`,
      target: buildMergeTarget(
        first.version,
        second.version,
        [...first.narrativeReferences, ...second.narrativeReferences],
        {
          baseVersionId,
          title,
        },
      ),
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
            {mergeError ? (
              <Alert variant="destructive">
                <AlertTitle>当前来源不能无损合并</AlertTitle>
                <AlertDescription>{mergeError}</AlertDescription>
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
                <Label>视觉与生成基础</Label>
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
                  仅从所选规格继承视觉、声音与生成意图；同场次动作、对白和资产引用会完整合并。
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
              disabled={busy || Boolean(mergeError) || !title.trim()}
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
