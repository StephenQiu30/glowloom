"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  loadOnboarding,
  rollbackOnboarding,
  verifyOnboarding,
  type OnboardingReview,
  type OnboardingSummary,
} from "@/lib/ai-gateway-admin";
import { InputField, type RunMutation } from "./gateway-forms";

export const onboardingState: Record<OnboardingSummary["state"], string> = {
  prepared: "待验证",
  verifying: "验证中",
  active: "已启用",
  failed: "验证失败",
  stale: "线上版本已更新",
  rolled_back: "已回退",
};

export function OnboardingPanel({
  id,
  initial,
  run,
  busy,
  refreshConfiguration,
}: {
  id: string;
  initial?: OnboardingReview;
  run: RunMutation;
  busy: boolean;
  refreshConfiguration: () => void;
}) {
  const [review, setReview] = useState(initial);
  const [readError, setReadError] = useState(false);
  const [revision, setRevision] = useState(0);

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

  return (
    <Card>
      <CardHeader>
        <CardTitle>API Key 与最终验证</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {readError && (
          <p role="alert">验证回执暂时无法读取。重新读取会查询原验证。</p>
        )}
        {!review && !readError && <p role="status">正在读取验证计划…</p>}
        {review && (
          <>
            <p role="status">
              {review.upstream_model} · {onboardingState[review.state]}
            </p>
            <dl className="grid gap-2 text-sm sm:grid-cols-2">
              <div>
                <dt>测试付款空间</dt>
                <dd className="break-all">{review.scope_id}</dd>
              </div>
              <div>
                <dt>销售版本</dt>
                <dd className="break-all">{review.model_id}</dd>
              </div>
              <div>
                <dt>采购版本</dt>
                <dd className="break-all">{review.id}</dd>
              </div>
              <div>
                <dt>确认上限</dt>
                <dd>
                  {review.plan.credits} 积分 · 采购{" "}
                  {review.plan.purchase_micros} 微人民币
                </dd>
              </div>
              <div>
                <dt>验证次数</dt>
                <dd>{review.plan.probes.length} 次</dd>
              </div>
            </dl>
            <Collapsible>
              <CollapsibleTrigger asChild>
                <Button type="button" variant="outline">
                  查看验证请求
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <ol className="mt-2 space-y-2">
                  {review.plan.probes.map((probe, index) => (
                    <li key={probe.name}>
                      <p>
                        {probe.name}
                        {review.progress[index]?.state === "unknown" &&
                          " · 待对账"}
                      </p>
                      <pre className="overflow-auto rounded-md bg-muted p-3 text-xs break-all whitespace-pre-wrap">
                        {JSON.stringify(probe.input, null, 2)}
                      </pre>
                    </li>
                  ))}
                </ol>
              </CollapsibleContent>
            </Collapsible>
            {review.state === "prepared" && !readError && (
              <form
                aria-label="确认预算并验证"
                onSubmit={(event) => {
                  event.preventDefault();
                  const form = event.currentTarget;
                  const data = new FormData(form);
                  if (data.get("confirm_budget") !== "on") return;
                  const key = String(data.get("key") ?? "");
                  const input = form.elements.namedItem("key");
                  if (input instanceof HTMLInputElement) input.value = "";
                  void run(async () => {
                    try {
                      setReview(await verifyOnboarding(review, key));
                    } finally {
                      setRevision((value) => value + 1);
                    }
                  }, "验证已提交，通过后自动启用。");
                }}
              >
                <FieldGroup>
                  <InputField label="API Key" name="key" type="password" />
                  <Field orientation="horizontal">
                    <FieldLabel>
                      <Checkbox name="confirm_budget" required />
                      确认以上测试和费用上限
                    </FieldLabel>
                  </Field>
                  <Button type="submit" disabled={busy}>
                    确认预算并验证
                  </Button>
                </FieldGroup>
              </form>
            )}
            {review.state === "failed" && (
              <p>可使用相同模型预设重新准备路由并验证。</p>
            )}
            {review.state === "stale" && (
              <p>请基于目前的线上版本重新准备接入。</p>
            )}
            {review.state === "active" && (
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button type="button" variant="outline">
                    回退该候选
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <p className="my-2 text-sm">
                    停止该路由接收新单，恢复仍合格的旧版本。
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={busy}
                    onClick={() => {
                      void run(async () => {
                        await rollbackOnboarding(id);
                        setRevision((value) => value + 1);
                      }, "已停止该候选接收新单。");
                    }}
                  >
                    确认回退
                  </Button>
                </CollapsibleContent>
              </Collapsible>
            )}
          </>
        )}
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() => setRevision((value) => value + 1)}
        >
          读取最新回执
        </Button>
      </CardContent>
    </Card>
  );
}
