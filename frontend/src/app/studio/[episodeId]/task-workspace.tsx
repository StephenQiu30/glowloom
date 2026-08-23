import {
  AlertCircle,
  CheckCircle2,
  CircleX,
  Clock3,
  Coins,
  ImageIcon,
  Pause,
  Play,
  RefreshCw,
  Settings2,
  Video,
} from "lucide-react";
import { type FormEvent, useState } from "react";

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

import { taskStatusLabels, taskTone } from "./episode-studio-model";

const taskTypeLabels: Record<API.TaskResponse["task_type"], string> = {
  episode_planning: "分集规划",
  script_adaptation: "剧本改写",
  script_extraction: "剧本结构提取",
  storyboard_draft: "AI 分镜草案",
  storyboard_export: "可信分镜包",
  image_generation: "镜头图片生成",
  video_generation: "镜头视频生成",
  media_probe: "媒体探测",
  upload_expiration: "上传临时文件清理",
  upload_cleanup: "过期上传补偿清理",
  media_location_migration: "媒体位置迁移 / 回滚",
  media_location_retirement: "媒体旧位置退役",
};

const requestTypeLabels: Record<API.TaskResponse["request_type"], string> = {
  episode_plan: "分集计划",
  adaptation_run: "剧本改写运行",
  extraction_batch: "提取批次",
  storyboard_draft_batch: "分镜草案批次",
  storyboard_export_job: "分镜包任务",
  generation_request: "镜头生成请求",
  media_version: "媒体版本",
  upload_session: "上传会话",
  workspace: "工作空间",
  media_location: "媒体存储位置",
};

const scheduleHandlerLabels: Record<API.ScheduleResponse["handler_name"], string> = {
  expire_upload_session: "单次上传到期清理",
  cleanup_expired_uploads: "周期过期上传补偿",
  retire_media_location: "媒体旧位置到期退役",
  unregistered: "未登记调度处理器",
};

const scheduleStatusLabels: Record<API.ScheduleResponse["status"], string> = {
  active: "运行中",
  paused: "已暂停",
  completed: "已完成",
  manual_attention: "需人工处理",
};

const dateTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "medium",
  timeStyle: "medium",
});

const capabilityStatusLabels: Record<
  API.ModelCapabilityResponse["status"],
  string
> = {
  active: "可用",
  inactive: "已停用",
  unavailable: "暂不可用",
};

const capabilityReasonLabels: Record<string, string> = {
  provider_contract_unverified: "真实账号参数、计费和权限契约尚未验收",
};

const moneyFormatter = new Intl.NumberFormat("zh-CN", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 6,
});

function formatMoney(amount: string, currency: string): string {
  return `${currency} ${moneyFormatter.format(Number(amount))}`;
}

function scheduleTone(status: API.ScheduleResponse["status"]): string {
  if (status === "completed") return "border-emerald-200 bg-emerald-50 text-emerald-700";
  if (status === "manual_attention") return "border-rose-200 bg-rose-50 text-rose-700";
  if (status === "paused") return "border-amber-200 bg-amber-50 text-amber-700";
  return "border-border bg-muted text-foreground";
}

function formatDateTime(value: string | null): string {
  return value ? dateTimeFormatter.format(new Date(value)) : "无后续触发";
}

const misfirePolicyLabels: Record<API.ScheduleResponse["misfire_policy"], string> = {
  skip: "跳过过期时点",
  run_once: "合并补执一次",
  catch_up: "有界逐次补执",
};

function scheduleRuleLabel(schedule: API.ScheduleResponse): string {
  if (schedule.kind === "one_off" && "at" in schedule.rule) {
    return `单次 · ${formatDateTime(schedule.rule.at)}`;
  }
  if (schedule.kind === "interval" && "seconds" in schedule.rule) {
    return `每 ${schedule.rule.seconds} 秒`;
  }
  if (schedule.kind === "cron" && "expression" in schedule.rule) {
    return `Cron ${schedule.rule.expression}`;
  }
  return "规则需要人工检查";
}

type ScheduleConfiguration = Omit<
  API.ScheduleConfigurationRequest,
  "expected_revision" | "effective_from"
>;

export function TaskWorkspace({
  busy,
  capabilities,
  costs,
  productionFactsLoading,
  productionFactsUnavailable,
  schedules,
  tasks,
  onConfigureSchedule,
  onCancelGenerationTask,
  onPauseSchedule,
  onResumeSchedule,
  onTriggerSchedule,
}: {
  busy: boolean;
  capabilities: API.ModelCapabilityResponse[];
  costs: API.CostQueryResponse | null;
  productionFactsLoading: boolean;
  productionFactsUnavailable: boolean;
  schedules: API.ScheduleResponse[];
  tasks: API.TaskResponse[];
  onCancelGenerationTask: (task: API.TaskResponse) => Promise<boolean>;
  onConfigureSchedule: (
    schedule: API.ScheduleResponse,
    configuration: ScheduleConfiguration,
  ) => Promise<boolean>;
  onPauseSchedule: (schedule: API.ScheduleResponse) => Promise<void>;
  onResumeSchedule: (
    schedule: API.ScheduleResponse,
    misfirePolicy: API.ScheduleResumeRequest["misfire_policy"],
    maxCatchUp: number,
  ) => Promise<boolean>;
  onTriggerSchedule: (schedule: API.ScheduleResponse) => Promise<void>;
}) {
  const [configurationTarget, setConfigurationTarget] =
    useState<API.ScheduleResponse | null>(null);
  const [cancellationTarget, setCancellationTarget] =
    useState<API.TaskResponse | null>(null);
  const [configurationKind, setConfigurationKind] =
    useState<API.ScheduleConfigurationRequest["kind"]>("interval");
  const [configurationPolicy, setConfigurationPolicy] =
    useState<API.ScheduleConfigurationRequest["misfire_policy"]>("run_once");
  const [resumeTarget, setResumeTarget] =
    useState<API.ScheduleResponse | null>(null);
  const [resumePolicy, setResumePolicy] =
    useState<API.ScheduleResumeRequest["misfire_policy"]>("run_once");
  const running = tasks.filter((task) =>
    ["queued", "running", "waiting_provider"].includes(task.status),
  ).length;

  function openConfiguration(schedule: API.ScheduleResponse) {
    setConfigurationKind(schedule.kind === "cron" ? "cron" : "interval");
    setConfigurationPolicy(schedule.misfire_policy);
    setConfigurationTarget(schedule);
  }

  function openResume(schedule: API.ScheduleResponse) {
    setResumePolicy(schedule.misfire_policy);
    setResumeTarget(schedule);
  }

  async function submitConfiguration(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!configurationTarget) return;
    const form = new FormData(event.currentTarget);
    const maxCatchUp =
      configurationPolicy === "catch_up"
        ? Number(form.get("maxCatchUp"))
        : 0;
    const configured = await onConfigureSchedule(configurationTarget, {
      kind: configurationKind,
      interval_seconds:
        configurationKind === "interval"
          ? Number(form.get("intervalSeconds"))
          : null,
      cron_expression:
        configurationKind === "cron"
          ? String(form.get("cronExpression") ?? "")
          : null,
      timezone:
        configurationKind === "cron"
          ? String(form.get("timezone") ?? "UTC")
          : "UTC",
      misfire_policy: configurationPolicy,
      max_catch_up: maxCatchUp,
      misfire_grace_seconds: Number(form.get("misfireGraceSeconds")),
    });
    if (configured) setConfigurationTarget(null);
  }

  async function submitResume(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!resumeTarget) return;
    const form = new FormData(event.currentTarget);
    const resumed = await onResumeSchedule(
      resumeTarget,
      resumePolicy,
      resumePolicy === "catch_up" ? Number(form.get("resumeMaxCatchUp")) : 0,
    );
    if (resumed) setResumeTarget(null);
  }

  async function confirmGenerationCancellation() {
    if (!cancellationTarget) return;
    if (await onCancelGenerationTask(cancellationTarget)) {
      setCancellationTarget(null);
    }
  }
  const failed = tasks.filter((task) => ["failed", "unknown"].includes(task.status)).length;

  return (
    <div className="grid gap-6">
      <div className="grid gap-4 sm:grid-cols-3">
        <Card><CardHeader><CardDescription>全部任务</CardDescription><CardTitle className="text-3xl">{tasks.length}</CardTitle></CardHeader></Card>
        <Card><CardHeader><CardDescription>进行中</CardDescription><CardTitle className="text-3xl text-foreground">{running}</CardTitle></CardHeader></Card>
        <Card><CardHeader><CardDescription>需处理</CardDescription><CardTitle className="text-3xl text-rose-700">{failed}</CardTitle></CardHeader></Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle aria-level={2} role="heading">
            AI 生成能力与费用事实
          </CardTitle>
          <CardDescription>
            能力状态、价格版本和费用均来自服务端；空 API Key 占位不会自动激活模型。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          {productionFactsUnavailable ? (
            <Alert variant="destructive">
              <AlertCircle aria-hidden="true" />
              <AlertTitle>生产事实暂时不可读取</AlertTitle>
              <AlertDescription>
                请检查 API 与数据库连接；页面不会把依赖故障误报为模型可用。
              </AlertDescription>
            </Alert>
          ) : productionFactsLoading ? (
            <Alert>
              <RefreshCw className="animate-spin" aria-hidden="true" />
              <AlertTitle>正在读取生产事实</AlertTitle>
              <AlertDescription>正在同步能力目录与项目费用账本。</AlertDescription>
            </Alert>
          ) : (
            <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
              <div className="grid gap-3">
                {capabilities.map((capability) => (
                  <article
                    className="rounded-xl border border-slate-200 p-4"
                    key={capability.id}
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="flex items-start gap-3">
                        <div className="rounded-lg bg-muted p-2 text-foreground">
                          {capability.kind === "image" ? (
                            <ImageIcon className="size-4" aria-hidden="true" />
                          ) : (
                            <Video className="size-4" aria-hidden="true" />
                          )}
                        </div>
                        <div>
                          <h3 className="font-medium">
                            {capability.kind === "image" ? "图片生成" : "视频生成"}
                          </h3>
                          <p className="mt-1 break-all text-xs text-slate-500">
                            {capability.model} · 配置 v{capability.config_version}
                          </p>
                        </div>
                      </div>
                      <Badge
                        className={
                          capability.status === "active"
                            ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                            : "border-amber-200 bg-amber-50 text-amber-700"
                        }
                        variant="outline"
                      >
                        {capabilityStatusLabels[capability.status]}
                      </Badge>
                    </div>
                    <p className="mt-3 text-sm text-slate-600">
                      {capability.pricing
                        ? `每次请求 ${formatMoney(capability.pricing.amount, capability.pricing.currency)}`
                        : capabilityReasonLabels[
                            capability.unavailable_reason ?? ""
                          ] ?? "当前能力没有可提交的价格契约"}
                    </p>
                  </article>
                ))}
              </div>

              <div className="rounded-xl border border-slate-200 p-4">
                <div className="flex items-center gap-2 text-slate-600">
                  <Coins className="size-4 text-foreground" aria-hidden="true" />
                  <h3 className="font-medium">项目费用账本</h3>
                </div>
                {costs ? (
                  <dl className="mt-4 grid gap-3 text-sm">
                    <div className="flex items-center justify-between gap-4">
                      <dt className="text-slate-500">累计预占</dt>
                      <dd className="font-medium">
                        {formatMoney(costs.summary.reserved, costs.currency)}
                      </dd>
                    </div>
                    <div className="flex items-center justify-between gap-4">
                      <dt className="text-slate-500">已结算</dt>
                      <dd className="font-medium">
                        {formatMoney(costs.summary.settled, costs.currency)}
                      </dd>
                    </div>
                    <div className="flex items-center justify-between gap-4">
                      <dt className="text-slate-500">当前仍预占</dt>
                      <dd className="font-medium text-foreground">
                        {formatMoney(
                          costs.summary.remaining_reserved,
                          costs.currency,
                        )}
                      </dd>
                    </div>
                    <div className="border-t border-slate-100 pt-3 text-xs text-slate-500">
                      {costs.total > 0
                        ? `${costs.total} 条追加账本记录；queued 请求只显示预占，不伪装为已结算。`
                        : "还没有费用记录；生成预检不会创建预占。"}
                    </div>
                  </dl>
                ) : null}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>任务时间线</CardTitle>
          <CardDescription>刷新页面后仍从后端 Task 事实恢复，不读取消息队列猜测状态。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {tasks.length === 0 ? (
            <Alert>
              <Clock3 aria-hidden="true" />
              <AlertTitle>还没有任务</AlertTitle>
              <AlertDescription>发布剧本后启动结构提取，或上传媒体开始探测。</AlertDescription>
            </Alert>
          ) : (
            tasks.map((task) => (
              <article className="grid gap-3 rounded-xl border border-slate-200 p-4 md:grid-cols-[1fr_auto] md:items-center" key={task.id}>
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-medium">{taskTypeLabels[task.task_type]}</h3>
                    <Badge className={taskTone(task.status)} variant="outline">
                      {taskStatusLabels[task.status]}
                    </Badge>
                  </div>
                  <p className="mt-1 text-sm text-slate-500">
                    阶段：{task.progress_stage} · revision {task.revision}
                  </p>
                  {task.error ? (
                    <p className="mt-2 text-sm text-rose-700">
                      {task.error.summary} · {task.next_action ?? "等待人工处理"}
                    </p>
                  ) : null}
                </div>
                <div className="flex flex-wrap items-center justify-end gap-2 text-sm text-slate-500">
                  <span className="flex items-center gap-2">
                    {task.status === "succeeded" ? (
                      <CheckCircle2 className="size-4 text-emerald-600" aria-hidden="true" />
                    ) : task.status === "failed" || task.status === "unknown" ? (
                      <AlertCircle className="size-4 text-rose-600" aria-hidden="true" />
                    ) : task.status === "cancelled" ? (
                      <CircleX className="size-4 text-slate-500" aria-hidden="true" />
                    ) : (
                      <RefreshCw className="size-4 animate-spin text-foreground" aria-hidden="true" />
                    )}
                    {requestTypeLabels[task.request_type]}
                  </span>
                  {task.request_type === "generation_request" &&
                  (task.task_type === "image_generation" ||
                    task.task_type === "video_generation") &&
                  task.status === "queued" &&
                  task.cancel_status === "none" ? (
                    <Button
                      disabled={busy}
                      onClick={() => setCancellationTarget(task)}
                      size="sm"
                      variant="outline"
                    >
                      <CircleX aria-hidden="true" />
                      取消任务
                    </Button>
                  ) : null}
                </div>
              </article>
            ))
          )}
        </CardContent>
      </Card>

      <Dialog
        open={Boolean(cancellationTarget)}
        onOpenChange={(open) => {
          if (!open) setCancellationTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>取消排队中的生成任务</DialogTitle>
            <DialogDescription>
              该任务尚未发送给图片或视频模型。确认后任务会终止，并由服务端释放尚未使用的全部预占费用；此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              disabled={busy}
              onClick={() => setCancellationTarget(null)}
              type="button"
              variant="outline"
            >
              返回
            </Button>
            <Button
              disabled={busy}
              onClick={() => void confirmGenerationCancellation()}
              type="button"
              variant="destructive"
            >
              确认取消
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Card>
        <CardHeader>
          <CardTitle>媒体清理计划</CardTitle>
          <CardDescription>
            PostgreSQL 同时保存每次上传的到期主计划与工作空间周期补偿；暂停不会取消已经创建的任务。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {schedules.length === 0 ? (
            <Alert>
              <Clock3 aria-hidden="true" />
              <AlertTitle>还没有清理计划</AlertTitle>
              <AlertDescription>初始化媒体上传后，这里会显示对应的到期清理事实。</AlertDescription>
            </Alert>
          ) : (
            schedules.map((schedule) => (
              <article
                className="grid gap-4 rounded-xl border border-slate-200 p-4 lg:grid-cols-[1fr_auto] lg:items-center"
                key={schedule.id}
              >
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-medium">
                      {scheduleHandlerLabels[schedule.handler_name]}
                    </h3>
                    <Badge className={scheduleTone(schedule.status)} variant="outline">
                      {scheduleStatusLabels[schedule.status]}
                    </Badge>
                  </div>
                  <p className="mt-1 text-sm text-slate-500">
                    下次触发：{formatDateTime(schedule.next_fire_at)} · 失败 {schedule.failure_count} 次
                  </p>
                  <p className="mt-1 text-xs text-slate-500">
                    {scheduleRuleLabel(schedule)} · {schedule.timezone} · {misfirePolicyLabels[schedule.misfire_policy]}
                    {schedule.misfire_policy === "catch_up"
                      ? `（最多 ${schedule.max_catch_up} 次）`
                      : ""}
                  </p>
                  {schedule.last_error ? (
                    <p className="mt-2 text-sm text-rose-700">
                      {schedule.last_error} · 请检查 Scheduler、数据库和对象存储
                    </p>
                  ) : null}
                </div>
                <div className="flex flex-wrap gap-2">
                  {schedule.handler_name === "cleanup_expired_uploads" &&
                  schedule.status !== "completed" ? (
                    <Button
                      disabled={busy}
                      onClick={() => openConfiguration(schedule)}
                      size="sm"
                      variant="outline"
                    >
                      <Settings2 aria-hidden="true" />
                      配置周期
                    </Button>
                  ) : null}
                  {schedule.status === "active" ? (
                    <Button
                      disabled={busy}
                      onClick={() => void onPauseSchedule(schedule)}
                      size="sm"
                      variant="outline"
                    >
                      <Pause aria-hidden="true" />
                      暂停
                    </Button>
                  ) : schedule.status === "paused" ? (
                    <Button
                      disabled={busy}
                      onClick={() => openResume(schedule)}
                      size="sm"
                      variant="outline"
                    >
                      <Play aria-hidden="true" />
                      恢复并执行
                    </Button>
                  ) : null}
                  {schedule.status !== "completed" &&
                  schedule.handler_name !== "unregistered" ? (
                    <Button
                      disabled={busy}
                      onClick={() => void onTriggerSchedule(schedule)}
                      size="sm"
                    >
                      <RefreshCw aria-hidden="true" />
                      立即触发
                    </Button>
                  ) : null}
                </div>
              </article>
            ))
          )}
        </CardContent>
      </Card>

      <Dialog
        open={Boolean(configurationTarget)}
        onOpenChange={(open) => {
          if (!open) setConfigurationTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>配置补偿清理周期</DialogTitle>
            <DialogDescription>
              只修改已登记的过期上传补偿计划；不接受任意 handler、函数路径或 payload。
            </DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={submitConfiguration}>
            <div className="grid gap-2">
              <Label>计划类型</Label>
              <Select
                value={configurationKind}
                onValueChange={(value) =>
                  setConfigurationKind(
                    value as API.ScheduleConfigurationRequest["kind"],
                  )
                }
              >
                <SelectTrigger className="w-full" aria-label="计划类型">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="interval">固定秒数</SelectItem>
                  <SelectItem value="cron">Cron + IANA 时区</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {configurationKind === "interval" ? (
              <div className="grid gap-2">
                <Label htmlFor="intervalSeconds">间隔秒数</Label>
                <Input
                  defaultValue={
                    configurationTarget && "seconds" in configurationTarget.rule
                      ? configurationTarget.rule.seconds
                      : 3600
                  }
                  id="intervalSeconds"
                  max={86400}
                  min={60}
                  name="intervalSeconds"
                  required
                  type="number"
                />
              </div>
            ) : (
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="grid gap-2 sm:col-span-2">
                  <Label htmlFor="cronExpression">数字五段 Cron</Label>
                  <Input
                    defaultValue={
                      configurationTarget && "expression" in configurationTarget.rule
                        ? configurationTarget.rule.expression
                        : "0 * * * *"
                    }
                    id="cronExpression"
                    name="cronExpression"
                    required
                  />
                  <p className="text-xs text-slate-500">
                    顺序为分、时、日、月、星期；支持数字、星号、列表、范围和步长。
                  </p>
                </div>
                <div className="grid gap-2 sm:col-span-2">
                  <Label htmlFor="scheduleTimezone">IANA 时区</Label>
                  <Input
                    defaultValue={configurationTarget?.timezone ?? "Asia/Shanghai"}
                    id="scheduleTimezone"
                    name="timezone"
                    placeholder="Asia/Shanghai"
                    required
                  />
                </div>
              </div>
            )}

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label>停机补偿策略</Label>
                <Select
                  value={configurationPolicy}
                  onValueChange={(value) =>
                    setConfigurationPolicy(
                      value as API.ScheduleConfigurationRequest["misfire_policy"],
                    )
                  }
                >
                  <SelectTrigger className="w-full" aria-label="停机补偿策略">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="skip">跳过过期时点</SelectItem>
                    <SelectItem value="run_once">合并补执一次</SelectItem>
                    <SelectItem value="catch_up">有界逐次补执</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="misfireGraceSeconds">超时宽限（秒）</Label>
                <Input
                  defaultValue={
                    configurationTarget &&
                    "misfire_grace_seconds" in configurationTarget.rule
                      ? configurationTarget.rule.misfire_grace_seconds
                      : 30
                  }
                  id="misfireGraceSeconds"
                  max={3600}
                  min={0}
                  name="misfireGraceSeconds"
                  required
                  type="number"
                />
              </div>
            </div>
            {configurationPolicy === "catch_up" ? (
              <div className="grid gap-2">
                <Label htmlFor="maxCatchUp">最多补执次数</Label>
                <Input
                  defaultValue={configurationTarget?.max_catch_up || 3}
                  id="maxCatchUp"
                  max={20}
                  min={1}
                  name="maxCatchUp"
                  required
                  type="number"
                />
              </div>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setConfigurationTarget(null)}
              >
                取消
              </Button>
              <Button disabled={busy} type="submit">保存计划配置</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(resumeTarget)}
        onOpenChange={(open) => {
          if (!open) setResumeTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>恢复调度计划</DialogTitle>
            <DialogDescription>
              暂停期间未创建的历史时点必须以明确策略处置，已创建的 Task 不会被取消。
            </DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={submitResume}>
            <div className="grid gap-2">
              <Label>恢复策略</Label>
              <Select
                value={resumePolicy}
                onValueChange={(value) =>
                  setResumePolicy(
                    value as API.ScheduleResumeRequest["misfire_policy"],
                  )
                }
              >
                <SelectTrigger className="w-full" aria-label="恢复策略">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="skip">跳过历史时点</SelectItem>
                  <SelectItem value="run_once">合并补执一次</SelectItem>
                  <SelectItem value="catch_up">有界逐次补执</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {resumePolicy === "catch_up" ? (
              <div className="grid gap-2">
                <Label htmlFor="resumeMaxCatchUp">最多补执次数</Label>
                <Input
                  defaultValue={resumeTarget?.max_catch_up || 3}
                  id="resumeMaxCatchUp"
                  max={20}
                  min={1}
                  name="resumeMaxCatchUp"
                  required
                  type="number"
                />
              </div>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setResumeTarget(null)}
              >
                取消
              </Button>
              <Button disabled={busy} type="submit">确认恢复</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
