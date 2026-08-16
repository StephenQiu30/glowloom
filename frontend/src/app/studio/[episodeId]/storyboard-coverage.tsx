"use client";

import { Film, Link2, ShieldAlert, TextQuote } from "lucide-react";
import { useState } from "react";

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
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/class-names";

import { toNarrativeReferenceInput } from "./narrative-reference";

type StoryboardCoverageProps = {
  busy: boolean;
  report: API.CoverageReportResponse;
  selectedShotId: string | null;
  shots: API.ShotResponse[];
  onDecide: (request: API.CoverageDecisionRequest) => Promise<boolean>;
  onReplace: (
    shot: API.ShotResponse,
    references: API.NarrativeReferenceInput[],
  ) => Promise<boolean>;
  onSelectShot: (shotId: string) => void;
};

const unitKindLabels: Record<API.UnitCoverageResponse["kind"], string> = {
  scene_heading: "场次",
  action: "动作",
  dialogue: "台词",
  narration: "旁白",
};

const unitStatusLabels: Record<API.UnitCoverageResponse["status"], string> = {
  covered: "已覆盖",
  approved_omitted: "已批准省略",
  uncovered: "未覆盖",
};

const shotStatusLabels: Record<API.ShotCoverageResponse["status"], string> = {
  linked: "已有来源",
  approved_invented: "已批准原创",
  orphan: "无来源",
};

function statusClass(status: string): string {
  if (status === "covered" || status === "linked") {
    return "border-emerald-200 bg-emerald-50 text-emerald-700";
  }
  if (status === "approved_omitted" || status === "approved_invented") {
    return "border-sky-200 bg-sky-50 text-sky-700";
  }
  return "border-amber-200 bg-amber-50 text-amber-700";
}

function DecisionDialog({
  action,
  busy,
  report,
  shotSpecVersionId,
  triggerLabel,
  unitVersionId,
  onDecide,
}: {
  action: API.CoverageDecisionRequest["action"];
  busy: boolean;
  report: API.CoverageReportResponse;
  shotSpecVersionId: string | null;
  triggerLabel: string;
  unitVersionId: string | null;
  onDecide: StoryboardCoverageProps["onDecide"];
}) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const approving = action.startsWith("approve");
  const targetName = action.includes("omission") ? "省略" : "原创";

  function changeOpen(nextOpen: boolean) {
    setOpen(nextOpen);
    if (!nextOpen) setReason("");
  }

  async function submit() {
    if (!reason.trim()) return;
    const succeeded = await onDecide({
      action,
      unit_version_id: unitVersionId,
      shot_spec_version_id: shotSpecVersionId,
      reason: reason.trim(),
      evidence: null,
      expected_evaluation_hash: report.evaluation_hash,
      idempotency_key: `studio-coverage:${action}:${unitVersionId ?? shotSpecVersionId}:${crypto.randomUUID()}`,
    });
    if (succeeded) changeOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button
          aria-label={triggerLabel}
          disabled={busy}
          size="sm"
          type="button"
          variant="outline"
        >
          {approving ? `批准${targetName}` : `撤销${targetName}`}
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{approving ? "记录覆盖决议" : "撤销覆盖决议"}</DialogTitle>
          <DialogDescription>
            决议固定当前剧本单元或镜头规格版本；底层版本变化后会显式过期。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor={`coverageReason-${unitVersionId ?? shotSpecVersionId}`}>
            覆盖决议原因
          </Label>
          <Textarea
            id={`coverageReason-${unitVersionId ?? shotSpecVersionId}`}
            maxLength={1_000}
            onChange={(event) => setReason(event.target.value)}
            placeholder="说明为何不会破坏叙事因果或为何必须保留创作性镜头"
            value={reason}
          />
        </div>
        <DialogFooter>
          <Button
            disabled={busy || !reason.trim()}
            onClick={() => void submit()}
            type="button"
          >
            确认{approving ? "批准" : "撤销"}{targetName}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MappingDialog({
  busy,
  report,
  shot,
  onReplace,
}: {
  busy: boolean;
  report: API.CoverageReportResponse;
  shot?: API.ShotResponse;
  onReplace: StoryboardCoverageProps["onReplace"];
}) {
  const [open, setOpen] = useState(false);
  const currentReferences = report.references
    .filter(
      (reference) =>
        reference.shot_id === shot?.id &&
        reference.shot_spec_version_id === shot?.current_spec_version_id,
    )
    .reduce<Record<string, API.NarrativeReferenceInput[]>>(
      (grouped, reference) => {
        const unitReferences = grouped[reference.unit_version_id] ?? [];
        grouped[reference.unit_version_id] = [
          ...unitReferences,
          toNarrativeReferenceInput(reference),
        ];
        return grouped;
      },
      {},
    );
  const [draft, setDraft] = useState<
    Record<string, API.NarrativeReferenceInput[]>
  >(currentReferences);

  function changeOpen(nextOpen: boolean) {
    if (nextOpen) setDraft(currentReferences);
    setOpen(nextOpen);
  }

  function toggle(unit: API.UnitCoverageResponse, checked: boolean) {
    setDraft((current) => {
      const next = { ...current };
      if (!checked) {
        delete next[unit.unit_version_id];
        return next;
      }
      const primaryTaken = report.references.some(
        (reference) =>
          reference.shot_id !== shot?.id &&
          reference.unit_version_id === unit.unit_version_id &&
          reference.channel === unit.required_channel &&
          reference.role === "primary" &&
          !report.stale_reference_ids.includes(reference.id),
      );
      next[unit.unit_version_id] = [
        {
          unit_version_id: unit.unit_version_id,
          channel: unit.required_channel,
          role: primaryTaken ? "supporting" : "primary",
          coverage_mode: "full",
          segment_start: null,
          segment_end: null,
          contribution: primaryTaken ? "supporting" : "required",
        },
      ];
      return next;
    });
  }

  function update(
    unitVersionId: string,
    referenceIndex: number,
    patch: Partial<API.NarrativeReferenceInput>,
  ) {
    setDraft((current) => {
      const unitReferences = [...(current[unitVersionId] ?? [])];
      const reference = unitReferences[referenceIndex];
      if (!reference) return current;
      unitReferences[referenceIndex] = { ...reference, ...patch };
      return { ...current, [unitVersionId]: unitReferences };
    });
  }

  async function submit() {
    if (!shot) return;
    const references = report.units.flatMap(
      (unit) => draft[unit.unit_version_id] ?? [],
    );
    if (await onReplace(shot, references)) changeOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button
          disabled={busy || !shot?.current_spec_version_id}
          type="button"
          variant="outline"
        >
          <Link2 aria-hidden="true" />编辑来源映射
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>编辑镜头叙事来源</DialogTitle>
          <DialogDescription>
            保存会克隆新的不可变镜头规格版本；未勾选的旧关系不会进入新版本。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          {report.units.map((unit) => {
            const selected = draft[unit.unit_version_id] ?? [];
            const label = `${unitKindLabels[unit.kind]}：${unit.exact_text}`;
            return (
              <div className="grid gap-3 rounded-xl border p-3" key={unit.unit_version_id}>
                <label className="flex items-start gap-3 text-sm">
                  <Checkbox
                    aria-label={`映射叙事单元 ${label}`}
                    checked={selected.length > 0}
                    onCheckedChange={(checked) => toggle(unit, checked === true)}
                  />
                  <span className="min-w-0">
                    <span className="font-medium">{label}</span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      需要 {unit.required_channel} · {unitStatusLabels[unit.status]}
                    </span>
                  </span>
                </label>
                {selected.map((reference, referenceIndex) => (
                  <div
                    className="grid gap-2 pl-7"
                    key={`${unit.unit_version_id}:${reference.channel}:${reference.segment_start ?? "full"}:${referenceIndex}`}
                  >
                    <span className="text-xs text-muted-foreground">
                      关系 {referenceIndex + 1}
                      {reference.coverage_mode === "partial"
                        ? ` · 字符 ${reference.segment_start}-${reference.segment_end}`
                        : " · 完整覆盖"}
                    </span>
                    <div className="grid gap-3 sm:grid-cols-3">
                      <Select
                        value={reference.channel}
                        onValueChange={(value) =>
                          update(unit.unit_version_id, referenceIndex, {
                            channel: value as API.NarrativeReferenceInput["channel"],
                          })
                        }
                      >
                        <SelectTrigger aria-label={`${label} 关系 ${referenceIndex + 1} 覆盖通道`}><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="visual">visual</SelectItem>
                          <SelectItem value="audio">audio</SelectItem>
                          <SelectItem value="both">both</SelectItem>
                        </SelectContent>
                      </Select>
                      <Select
                        value={reference.role}
                        onValueChange={(value) =>
                          update(unit.unit_version_id, referenceIndex, {
                            role: value as API.NarrativeReferenceInput["role"],
                          })
                        }
                      >
                        <SelectTrigger aria-label={`${label} 关系 ${referenceIndex + 1} 角色`}><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {[
                            "primary",
                            "dialogue",
                            "reaction",
                            "insert",
                            "setup",
                            "payoff",
                            "transition",
                            "supporting",
                          ].map((role) => <SelectItem key={role} value={role}>{role}</SelectItem>)}
                        </SelectContent>
                      </Select>
                      <Select
                        value={reference.contribution}
                        onValueChange={(value) =>
                          update(unit.unit_version_id, referenceIndex, {
                            contribution: value as API.NarrativeReferenceInput["contribution"],
                          })
                        }
                      >
                        <SelectTrigger aria-label={`${label} 关系 ${referenceIndex + 1} 覆盖贡献`}><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="required">required</SelectItem>
                          <SelectItem value="supporting">supporting</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                ))}
              </div>
            );
          })}
        </div>
        <DialogFooter>
          <Button disabled={busy || !shot} onClick={() => void submit()} type="button">
            保存来源映射
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function StoryboardCoverage({
  busy,
  report,
  selectedShotId,
  shots,
  onDecide,
  onReplace,
  onSelectShot,
}: StoryboardCoverageProps) {
  const [selectedUnitId, setSelectedUnitId] = useState<string | null>(null);
  const selectedShot = shots.find((shot) => shot.id === selectedShotId);
  const selectedCoverageShot = report.shots.find(
    (shot) => shot.shot_id === selectedShotId,
  );
  const selectedUnit = report.units.find(
    (unit) => unit.unit_version_id === selectedUnitId,
  );
  const activeShotIds = new Set(
    selectedUnit?.shot_ids ?? (selectedShotId ? [selectedShotId] : []),
  );
  const activeUnitIds = new Set(
    selectedUnitId
      ? [selectedUnitId]
      : (selectedCoverageShot?.unit_version_ids ?? []),
  );

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Link2 className="size-5" aria-hidden="true" />剧本覆盖与镜头来源
            </CardTitle>
            <CardDescription className="mt-1">
              文本与镜头通过固定版本的多对多关系关联；点击任一侧可定位另一侧。
            </CardDescription>
          </div>
          <MappingDialog
            busy={busy}
            report={report}
            shot={selectedShot}
            onReplace={onReplace}
          />
        </div>
      </CardHeader>
      <CardContent className="grid gap-5">
        <div className="flex flex-wrap gap-2" aria-label="覆盖摘要">
          <Badge variant="outline">{report.summary.covered} 个已覆盖</Badge>
          <Badge className={statusClass("uncovered")} variant="outline">
            {report.summary.uncovered} 个未覆盖
          </Badge>
          <Badge className={statusClass("orphan")} variant="outline">
            {report.summary.orphan} 个无来源镜头
          </Badge>
          <Badge variant="outline">{report.summary.stale} 个过期关系</Badge>
        </div>
        {report.status === "unavailable" ? (
          <Alert variant="destructive">
            <ShieldAlert aria-hidden="true" />
            <AlertTitle>覆盖依赖不可用</AlertTitle>
            <AlertDescription>剧本结构或镜头事实读取失败，当前不能进入生产。</AlertDescription>
          </Alert>
        ) : report.summary.stale ? (
          <Alert className="border-amber-200 bg-amber-50 text-amber-800">
            <ShieldAlert aria-hidden="true" />
            <AlertTitle>存在过期覆盖证据</AlertTitle>
            <AlertDescription>重新映射或重新审批后才能恢复 readiness。</AlertDescription>
          </Alert>
        ) : null}
        <div className="grid gap-5 xl:grid-cols-2">
          <section aria-labelledby="coverage-units-title" className="grid gap-2">
            <div className="flex items-center gap-2">
              <TextQuote className="size-4" aria-hidden="true" />
              <h3 className="font-medium" id="coverage-units-title">剧本叙事单元</h3>
            </div>
            {report.units.map((unit) => (
              <div
                className={cn(
                  "flex items-start gap-2 rounded-xl border p-2",
                  activeUnitIds.has(unit.unit_version_id) && "ring-2 ring-primary/25",
                )}
                key={unit.unit_version_id}
              >
                <Button
                  className="h-auto min-w-0 flex-1 justify-start whitespace-normal px-2 py-2 text-left"
                  onClick={() => {
                    setSelectedUnitId(unit.unit_version_id);
                    const shotId = unit.shot_ids[0];
                    if (shotId) onSelectShot(shotId);
                  }}
                  type="button"
                  variant="ghost"
                >
                  <span className="min-w-0">
                    <span className="block text-xs text-muted-foreground">
                      {unitKindLabels[unit.kind]} · {unit.required_channel}
                    </span>
                    <span className="mt-1 block text-sm">{unit.exact_text}</span>
                  </span>
                </Button>
                <div className="grid shrink-0 justify-items-end gap-2">
                  <Badge className={statusClass(unit.status)} variant="outline">
                    {unitStatusLabels[unit.status]}
                  </Badge>
                  {unit.status !== "covered" ? (
                    <DecisionDialog
                      action={unit.status === "approved_omitted" ? "revoke_omission" : "approve_omission"}
                      busy={busy}
                      report={report}
                      shotSpecVersionId={null}
                      triggerLabel={`${unit.status === "approved_omitted" ? "撤销省略" : "批准省略"} ${unit.exact_text}`}
                      unitVersionId={unit.unit_version_id}
                      onDecide={onDecide}
                    />
                  ) : null}
                </div>
              </div>
            ))}
          </section>
          <section aria-labelledby="coverage-shots-title" className="grid gap-2">
            <div className="flex items-center gap-2">
              <Film className="size-4" aria-hidden="true" />
              <h3 className="font-medium" id="coverage-shots-title">镜头来源</h3>
            </div>
            {report.shots.map((shot) => (
              <div
                className={cn(
                  "flex items-start gap-2 rounded-xl border p-2",
                  activeShotIds.has(shot.shot_id) && "ring-2 ring-primary/25",
                )}
                key={shot.shot_id}
              >
                <Button
                  className="h-auto min-w-0 flex-1 justify-start whitespace-normal px-2 py-2 text-left"
                  onClick={() => {
                    setSelectedUnitId(null);
                    onSelectShot(shot.shot_id);
                  }}
                  type="button"
                  variant="ghost"
                >
                  <span>
                    <span className="block text-xs text-muted-foreground">
                      镜头 {String(shot.position).padStart(2, "0")}
                    </span>
                    <span className="mt-1 block text-sm">{shot.title}</span>
                  </span>
                </Button>
                <div className="grid shrink-0 justify-items-end gap-2">
                  <Badge className={statusClass(shot.status)} variant="outline">
                    {shotStatusLabels[shot.status]}
                  </Badge>
                  {shot.status !== "linked" && shot.spec_version_id ? (
                    <DecisionDialog
                      action={shot.status === "approved_invented" ? "revoke_invented" : "approve_invented"}
                      busy={busy}
                      report={report}
                      shotSpecVersionId={shot.spec_version_id}
                      triggerLabel={`${shot.status === "approved_invented" ? "撤销原创" : "批准原创"} ${shot.title}`}
                      unitVersionId={null}
                      onDecide={onDecide}
                    />
                  ) : null}
                </div>
              </div>
            ))}
          </section>
        </div>
      </CardContent>
    </Card>
  );
}
