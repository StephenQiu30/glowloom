"use client";

import { useEffect, useState, type Ref } from "react";
import {
  ArrowLeft,
  RefreshCw,
  Undo2,
  Route,
  Check,
  X,
  ShieldCheck,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { FieldGroup } from "@/components/ui/field";
import { AiServiceAction } from "./admin-action";
import { Skeleton } from "@/components/ui/skeleton";
import {
  isReadOnlyKeyCheck,
  loadOnboarding,
  rollbackOnboarding,
  verifyOnboarding,
  type AiServiceConfiguration,
  type OnboardingReview,
  type OnboardingSummary,
} from "@/lib/ai-service-admin";
import {
  CheckField,
  FormDetails,
  InputField,
  SubmitButton,
  money,
  priceSummary,
  type RunMutation,
} from "./admin-fields";
import { RequestError } from "@/lib/request";
import { modeLabel } from "./media-fields";

export const onboardingState: Record<OnboardingSummary["state"], string> = {
  prepared: "待验证",
  verifying: "验证中",
  active: "已启用",
  failed: "验证失败",
  stale: "线上版本已更新",
  rolled_back: "已回退",
};

const progressLabels: Record<string, string> = {
  sending: "请求已提交",
  pending: "供应商处理中",
  unknown: "待对账",
  received: "已收到结果",
  settled: "已结算",
  released: "已退还预留积分",
};

export function OnboardingPanel({
  id,
  initial,
  models,
  headingRef,
  run,
  busy,
  refreshConfiguration,
  back,
  retryRoute,
  onKeyDirty,
}: {
  id: string;
  initial?: OnboardingReview;
  models: AiServiceConfiguration["models"];
  headingRef?: Ref<HTMLHeadingElement>;
  run: RunMutation;
  busy: boolean;
  refreshConfiguration: () => void;
  back?: () => void;
  retryRoute: (model: string) => void;
  onKeyDirty?: (dirty: boolean) => void;
}) {
  const [review, setReview] = useState(initial);
  const [readError, setReadError] = useState(false);
  const [revision, setRevision] = useState(0);
  const [keyEntered, setKeyEntered] = useState(false);
  const readID = review?.id ?? id;

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let verifying = false;
    async function read() {
      try {
        const next = await loadOnboarding(readID, controller.signal);
        if (controller.signal.aborted) return;
        setReview(next);
        setReadError(false);
        if (next.state === "verifying") {
          verifying = true;
          timer = setTimeout(read, 5000);
        } else if (verifying) {
          refreshConfiguration();
        }
      } catch {
        if (!controller.signal.aborted) setReadError(true);
      }
    }
    void read();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [readID, revision, refreshConfiguration]);

  const keyCheck = review !== undefined && isReadOnlyKeyCheck(review.plan);
  const [trialApproved, setTrialApproved] = useState(false);
  const unknown = review?.progress.some((record) => record.state === "unknown");
  const availability =
    review?.state === "active"
      ? review.available === false
        ? "不可用"
        : "可用"
      : review?.state === "verifying" && !unknown
        ? "验证中"
        : review?.state === "prepared"
          ? "待验证"
          : "不可用";
  const model = models.find(
    ({ model }) => model.id === review?.model_id,
  )?.model;
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {(back || readError) && (
        <div className="flex flex-wrap justify-between gap-2">
          {back && (
            <AiServiceAction
              onClick={back}
              label="返回验证列表"
              icon={ArrowLeft}
            />
          )}
          <div className="ml-auto">
            <AiServiceAction
              disabled={busy}
              onClick={() => setRevision((value) => value + 1)}
              label="读取最新回执"
              icon={RefreshCw}
            />
          </div>
        </div>
      )}
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <h3 ref={headingRef} tabIndex={-1} className="text-sm font-semibold">
          接入验证
        </h3>
        {review && (
          <Badge
            role="status"
            variant={review.state === "failed" ? "outline" : "secondary"}
          >
            {back
              ? unknown
                ? "待对账"
                : onboardingState[review.state]
              : availability}
          </Badge>
        )}
      </div>
      {readError && (
        <Alert variant="destructive">
          <AlertTitle>无法读取验证回执</AlertTitle>
          <AlertDescription>请重试读取原验证。</AlertDescription>
        </Alert>
      )}
      {!review && !readError && (
        <Skeleton className="h-44 w-full" aria-label="正在读取验证计划" />
      )}
      {review && (
        <>
          {back && (
            <>
              <div className="flex flex-col gap-1">
                <p className="font-medium wrap-anywhere">
                  {review.upstream_model}
                </p>
                {model && (
                  <p className="text-sm wrap-anywhere text-muted-foreground">
                    <span className="mr-2 font-medium text-foreground">
                      销售基准
                    </span>
                    {priceSummary(model.price, model.protocol)}
                  </p>
                )}
              </div>
              {!keyCheck && (
                <dl className="flex flex-wrap gap-x-6 gap-y-3">
                  {typeof review.plan.estimated_credits === "number" && (
                    <Summary
                      label="预计验证积分"
                      value={`${review.plan.estimated_credits} 积分`}
                    />
                  )}
                  <Summary
                    label="最高验证积分"
                    value={`${review.plan.credits} 积分`}
                  />
                  <Summary
                    label="采购上限"
                    value={money(review.plan.purchase_micros)}
                  />
                </dl>
              )}
              {!keyCheck && (
                <div className="flex flex-col gap-1">
                  <p className="text-sm font-medium">
                    验证请求{" "}
                    <span className="tabular-nums">
                      {review.plan.probes.length} 次
                    </span>
                  </p>
                  <ul
                    aria-label="验证用例"
                    className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground"
                  >
                    {review.plan.probes.map((probe) => (
                      <li key={probe.name}>{probeLabel(probe)}</li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}
          {review.state === "prepared" && !readError && (
            <form
              aria-label={keyCheck ? "保存接入" : "确认生成试跑"}
              onSubmit={(event) => {
                event.preventDefault();
                const form = event.currentTarget;
                const data = new FormData(form);
                const key = String(data.get("key") ?? "").trim();
                if (!key || busy || (!keyCheck && !trialApproved)) return;
                const input = form.elements.namedItem("key");
                function clearKey() {
                  if (input instanceof HTMLInputElement) input.value = "";
                  setKeyEntered(false);
                  onKeyDirty?.(false);
                }
                void run(
                  async () => {
                    try {
                      setReview(
                        await verifyOnboarding(
                          review,
                          key,
                          !keyCheck && trialApproved,
                        ),
                      );
                      clearKey();
                    } catch (cause) {
                      if (
                        cause instanceof RequestError &&
                        cause.code === "insufficient_credits"
                      ) {
                        throw new Error(
                          `验证空间积分不足，需至少 ${review.plan.credits} 积分`,
                        );
                      }
                      if (!(
                        cause instanceof RequestError &&
                        [
                          "budget_exceeded",
                          "provider_key_rejected",
                          "provider_key_scope",
                        ].includes(cause.code)
                      )) {
                        clearKey();
                      }
                      setRevision((value) => value + 1);
                      throw cause;
                    }
                  },
                  keyCheck ? "接入已保存" : "试跑已提交",
                );
              }}
            >
              <FieldGroup className="gap-3">
                <InputField
                  label="API Key"
                  disabled={busy}
                  name="key"
                  type="password"
                  maxLength={8192}
                  description={keyCheck ? "只读鉴权 · 0 积分" : undefined}
                  onChange={(event) => {
                    const hasKey = Boolean(event.currentTarget.value.trim());
                    setKeyEntered(hasKey);
                    onKeyDirty?.(hasKey);
                  }}
                />
                {!keyCheck && !back && (
                  <p className="text-sm text-muted-foreground">
                    {typeof review.plan.estimated_credits === "number" &&
                      `预计 ${review.plan.estimated_credits} 积分 · `}
                    采购上限 {money(review.plan.purchase_micros)}
                  </p>
                )}
                {!keyCheck && (
                  <CheckField
                    checked={trialApproved}
                    onCheckedChange={(checked) =>
                      setTrialApproved(checked === true)
                    }
                    disabled={busy}
                    label={`确认生成试跑，最多使用 ${review.plan.credits} 积分`}
                  />
                )}
                <div>
                  <SubmitButton
                    busy={busy}
                    disabled={!keyEntered || (!keyCheck && !trialApproved)}
                    label={keyCheck ? "保存接入" : "确认并试跑"}
                    icon={ShieldCheck}
                  />
                </div>
              </FieldGroup>
            </form>
          )}
          {back && (
            <FormDetails label="验证详情" keepMounted={false}>
              {keyCheck ? (
                <p className="text-sm text-muted-foreground">
                  API Key 只读鉴权 · 0 积分
                </p>
              ) : (
                <ul className="flex flex-col gap-4">
                  {review.plan.probes.map((probe, index) => (
                    <ProbeSummary
                      key={probe.name}
                      probe={probe}
                      progress={review.progress[index]}
                    />
                  ))}
                </ul>
              )}
              <dl className="mt-4 grid gap-3 sm:grid-cols-2">
                <Summary label="测试付款空间" value={review.scope_id} compact />
                <Summary label="模型版本" value={review.model_id} compact />
                <Summary label="接入记录" value={review.id} compact />
              </dl>
            </FormDetails>
          )}
          {back && unknown && (
            <Alert>
              <AlertTitle>供应商受理状态待确认</AlertTitle>
              <AlertDescription>在对账中核对原请求后处理。</AlertDescription>
            </Alert>
          )}
          {back && (review.state === "failed" || review.state === "stale") && (
            <Alert variant="destructive">
              <AlertTitle>{onboardingState[review.state]}</AlertTitle>
              <AlertDescription>
                当前线上版本继续服务。
                {!unknown && (
                  <AiServiceAction
                    onClick={() => retryRoute(review.model_id)}
                    label="重新准备采购路由"
                    icon={Route}
                  />
                )}
              </AlertDescription>
            </Alert>
          )}
          {back && review.state === "active" && review.available !== false && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <AiServiceAction label="回退该候选" icon={Undo2} />
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>确认回退当前版本？</AlertDialogTitle>
                  <AlertDialogDescription>
                    该路由停止接收新单。仍合格的旧版本会恢复，已提交任务继续按原记录处理。
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel size="icon" aria-label="取消" title="取消">
                    <X aria-hidden="true" />
                  </AlertDialogCancel>
                  <AlertDialogAction
                    size="icon"
                    aria-label="确认回退"
                    title="确认回退"
                    disabled={busy}
                    onClick={() => {
                      void run(async () => {
                        await rollbackOnboarding(readID);
                        setRevision((value) => value + 1);
                      }, "已回退该版本。");
                    }}
                  >
                    <Check aria-hidden="true" />
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
        </>
      )}
    </div>
  );
}

function Summary({
  label,
  value,
  compact,
}: {
  label: string;
  value: string;
  compact?: boolean;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd
        className={
          compact
            ? "font-mono text-sm break-all"
            : "text-lg font-semibold wrap-anywhere tabular-nums"
        }
      >
        {value}
      </dd>
    </div>
  );
}

function ProbeSummary({
  probe,
  progress,
}: {
  probe: OnboardingReview["plan"]["probes"][number];
  progress?: OnboardingReview["progress"][number];
}) {
  const input = record(probe.input);
  const source = input.messages ?? input.input;
  const message = Array.isArray(source)
    ? source
        .map((item) => record(item).content)
        .find((value) => typeof value === "string")
    : source;
  const prompt =
    typeof input.prompt === "string"
      ? input.prompt
      : typeof message === "string"
        ? message
        : "";
  const maxOutput = input.max_tokens ?? input.max_output_tokens;
  const params = Object.entries(record(input.params)).filter(
    ([, value]) =>
      typeof value === "string" ||
      typeof value === "number" ||
      typeof value === "boolean",
  );
  const refs = Array.isArray(input.references) ? input.references.length : 0;
  return (
    <li className="flex min-w-0 flex-col gap-1">
      <p className="text-sm font-medium">{modeLabel(probe.name)}</p>
      {prompt && (
        <p className="text-sm wrap-anywhere whitespace-pre-wrap text-muted-foreground">
          {prompt}
        </p>
      )}
      {typeof maxOutput === "number" && (
        <p className="text-sm text-muted-foreground">
          最多输出 {maxOutput} token
        </p>
      )}
      {refs > 0 && (
        <p className="text-sm text-muted-foreground">测试素材 {refs} 个</p>
      )}
      {!!params.length && (
        <dl className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
          {params.map(([name, value]) => (
            <div className="flex min-w-0 gap-1 break-all" key={name}>
              <dt>{name}</dt>
              <dd>{String(value)}</dd>
            </div>
          ))}
        </dl>
      )}
      {progress && (
        <p className="text-sm wrap-anywhere text-muted-foreground">
          {progressLabels[progress.state] ?? "状态待更新"} ·{" "}
          {progress.request_id}
        </p>
      )}
    </li>
  );
}

function probeLabel(probe: OnboardingReview["plan"]["probes"][number]) {
  const input = record(probe.input);
  const { resolution, duration, ratio } = record(input.params);
  const maxOutput = input.max_tokens ?? input.max_output_tokens;
  return [
    modeLabel(probe.name),
    typeof resolution === "string" && resolution,
    typeof duration === "number" && `${duration} 秒`,
    typeof ratio === "string" && ratio,
    typeof maxOutput === "number" && `最多 ${maxOutput} Token`,
  ]
    .filter(Boolean)
    .join(" · ");
}

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
