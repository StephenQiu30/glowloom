"use client";

import { useId, useState } from "react";
import useSWR from "swr";
import { useSession } from "@/components/auth/session";
import { Check, RefreshCw } from "lucide-react";
import {
  activateProviderOnboarding,
  getProviderOnboarding,
  testProviderOnboardingModel,
} from "@/api/settings";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Badge } from "@/components/ui/badge";
import {
  catalogMessage,
  useCatalogMutation,
} from "@/components/catalog/mutation";

function accepted(value: unknown, provider: string) {
  if (!value || typeof value !== "object") return false;
  const receipt = value as API.CredentialTestAccepted;
  return (
    receipt.accepted === true &&
    receipt.provider_id === provider &&
    !!receipt.check_id &&
    !!receipt.model_id
  );
}

export function ConnectionOnboarding({
  id,
  onSaved,
  revision = 0,
}: {
  id: string;
  onSaved: (id: string) => void;
  revision?: number;
}) {
  const fieldID = useId();
  const session = useSession();
  const [requestedCheck, setRequestedCheck] = useState<{
    id: string;
    credentialID: string;
  } | null>(null);
  const [selected, setSelected] = useState("");
  const { data, error, isLoading, isValidating, mutate } = useSWR(
    ["provider-onboarding", session.actor.id, id, revision],
    () => getProviderOnboarding({ id }),
    {
      refreshInterval: (state) =>
        requestedCheck ||
        ["pending", "running"].includes(state?.check?.status ?? "")
          ? 2500
          : 0,
      revalidateOnFocus: true,
      onSuccess: (state) => {
        if (
          state.check?.id === requestedCheck?.id ||
          state.credential_id !== requestedCheck?.credentialID
        )
          setRequestedCheck(null);
      },
    },
  );
  const test = useCatalogMutation(
    (value) => {
      const receipt = value as API.CredentialTestAccepted;
      setRequestedCheck({
        id: receipt.check_id!,
        credentialID: data?.credential_id ?? "",
      });
      setSelected(receipt.model_id!);
      void mutate();
    },
    `provider.onboarding.test.${id}`,
    accepted,
  );
  const activate = useCatalogMutation(
    () => {
      void mutate();
      onSaved(id);
    },
    `provider.onboarding.activate.${id}`,
    (value) => {
      const result = value as API.ProviderOnboardingState | undefined;
      return (
        result?.provider_id === id &&
        result.status === "active" &&
        !!result.models?.length &&
        result.models.every((m) => m.status === "active")
      );
    },
  );
  const check = data?.check;
  const model =
    (data?.models?.some((m) => m.id === selected) ? selected : "") ||
    check?.model_id ||
    data?.models?.find((m) => m.capability === "text.generate")?.id ||
    data?.models?.[0]?.id ||
    "";
  const running = ["pending", "running"].includes(check?.status ?? "");
  const selectedCheck = check?.model_id === model ? check : undefined;
  const verified =
    selectedCheck?.status === "completed" &&
    selectedCheck.auth_result === "ok" &&
    ["completed", "accepted"].includes(selectedCheck.model_result ?? "");
  const awaitingCheck =
    !!requestedCheck &&
    requestedCheck.credentialID === data?.credential_id &&
    data?.check?.id !== requestedCheck.id;
  const waiting = running || awaitingCheck;
  const busy = test.busy || activate.busy || waiting;
  if (isLoading)
    return (
      <p role="status" className="flex items-center gap-2 text-sm">
        <Spinner />
        正在读取模型验证状态…
      </p>
    );
  if (error)
    return (
      <Alert variant="destructive">
        <AlertTitle>无法读取模型验证状态</AlertTitle>
        <AlertDescription>
          {catalogMessage(error)}
          <Button type="button" variant="outline" onClick={() => void mutate()}>
            重试
          </Button>
        </AlertDescription>
      </Alert>
    );
  if (!data?.models?.length) return null;
  const errors: Record<string, string> = {
    auth_failed: "API Key 无效或模型未授权。",
    insufficient_balance: "服务商余额不足，请充值后重试。",
    rate_limited: "服务商限流，请稍后重试。",
    not_found: "模型不可用，请检查区域与版本。",
    invalid_parameters: "验证参数无效，请检查模型配置。",
  };
  const result = !data.credential_id
    ? "未配置"
    : test.busy || waiting
      ? "验证中"
      : test.pending
        ? "待确认"
        : verified
          ? selectedCheck?.model_result === "accepted"
            ? "已受理"
            : "已通过"
          : selectedCheck?.status === "failed"
            ? "未通过"
            : "未验证";
  const successMessages = [
    "已保存。",
    "原请求已保存，已恢复当前配置。",
    "请求已有成功回执，已刷新当前配置。",
    "已找到成功回执，列表已刷新。",
  ];
  return (
    <div className="flex min-w-0 flex-col gap-3">
      <Field>
        <div className="flex items-center justify-between gap-2">
          <FieldLabel variant="title" htmlFor={`${fieldID}-model`}>
            模型验证
          </FieldLabel>
          <Badge
            variant={result === "未通过" ? "destructive" : "secondary"}
            role="status"
          >
            {result === "验证中" ? (
              <Spinner data-icon="inline-start" />
            ) : result === "已通过" || result === "已受理" ? (
              <Check data-icon="inline-start" />
            ) : null}
            {result}
          </Badge>
        </div>
        <div className="flex min-w-0 items-center gap-2">
          <Select
            value={model}
            disabled={busy || !!test.pending || !!activate.pending}
            onValueChange={setSelected}
          >
            <SelectTrigger
              id={`${fieldID}-model`}
              className="min-w-0 flex-1 max-md:min-h-11"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {data.models.map((m) =>
                  m.id ? (
                    <SelectItem key={m.id} value={m.id}>
                      {m.display_name}
                    </SelectItem>
                  ) : null,
                )}
              </SelectGroup>
            </SelectContent>
          </Select>
          {test.pending ? (
            <Button
              type="button"
              variant="outline"
              disabled={test.busy}
              onClick={test.recover}
              className="shrink-0 max-md:h-11"
            >
              {test.busy ? <Spinner data-icon="inline-start" /> : null}
              {test.busy ? "提交中…" : "查询回执"}
            </Button>
          ) : waiting ? (
            <Button
              type="button"
              variant="outline"
              disabled={isValidating}
              onClick={() => void mutate()}
              className="shrink-0 max-md:h-11"
            >
              <RefreshCw data-icon="inline-start" />
              查询结果
            </Button>
          ) : (
            <Button
              type="button"
              variant="outline"
              disabled={
                busy || !data.credential_id || !model || !!activate.pending
              }
              className="shrink-0 max-md:h-11"
              onClick={() =>
                void test.run("credential.test", id, (requestID) =>
                  testProviderOnboardingModel(
                    { id, credential_id: data.credential_id!, model_id: model },
                    { headers: { "Idempotency-Key": requestID } },
                  ),
                )
              }
            >
              {test.busy ? <Spinner data-icon="inline-start" /> : null}
              {verified || selectedCheck?.status === "failed"
                ? "重新验证"
                : "验证模型"}
            </Button>
          )}
        </div>
        <FieldDescription>
          {!data.credential_id
            ? "请先配置 API Key。"
            : test.pending
              ? test.busy
                ? "正在提交验证请求。"
                : "先查询原回执，避免重复调用。"
              : "验证会调用服务商，可能产生费用。"}
        </FieldDescription>
        {verified && !busy && !test.pending ? (
          <FieldDescription>
            {selectedCheck?.model_result === "accepted"
              ? "服务商已受理视频验证任务，视频仍在生成。"
              : "仅所选模型已验证。"}
          </FieldDescription>
        ) : null}
      </Field>
      {selectedCheck?.status === "failed" && !busy && !test.pending ? (
        <Alert variant="destructive">
          <AlertDescription>
            {errors[selectedCheck.auth_result ?? ""] ??
              "结果尚未确认，请先核对供应商记录；重试将再次调用。"}
          </AlertDescription>
        </Alert>
      ) : null}
      {!test.pending &&
      test.message &&
      !successMessages.includes(test.message) ? (
        <Alert variant="destructive">
          <AlertDescription>{test.message}</AlertDescription>
        </Alert>
      ) : null}
      {activate.pending ? (
        <Alert>
          <AlertTitle>启用结果待确认</AlertTitle>
          <AlertDescription>
            <Button
              type="button"
              variant="outline"
              disabled={activate.busy}
              onClick={activate.recover}
            >
              查询回执
            </Button>
          </AlertDescription>
        </Alert>
      ) : activate.message && !successMessages.includes(activate.message) ? (
        <Alert variant="destructive">
          <AlertDescription>{activate.message}</AlertDescription>
        </Alert>
      ) : null}
      {verified && data.status !== "active" ? (
        <Button
          type="button"
          disabled={busy || !!test.pending || !!activate.pending}
          className="self-end max-md:h-11"
          onClick={() =>
            void activate.run("provider.onboarding.activate", id, (requestID) =>
              activateProviderOnboarding(
                { id },
                {
                  request_id: requestID,
                  check_id: check!.id,
                  expected_revision: data.revision,
                },
              ),
            )
          }
        >
          {activate.busy ? <Spinner data-icon="inline-start" /> : null}
          启用 {data.models.length} 个模型
        </Button>
      ) : null}
    </div>
  );
}
