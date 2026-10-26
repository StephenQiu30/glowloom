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
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Spinner } from "@/components/ui/spinner";
import {
  catalogMessage,
  MutationStatus,
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
  onCancel,
}: {
  id: string;
  onSaved: (id: string) => void;
  onCancel?: () => void;
}) {
  const fieldID = useId();
  const session = useSession();
  const [requestedCheck, setRequestedCheck] = useState<{
    id: string;
    credentialID: string;
  } | null>(null);
  const [selected, setSelected] = useState("");
  const { data, error, isLoading, isValidating, mutate } = useSWR(
    ["provider-onboarding", session.actor.id, id],
    () => getProviderOnboarding({ id }),
    {
      refreshInterval: (state) =>
        ["pending", "running"].includes(state?.check?.status ?? "") ? 2500 : 0,
      revalidateOnFocus: true,
      onSuccess: (state) => {
        if (state.check?.id) setRequestedCheck(null);
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
    selected ||
    check?.model_id ||
    data?.models?.find((m) => m.capability === "text.generate")?.id ||
    data?.models?.[0]?.id ||
    "";
  const running = ["pending", "running"].includes(check?.status ?? "");
  const verified =
    check?.status === "completed" &&
    check.auth_result === "ok" &&
    ["completed", "accepted"].includes(check.model_result ?? "");
  const busy =
    test.busy ||
    activate.busy ||
    running ||
    (!!requestedCheck &&
      requestedCheck.credentialID === data?.credential_id &&
      data?.check?.id !== requestedCheck.id);
  if (isLoading)
    return (
      <p role="status" className="flex items-center gap-2 text-sm">
        <Spinner />
        正在读取接入进度…
      </p>
    );
  if (error)
    return (
      <Alert variant="destructive">
        <AlertTitle>无法读取接入进度</AlertTitle>
        <AlertDescription>
          {catalogMessage(error)}
          <Button type="button" variant="outline" onClick={() => void mutate()}>
            重试
          </Button>
        </AlertDescription>
      </Alert>
    );
  if (!data?.models?.length || data.status === "active") return null;
  const errors: Record<string, string> = {
    auth_failed: "API Key 无效或没有该模型的调用权限，请检查 Key 和模型授权。",
    insufficient_balance: "服务商账户余额不足，请充值后再验证。",
    rate_limited: "服务商限流，请稍后重新验证。",
    not_found: "服务商未找到该模型，请确认账号所在区域与模型版本。",
    invalid_parameters: "服务商拒绝了验证参数，请检查该模型的接入配置。",
  };
  return (
    <div className="flex min-h-0 flex-col gap-5 overflow-y-auto">
      <div className="flex flex-col gap-2">
        <p className="text-sm font-medium">验证一个模型，完成接入</p>
        <p className="text-sm text-muted-foreground">
          已保存 {data.models.length} 个模型。验证通过后，可一起启用并用于创作。
        </p>
      </div>
      <Field>
        <FieldLabel htmlFor={`${fieldID}-model`}>用于验证的模型</FieldLabel>
        <NativeSelect
          id={`${fieldID}-model`}
          value={model}
          disabled={busy || !!test.pending || !!activate.pending || verified}
          onChange={(e) => setSelected(e.target.value)}
          className="w-full max-md:h-11"
        >
          {data.models.map((m) => (
            <NativeSelectOption key={m.id} value={m.id}>
              {m.display_name}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <FieldDescription className="break-all">
          {data.models.find((m) => m.id === model)?.provider_model_id}
        </FieldDescription>
      </Field>
      {!data.credential_id ? (
        <Alert>
          <AlertTitle>请先保存 API Key</AlertTitle>
          <AlertDescription>
            在连接凭据中填写 Key 后，即可继续验证。
          </AlertDescription>
        </Alert>
      ) : null}
      {running ? (
        <Alert>
          <Spinner />
          <AlertTitle>正在验证模型…</AlertTitle>
          <AlertDescription>
            请求已提交，可稍后从此连接继续查看结果。
          </AlertDescription>
        </Alert>
      ) : null}
      {verified ? (
        <Alert>
          <Check />
          <AlertTitle>接入验证通过</AlertTitle>
          <AlertDescription>
            {check?.model_result === "accepted"
              ? "服务商已受理视频验证任务，视频仍在生成。"
              : "所选验证模型已返回有效结果。"}
            其他所选模型尚未单独测试，实际可用性以服务商授权为准。
          </AlertDescription>
        </Alert>
      ) : null}
      {check?.status === "failed" ? (
        <Alert variant="destructive">
          <AlertTitle>验证未通过</AlertTitle>
          <AlertDescription>
            {errors[check.auth_result ?? ""] ??
              "尚未确认调用结果，请先在服务商控制台核对。再次验证会发起一次新的真实调用。"}
          </AlertDescription>
        </Alert>
      ) : null}
      <MutationStatus inline mutation={test} />
      <MutationStatus inline mutation={activate} />
      <p className="text-xs text-muted-foreground">
        验证会发起一次真实调用，可能产生服务商费用。平台暂不额外收费，后续创作由服务商按账号实际用量计费。
      </p>
      <div className="flex flex-wrap items-center justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          disabled={isValidating}
          onClick={() => void mutate()}
          className="mr-auto max-md:h-11"
        >
          <RefreshCw data-icon="inline-start" />
          查询结果
        </Button>
        {onCancel ? (
          <Button type="button" variant="ghost" onClick={onCancel}>
            稍后继续
          </Button>
        ) : null}
        {verified ? (
          <Button
            type="button"
            disabled={busy || !!activate.pending}
            className="max-md:h-11"
            onClick={() =>
              void activate.run(
                "provider.onboarding.activate",
                id,
                (requestID) =>
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
            {activate.busy ? <Spinner data-icon="inline-start" /> : null}启用{" "}
            {data.models.length} 个模型
          </Button>
        ) : (
          <Button
            type="button"
            disabled={busy || !data.credential_id || !model || !!test.pending}
            className="max-md:h-11"
            onClick={() =>
              void test.run("credential.test", id, (requestID) =>
                testProviderOnboardingModel(
                  { id, credential_id: data.credential_id!, model_id: model },
                  { headers: { "Idempotency-Key": requestID } },
                ),
              )
            }
          >
            {busy ? <Spinner data-icon="inline-start" /> : null}
            {running
              ? "验证中…"
              : check?.status === "failed"
                ? "重新验证（新调用）"
                : "验证模型"}
          </Button>
        )}
      </div>
    </div>
  );
}
