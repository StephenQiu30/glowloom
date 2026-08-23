"use client";

import { Save, ScanText, Target } from "lucide-react";
import { useMemo, useState } from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const kindLabels: Record<API.NarrativeUnitResponse["kind"], string> = {
  scene_heading: "场景标题",
  action: "动作",
  dialogue: "对白",
  narration: "旁白",
};

type UnitDraft = API.NarrativeUnitRevisionItem;

function unitDraft(unit: API.NarrativeUnitResponse): UnitDraft {
  return {
    unit_id: unit.unit_id,
    kind: unit.kind,
    source_start: unit.source_range.start,
    source_end: unit.source_range.end,
    required_for_coverage: unit.required_for_coverage,
  };
}

export function NarrativeStructurePanel({
  structure,
  scriptBody,
  busy,
  onRevise,
}: {
  structure: API.NarrativeStructureResponse;
  scriptBody: string;
  busy: boolean;
  onRevise: (request: API.NarrativeStructureRevisionRequest) => Promise<void>;
}) {
  const [drafts, setDrafts] = useState<UnitDraft[]>(() =>
    structure.units.map(unitDraft),
  );
  const [selectedUnitId, setSelectedUnitId] = useState(
    structure.units[0]?.unit_id ?? null,
  );
  const source = useMemo(() => Array.from(scriptBody), [scriptBody]);
  const selected = structure.units.find(
    (unit) => unit.unit_id === selectedUnitId,
  );
  const selectedDraft = drafts.find(
    (unit) => unit.unit_id === selectedUnitId,
  );
  const invalidPosition = drafts.findIndex((unit, index) => {
    const previous = drafts[index - 1];
    return (
      !Number.isInteger(unit.source_start) ||
      !Number.isInteger(unit.source_end) ||
      unit.source_start < 0 ||
      unit.source_end <= unit.source_start ||
      unit.source_end > source.length ||
      (previous !== undefined && unit.source_start < previous.source_end)
    );
  });
  const changed = drafts.some((draft, index) => {
    const original = structure.units[index];
    return (
      !original ||
      draft.kind !== original.kind ||
      draft.source_start !== original.source_range.start ||
      draft.source_end !== original.source_range.end ||
      draft.required_for_coverage !== original.required_for_coverage
    );
  });

  function updateDraft(unitId: string, update: Partial<UnitDraft>) {
    setDrafts((current) =>
      current.map((draft) =>
        draft.unit_id === unitId ? { ...draft, ...update } : draft,
      ),
    );
  }

  const highlight = selectedDraft
    ? {
        before: source.slice(0, selectedDraft.source_start).join(""),
        exact: source
          .slice(selectedDraft.source_start, selectedDraft.source_end)
          .join(""),
        after: source.slice(selectedDraft.source_end).join(""),
      }
    : null;

  return (
    <Card>
      <CardHeader className="gap-4 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">结构 revision {structure.revision}</Badge>
            <Badge variant="secondary">{structure.units.length} 个稳定单元</Badge>
          </div>
          <CardTitle className="mt-3">稳定叙事单元</CardTitle>
          <CardDescription className="mt-2">
            稳定 ID 与不可变版本连接剧本、分镜覆盖和导出；AI 只给出初始结构，人工修正后追加新 revision。
          </CardDescription>
        </div>
        <Button
          disabled={busy || invalidPosition >= 0 || !changed}
          onClick={() =>
            void onRevise({
              expected_revision: structure.revision,
              expected_current_script_version_id: structure.script_version_id,
              idempotency_key: `studio-narrative:${structure.id}:${crypto.randomUUID()}`,
              units: drafts,
            })
          }
        >
          <Save aria-hidden="true" />保存结构修正
        </Button>
      </CardHeader>
      <CardContent className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(340px,.8fr)]">
        <div className="grid max-h-[620px] content-start gap-3 overflow-auto pr-1">
          {drafts.map((draft, index) => {
            const unit = structure.units[index];
            if (!unit) return null;
            const selectedRow = draft.unit_id === selectedUnitId;
            return (
              <article
                className={`rounded-xl border p-3 ${
                  selectedRow
                    ? "border-foreground bg-muted/50"
                    : "border-border bg-background"
                }`}
                key={draft.unit_id}
              >
                <button
                  aria-pressed={selectedRow}
                  className="flex w-full items-start gap-3 text-left"
                  type="button"
                  onClick={() => setSelectedUnitId(draft.unit_id)}
                >
                  <span className="grid size-7 shrink-0 place-items-center rounded-full bg-foreground text-xs text-background">
                    {unit.position}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{kindLabels[draft.kind]}</span>
                      <span className="font-mono text-[11px] text-muted-foreground">
                        {draft.source_start}–{draft.source_end}
                      </span>
                    </span>
                    <span className="mt-1 block whitespace-pre-wrap text-sm leading-6 text-muted-foreground">
                      {unit.exact_text}
                    </span>
                  </span>
                </button>
                <div className="mt-3 grid gap-3 border-t pt-3 sm:grid-cols-[1fr_88px_88px]">
                  <div className="grid gap-1.5">
                    <Label htmlFor={`narrative-kind-${draft.unit_id}`}>类型</Label>
                    <Input
                      id={`narrative-kind-${draft.unit_id}`}
                      readOnly
                      value={kindLabels[draft.kind]}
                    />
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor={`narrative-start-${draft.unit_id}`}>起点</Label>
                    <Input
                      id={`narrative-start-${draft.unit_id}`}
                      min={0}
                      type="number"
                      value={draft.source_start}
                      onChange={(event) =>
                        updateDraft(draft.unit_id, {
                          source_start: event.currentTarget.valueAsNumber,
                        })
                      }
                    />
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor={`narrative-end-${draft.unit_id}`}>终点</Label>
                    <Input
                      id={`narrative-end-${draft.unit_id}`}
                      min={1}
                      type="number"
                      value={draft.source_end}
                      onChange={(event) =>
                        updateDraft(draft.unit_id, {
                          source_end: event.currentTarget.valueAsNumber,
                        })
                      }
                    />
                  </div>
                </div>
                <div className="mt-3 flex items-center gap-2">
                  <Checkbox
                    checked={draft.required_for_coverage}
                    id={`narrative-required-${draft.unit_id}`}
                    onCheckedChange={(checked) =>
                      updateDraft(draft.unit_id, {
                        required_for_coverage: checked === true,
                      })
                    }
                  />
                  <Label htmlFor={`narrative-required-${draft.unit_id}`}>
                    必须被分镜覆盖
                  </Label>
                </div>
              </article>
            );
          })}
        </div>

        <div className="grid content-start gap-4">
          <div className="rounded-xl border bg-slate-950 p-4 text-slate-200">
            <div className="mb-3 flex items-center justify-between gap-3">
              <p className="flex items-center gap-2 text-sm font-medium">
                <Target className="size-4" aria-hidden="true" />原文定位
              </p>
              {selected ? (
                <span className="font-mono text-[11px] text-slate-400">
                  {selected.unit_id.slice(0, 8)}
                </span>
              ) : null}
            </div>
            <pre className="max-h-[440px] overflow-auto whitespace-pre-wrap font-mono text-xs leading-6">
              {highlight ? (
                <>
                  <span className="text-slate-500">{highlight.before}</span>
                  <mark className="rounded bg-amber-300 px-0.5 text-slate-950">
                    {highlight.exact}
                  </mark>
                  <span className="text-slate-500">{highlight.after}</span>
                </>
              ) : (
                "选择一个叙事单元查看原文锚点。"
              )}
            </pre>
          </div>
          {invalidPosition >= 0 ? (
            <Alert variant="destructive">
              <ScanText aria-hidden="true" />
              <AlertTitle>字符范围无效</AlertTitle>
              <AlertDescription>
                第 {invalidPosition + 1} 个单元越界、为空、乱序或与前一单元重叠；修正后才能保存。
              </AlertDescription>
            </Alert>
          ) : (
            <Alert>
              <ScanText aria-hidden="true" />
              <AlertTitle>Unicode code point 锚点</AlertTitle>
              <AlertDescription>
                字符范围以 Unicode code point 计算；保存时服务端会重读原文、校验完整单元集合并生成新的依赖 hash。
              </AlertDescription>
            </Alert>
          )}
          <p className="break-all font-mono text-[11px] leading-5 text-muted-foreground">
            dependency {structure.dependency_hash}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}
