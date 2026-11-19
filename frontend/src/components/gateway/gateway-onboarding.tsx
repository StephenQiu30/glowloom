"use client";

import { useEffect, useState } from "react";
import { ArrowLeft, RefreshCw } from "lucide-react";
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
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { FieldGroup } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import {
  loadOnboarding,
  rollbackOnboarding,
  verifyOnboarding,
  type OnboardingReview,
  type OnboardingSummary,
} from "@/lib/ai-gateway-admin";
import {
  CheckField,
  FormDetails,
  InputField,
  SubmitButton,
  money,
  type RunMutation,
} from "./gateway-fields";
import { modeLabel } from "./gateway-media-fields";

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
  run,
  busy,
  refreshConfiguration,
  back,
  retryRoute,
}: {
  id: string;
  initial?: OnboardingReview;
  run: RunMutation;
  busy: boolean;
  refreshConfiguration: () => void;
  back: () => void;
  retryRoute: (model: string) => void;
}) {
  const [review, setReview] = useState(initial);
  const [readError, setReadError] = useState(false);
  const [revision, setRevision] = useState(0);
  const [confirmed, setConfirmed] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let verifying = false;
    async function read() {
      try {
        const next = await loadOnboarding(id, controller.signal);
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
  }, [id, revision, refreshConfiguration]);

  const unknown = review?.progress.some((record) => record.state === "unknown");
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex flex-wrap justify-between gap-2">
        <Button
          type="button"
          variant="ghost"
          className="max-md:min-h-11"
          onClick={back}
        >
          <ArrowLeft data-icon="inline-start" aria-hidden="true" />
          返回验证列表
        </Button>
        <Button
          type="button"
          variant="outline"
          className="max-md:min-h-11"
          disabled={busy}
          onClick={() => setRevision((value) => value + 1)}
        >
          <RefreshCw data-icon="inline-start" aria-hidden="true" />
          读取最新回执
        </Button>
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
          <Card>
            <CardHeader>
              <CardTitle>验证采购上限</CardTitle>
              <CardDescription className="wrap-anywhere">
                {review.upstream_model}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
                <Summary
                  label="人民币"
                  value={money(review.plan.purchase_micros)}
                />
                <Summary
                  label="平台积分"
                  value={`${review.plan.credits} 积分`}
                />
                <Summary
                  label="供应商验证请求"
                  value={`${review.plan.probes.length} 次`}
                />
              </dl>
              <div className="flex items-center gap-2" role="status">
                <Badge
                  variant={review.state === "failed" ? "outline" : "secondary"}
                >
                  {unknown ? "待对账" : onboardingState[review.state]}
                </Badge>
                {unknown && (
                  <span className="text-sm text-muted-foreground">
                    供应商受理状态待确认。
                  </span>
                )}
              </div>
              <Separator />
              <ul className="flex flex-col gap-3" aria-label="验证用例">
                {review.plan.probes.map((probe, index) => (
                  <ProbeSummary
                    key={probe.name}
                    probe={probe}
                    progress={review.progress[index]}
                  />
                ))}
              </ul>
              <FormDetails label="验证对象与版本">
                <dl className="grid gap-3 sm:grid-cols-2">
                  <Summary
                    label="测试付款空间"
                    value={review.scope_id}
                    compact
                  />
                  <Summary label="销售版本" value={review.model_id} compact />
                  <Summary label="采购版本" value={review.id} compact />
                </dl>
              </FormDetails>
            </CardContent>
          </Card>
          {review.state === "prepared" && !readError && (
            <Card>
              <CardHeader>
                <CardTitle>API Key 与最终验证</CardTitle>
                <CardDescription>
                  验证通过后自动启用，或切换到这个版本。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <form
                  aria-label="确认预算并验证"
                  onSubmit={(event) => {
                    event.preventDefault();
                    const form = event.currentTarget;
                    const data = new FormData(form);
                    if (!confirmed || data.get("confirm_budget") !== "on")
                      return;
                    const key = String(data.get("key") ?? "");
                    const input = form.elements.namedItem("key");
                    if (input instanceof HTMLInputElement) input.value = "";
                    setConfirmed(false);
                    void run(async () => {
                      try {
                        setReview(await verifyOnboarding(review, key));
                      } finally {
                        setRevision((value) => value + 1);
                      }
                    }, "验证已提交。");
                  }}
                >
                  <FieldGroup>
                    <InputField
                      label="API Key"
                      name="key"
                      type="password"
                      maxLength={8192}
                    />
                    <CheckField
                      label="确认以上测试和费用上限"
                      name="confirm_budget"
                      required
                      checked={confirmed}
                      onCheckedChange={(value) => setConfirmed(value === true)}
                    />
                    <div>
                      <SubmitButton busy={busy} disabled={!confirmed}>
                        确认预算并验证
                      </SubmitButton>
                    </div>
                  </FieldGroup>
                </form>
              </CardContent>
            </Card>
          )}
          {unknown && (
            <Alert>
              <AlertTitle>先核对原请求</AlertTitle>
              <AlertDescription>
                请在积分与对账中按供应商证据处理原验证请求。
              </AlertDescription>
            </Alert>
          )}
          {(review.state === "failed" || review.state === "stale") && (
            <Alert variant="destructive">
              <AlertTitle>{onboardingState[review.state]}</AlertTitle>
              <AlertDescription>
                当前线上版本继续服务。
                {!unknown && (
                  <Button
                    type="button"
                    variant="link"
                    className="h-auto px-0 max-md:min-h-11"
                    onClick={() => retryRoute(review.model_id)}
                  >
                    重新准备采购路由
                  </Button>
                )}
              </AlertDescription>
            </Alert>
          )}
          {review.state === "active" && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  className="w-fit max-md:min-h-11"
                >
                  回退该候选
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>确认回退当前版本？</AlertDialogTitle>
                  <AlertDialogDescription>
                    该路由停止接收新单。仍合格的旧版本会恢复，已提交任务继续按原记录处理。
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>取消</AlertDialogCancel>
                  <AlertDialogAction
                    disabled={busy}
                    onClick={() => {
                      void run(async () => {
                        await rollbackOnboarding(id);
                        setRevision((value) => value + 1);
                      }, "已回退该版本。");
                    }}
                  >
                    确认回退
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
    <div className="flex min-w-0 flex-col-reverse gap-1">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd
        className={
          compact
            ? "font-mono text-sm break-all"
            : "text-xl font-semibold wrap-anywhere tabular-nums"
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
      <p className="font-medium">
        {modeLabel(probe.name)}
        {progress && (
          <span className="ml-2 text-sm font-normal text-muted-foreground">
            {progressLabels[progress.state] ?? "状态待更新"}
          </span>
        )}
      </p>
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
        <p className="font-mono text-xs break-all text-muted-foreground">
          原请求 {progress.request_id}
        </p>
      )}
    </li>
  );
}

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
