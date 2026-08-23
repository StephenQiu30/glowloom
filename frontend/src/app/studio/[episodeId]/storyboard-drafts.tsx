"use client";

import { Bot, Check, LoaderCircle, PencilLine, ShieldCheck, X } from "lucide-react";
import { type FormEvent, useEffect, useMemo, useRef, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

type DraftAction = "accepted" | "modified" | "ignored";

type StoryboardDraftsProps = {
  assetBible?: API.AssetBibleResponse;
  batch?: API.DraftBatchResponse;
  busy: boolean;
  canCreate: boolean;
  episodeId: string;
  onApply: (preflight: API.DraftApplyPreflightResponse) => Promise<void>;
  onApprove: () => Promise<void>;
  onCreate: (assetStateIds: string[]) => Promise<void>;
  onDecide: (
    draft: API.DraftShotResponse,
    action: DraftAction,
    target?: API.DraftTarget,
  ) => Promise<void>;
  onPreflight: () => Promise<API.DraftApplyPreflightResponse | undefined>;
};

const statusLabels: Record<API.DraftBatchResponse["status"], string> = {
  queued: "等待生成",
  running: "正在生成",
  needs_review: "等待逐镜审核",
  approved: "已批准，等待应用",
  applied: "已写入正式分镜",
  failed: "生成失败",
  unknown: "结果未知，需新建批次",
  cancelled: "已取消",
};

function latestDecision(draft: API.DraftShotResponse) {
  return draft.decision_history.at(-1);
}

function editableTarget(
  draft: API.DraftShotResponse,
  title: string,
  purpose: string,
): API.DraftTarget {
  return {
    title: title.trim(),
    narrative_unit_version_ids: draft.narrative_unit_version_ids,
    spec: {
      ...draft.spec,
      narrative: { ...draft.spec.narrative, purpose: purpose.trim() },
    },
    asset_references: draft.asset_references.map((reference) => ({
      slot_key: reference.slot_key,
      role: reference.role,
      asset_version_id: reference.asset_version_id,
      subject_key: reference.subject_key,
    })),
  };
}

function EditDraftDialog({
  busy,
  draft,
  onSave,
}: {
  busy: boolean;
  draft: API.DraftShotResponse;
  onSave: (target: API.DraftTarget) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState(draft.title);
  const [purpose, setPurpose] = useState(draft.spec.narrative.purpose);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!title.trim() || !purpose.trim()) return;
    await onSave(editableTarget(draft, title, purpose));
    setOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button disabled={busy} size="sm" type="button" variant="outline">
          <PencilLine aria-hidden="true" />编辑后采用
        </Button>
      </DialogTrigger>
      <DialogContent>
        <form className="grid gap-4" onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>编辑草案后采用</DialogTitle>
            <DialogDescription>
              当前只修改镜头标题与叙事目的；原始 AI 草案和完整决议历史仍保留。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor={`draft-title-${draft.id}`}>镜头标题</Label>
            <Input
              id={`draft-title-${draft.id}`}
              maxLength={200}
              onChange={(event) => setTitle(event.target.value)}
              value={title}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`draft-purpose-${draft.id}`}>叙事目的</Label>
            <Textarea
              id={`draft-purpose-${draft.id}`}
              maxLength={500}
              onChange={(event) => setPurpose(event.target.value)}
              value={purpose}
            />
          </div>
          <DialogFooter>
            <Button disabled={busy || !title.trim() || !purpose.trim()} type="submit">
              保存人工版本
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function StoryboardDrafts({
  assetBible,
  batch,
  busy,
  canCreate,
  episodeId,
  onApply,
  onApprove,
  onCreate,
  onDecide,
  onPreflight,
}: StoryboardDraftsProps) {
  const availableStates = useMemo(
    () =>
      (assetBible?.items ?? []).flatMap((item) =>
        item.states
          .filter(
            (entry) =>
              entry.readiness.status === "ready" &&
              entry.current_version &&
              entry.occurrences.some(
                (occurrence) =>
                  occurrence.episode_id === episodeId &&
                  occurrence.decision === "link" &&
                  occurrence.freshness === "current",
              ),
          )
          .map((entry) => ({
            id: entry.state.id,
            label: `${item.asset.name} · ${entry.state.label}`,
          })),
      ),
    [assetBible, episodeId],
  );
  const [selectedStates, setSelectedStates] = useState<Set<string>>(
    () => new Set(availableStates.map((state) => state.id)),
  );
  const statesInitialized = useRef(assetBible !== undefined);
  const [preflight, setPreflight] = useState<API.DraftApplyPreflightResponse>();

  useEffect(() => {
    if (statesInitialized.current || assetBible === undefined) return;
    setSelectedStates(new Set(availableStates.map((state) => state.id)));
    statesInitialized.current = true;
  }, [assetBible, availableStates]);

  async function prepareApply() {
    const result = await onPreflight();
    setPreflight(result);
  }

  return (
    <Card className="border-primary/20 bg-primary/[0.025]">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Bot className="size-5 text-primary" aria-hidden="true" />AI 分镜草案
            </CardTitle>
            <CardDescription className="mt-1">
              模型只写待审草案；逐镜决议和整批 Apply 完成前，正式镜头尚未写入。
            </CardDescription>
          </div>
          {batch ? <Badge variant="outline">{statusLabels[batch.status]}</Badge> : null}
        </div>
      </CardHeader>
      <CardContent className="grid gap-4">
        {!batch ? (
          <div className="grid gap-4">
            {availableStates.length ? (
              <div className="grid gap-2 sm:grid-cols-2">
                {availableStates.map((state) => (
                  <Label className="flex items-center gap-2 rounded-lg border p-3" key={state.id}>
                    <Checkbox
                      checked={selectedStates.has(state.id)}
                      onCheckedChange={(checked) => {
                        setSelectedStates((current) => {
                          const next = new Set(current);
                          if (checked) next.add(state.id);
                          else next.delete(state.id);
                          return next;
                        });
                      }}
                    />
                    <span>{state.label}</span>
                  </Label>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                当前没有与本集关联且已就绪的资产状态，可先生成无资产绑定草案。
              </p>
            )}
            <Button
              className="w-fit"
              disabled={busy || !canCreate}
              onClick={() => void onCreate([...selectedStates])}
              type="button"
            >
              生成待审核草案
            </Button>
          </div>
        ) : batch.status === "queued" || batch.status === "running" ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
            正在基于固定剧本、叙事单元和资产版本生成草案…
          </div>
        ) : ["failed", "unknown", "cancelled"].includes(batch.status) ? (
          <div className="grid gap-3">
            <Alert variant="destructive">
              <AlertTitle>{statusLabels[batch.status]}</AlertTitle>
              <AlertDescription>
                错误代码：{batch.error_code ?? "unknown"}。旧批次保持只读，请创建新批次重试。
              </AlertDescription>
            </Alert>
            <Button
              className="w-fit"
              disabled={busy || !canCreate}
              onClick={() => void onCreate([...selectedStates])}
              type="button"
              variant="outline"
            >
              创建新草案批次
            </Button>
          </div>
        ) : (
          <>
            <div className="grid gap-3">
              {batch.drafts.map((draft) => {
                const decision = latestDecision(draft);
                return (
                  <div className="grid gap-3 rounded-xl border bg-background p-4" key={draft.id}>
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="font-medium">
                          {String(draft.position).padStart(2, "0")} · {draft.title}
                        </p>
                        <p className="mt-1 text-sm text-muted-foreground">
                          {(draft.spec.duration_ms ?? 0) / 1000} 秒 · 覆盖 {draft.narrative_unit_version_ids.length} 个叙事单元
                        </p>
                      </div>
                      {decision ? <Badge variant="secondary">{decision.action}</Badge> : null}
                    </div>
                    <p className="text-sm">{draft.spec.narrative.purpose}</p>
                    {draft.risk_codes.length ? (
                      <p className="text-xs text-amber-700">
                        待复核：{draft.risk_codes.join("、")}
                      </p>
                    ) : null}
                    {batch.status === "needs_review" ? (
                      <div className="flex flex-wrap gap-2">
                        <Button
                          aria-label="接受此镜"
                          disabled={busy}
                          onClick={() => void onDecide(draft, "accepted", undefined)}
                          size="sm"
                          type="button"
                        >
                          <Check aria-hidden="true" />接受
                        </Button>
                        <EditDraftDialog
                          busy={busy}
                          draft={draft}
                          onSave={(target) => onDecide(draft, "modified", target)}
                        />
                        <Button
                          aria-label="忽略此镜"
                          disabled={busy}
                          onClick={() => void onDecide(draft, "ignored", undefined)}
                          size="sm"
                          type="button"
                          variant="ghost"
                        >
                          <X aria-hidden="true" />忽略
                        </Button>
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
            {batch.status === "needs_review" ? (
              <Button
                className="w-fit"
                disabled={busy || batch.decision_summary.pending > 0}
                onClick={() => void onApprove()}
                type="button"
              >
                <ShieldCheck aria-hidden="true" />批准整批草案
              </Button>
            ) : null}
            {batch.status === "approved" ? (
              <div className="grid gap-3 rounded-xl border border-emerald-200 bg-emerald-50 p-4">
                {preflight ? (
                  <p className="text-sm text-emerald-900">
                    将保留 {preflight.diff.kept} 个现有镜头，新建 {preflight.diff.created} 个镜头；修改 0、归档 0。
                  </p>
                ) : null}
                <div className="flex flex-wrap gap-2">
                  <Button disabled={busy} onClick={() => void prepareApply()} type="button" variant="outline">
                    预检写入影响
                  </Button>
                  <Button
                    disabled={busy || !preflight}
                    onClick={() => preflight && void onApply(preflight)}
                    type="button"
                  >
                    原子写入正式分镜
                  </Button>
                </div>
              </div>
            ) : null}
            {batch.status === "applied" ? (
              <Button
                className="w-fit"
                disabled={busy || !canCreate}
                onClick={() => void onCreate([...selectedStates])}
                type="button"
                variant="outline"
              >
                基于当前事实生成新批次
              </Button>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  );
}
