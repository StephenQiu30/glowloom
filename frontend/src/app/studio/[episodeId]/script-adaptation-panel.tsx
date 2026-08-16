"use client";

import {
  Ban,
  GitCompareArrows,
  LoaderCircle,
  RefreshCcw,
  Save,
  Sparkles,
  Upload,
} from "lucide-react";
import { type FormEvent, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

const statusLabels: Record<API.AdaptationRunResponse["status"], string> = {
  queued: "排队中",
  running: "改写中",
  succeeded: "候选待确认",
  published: "已发布",
  failed: "失败",
  cancelled: "已取消",
  unknown: "结果待人工确认",
};

export function ScriptAdaptationPanel({
  currentVersion,
  episode,
  run,
  difference,
  busy,
  onCreate,
  onSaveDraft,
  onCompare,
  onPublish,
  onCancel,
  onReset,
}: {
  currentVersion?: API.ScriptVersionResponse;
  episode: API.EpisodeResponse;
  run?: API.AdaptationRunResponse;
  difference: API.AdaptationDiffResponse | null;
  busy: boolean;
  onCreate: (request: API.AdaptationRunCreateRequest) => Promise<void>;
  onSaveDraft: (body: string) => Promise<void>;
  onCompare: () => Promise<void>;
  onPublish: () => Promise<void>;
  onCancel: () => Promise<void>;
  onReset: () => void;
}) {
  const [pacing, setPacing] = useState<API.AdaptationRunCreateRequest["pacing"]>(
    "fast",
  );
  const [colloquialDialogue, setColloquialDialogue] = useState(true);
  const [draftBody, setDraftBody] = useState(run?.draft_body ?? "");

  async function submitCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!currentVersion) return;
    const form = new FormData(event.currentTarget);
    const corePlotPoints = String(form.get("corePlotPoints") ?? "")
      .split("\n")
      .map((item) => item.trim())
      .filter(Boolean);
    await onCreate({
      input_script_version_id: currentVersion.id,
      target_duration_ms: Number(form.get("targetDurationMs")),
      core_plot_points: corePlotPoints,
      pacing,
      colloquial_dialogue: colloquialDialogue,
      idempotency_key: `studio-adaptation:${crypto.randomUUID()}`,
    });
  }

  const currentIsPublished =
    currentVersion?.status === "published" &&
    currentVersion.id === episode.current_script_version_id;
  const terminal = run &&
    ["published", "failed", "cancelled", "unknown"].includes(run.status);

  return (
    <Card>
      <CardHeader className="gap-4 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <Sparkles className="size-4 text-violet-500" aria-hidden="true" />
            <CardTitle>AI 剧本改写</CardTitle>
            {run ? <Badge variant="outline">{statusLabels[run.status]}</Badge> : null}
          </div>
          <CardDescription className="mt-2">
            固定当前 ScriptVersion 生成一个候选；AI 不会覆盖原稿，也不会自动发布。
          </CardDescription>
        </div>
        {terminal ? (
          <Button disabled={busy} variant="outline" onClick={onReset}>
            <RefreshCcw aria-hidden="true" />新建改写
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        {!run ? (
          <form className="grid gap-4" onSubmit={submitCreate}>
            {!currentIsPublished ? (
              <Alert className="border-amber-200 bg-amber-50 text-amber-800">
                <AlertTitle>先发布当前剧本</AlertTitle>
                <AlertDescription>
                  改写只能绑定当前已发布版本，草稿不会直接提交给 AI。
                </AlertDescription>
              </Alert>
            ) : null}
            <div className="grid gap-4 md:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="adaptationTargetDuration">目标时长（毫秒）</Label>
                <Input
                  defaultValue={Math.min(Math.max(episode.target_duration_ms, 15_000), 600_000)}
                  id="adaptationTargetDuration"
                  max={600_000}
                  min={15_000}
                  name="targetDurationMs"
                  required
                  type="number"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="adaptationPacing">节奏</Label>
                <Select value={pacing} onValueChange={(value) => setPacing(value as typeof pacing)}>
                  <SelectTrigger className="w-full" id="adaptationPacing">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="fast">快节奏</SelectItem>
                    <SelectItem value="balanced">均衡</SelectItem>
                    <SelectItem value="slow">舒缓</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="adaptationCorePlotPoints">必须保留的核心情节</Label>
              <Textarea
                id="adaptationCorePlotPoints"
                name="corePlotPoints"
                placeholder={"每行一项，例如：\n孩子必须获救\n结尾揭示主角真实身份"}
                required
                rows={4}
              />
            </div>
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={colloquialDialogue}
                onCheckedChange={(checked) => setColloquialDialogue(checked === true)}
              />
              对白口语化
            </label>
            <div className="flex justify-end">
              <Button disabled={busy || !currentIsPublished} type="submit">
                {busy ? (
                  <LoaderCircle className="animate-spin" aria-hidden="true" />
                ) : (
                  <Sparkles aria-hidden="true" />
                )}
                生成改写候选
              </Button>
            </div>
          </form>
        ) : run.status === "queued" || run.status === "running" ? (
          <div className="grid gap-4">
            <Alert>
              <LoaderCircle className="animate-spin" aria-hidden="true" />
              <AlertTitle>{statusLabels[run.status]}</AlertTitle>
              <AlertDescription>
                任务状态由服务端恢复；Worker 中断后会进入 unknown，不会盲目重发。
              </AlertDescription>
            </Alert>
            {run.status === "queued" ? (
              <div className="flex justify-end">
                <Button disabled={busy} variant="outline" onClick={onCancel}>
                  <Ban aria-hidden="true" />取消排队
                </Button>
              </div>
            ) : null}
          </div>
        ) : run.status === "succeeded" || run.status === "published" ? (
          <div className="grid gap-4">
            <div className="flex flex-wrap gap-2 text-sm text-muted-foreground">
              <Badge variant="secondary">
                估算 {Math.round((run.estimated_duration_ms ?? 0) / 1000)} 秒
              </Badge>
              <span>{run.change_summary}</span>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="adaptationDraft">改写工作稿</Label>
              <Textarea
                disabled={run.status === "published"}
                id="adaptationDraft"
                maxLength={20_000}
                rows={16}
                value={draftBody}
                onChange={(event) => setDraftBody(event.target.value)}
              />
            </div>
            {difference ? (
              <div className="grid gap-2">
                <p className="text-sm font-medium">
                  新增 {difference.added_lines} 行 · 删除 {difference.removed_lines} 行
                </p>
                <div className="max-h-72 overflow-auto rounded-lg bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-200">
                  {difference.diff_lines.map((line, index) => (
                    <p
                      className={
                        line.startsWith("+") && !line.startsWith("+++")
                          ? "text-emerald-300"
                          : line.startsWith("-") && !line.startsWith("---")
                            ? "text-rose-300"
                            : "text-slate-400"
                      }
                      key={`${index}:${line}`}
                    >
                      {line || " "}
                    </p>
                  ))}
                </div>
              </div>
            ) : null}
            {run.status === "succeeded" ? (
              <div className="flex flex-wrap justify-end gap-2">
                <Button disabled={busy || !draftBody.trim()} variant="outline" onClick={() => onSaveDraft(draftBody)}>
                  <Save aria-hidden="true" />保存工作稿
                </Button>
                <Button disabled={busy} variant="outline" onClick={onCompare}>
                  <GitCompareArrows aria-hidden="true" />查看差异
                </Button>
                <Button disabled={busy || !draftBody.trim()} onClick={onPublish}>
                  <Upload aria-hidden="true" />发布并设为当前
                </Button>
              </div>
            ) : (
              <Alert>
                <AlertTitle>已发布为新版本</AlertTitle>
                <AlertDescription>
                  版本 {run.published_script_version_id} 已成为当前剧本，候选历史仍可读。
                </AlertDescription>
              </Alert>
            )}
          </div>
        ) : (
          <Alert variant="destructive">
            <AlertTitle>{statusLabels[run.status]}</AlertTitle>
            <AlertDescription>
              错误代码：{run.error_code ?? "未提供"}。原稿与当前版本未改变，请新建改写任务。
            </AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  );
}
