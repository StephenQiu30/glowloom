"use client";

import { useRef, useState } from "react";
import { checkModelConfiguration } from "@/api/settings";
import { catalogMessage, MutationStatus } from "@/components/catalog/mutation";
import { useProviderCatalogProbe } from "@/components/catalog/provider-catalog-probe";
import { Button } from "@/components/ui/button";

export function ModelConfigurationCheck({
  id,
  provider,
  version,
  config,
}: {
  id: string;
  provider: string;
  version: string;
  config: string;
}) {
  const [checking, setChecking] = useState(false);
  const [result, setResult] = useState<API.ModelConfigurationCheck>();
  const [error, setError] = useState("");
  const lock = useRef(false);
  const probe = useProviderCatalogProbe(
    provider,
    `model.configuration-check.${id}`,
  );
  const check = probe.data;
  const current =
    check?.current &&
    check.config_version_id === result?.config_version_id &&
    check.credential_id === result?.credential_id;
  const message = !result
    ? ""
    : !result.parameters_valid
      ? "保存的模型参数或连接配置未通过校验。"
      : !result.credential_id
        ? "参数通过 · 尚未配置 API Key"
        : checking || probe.running
          ? "参数通过 · 正在检查凭据与目录…"
          : check && !current
            ? "参数通过 · 连接已变化，请重新验证"
            : check?.auth_result === "auth_failed"
              ? "参数通过 · 供应商拒绝了凭据"
              : check?.discovery_result === "ok"
                ? check.candidates?.some(
                    (candidate) =>
                      candidate.provider_model_id === result.provider_model_id,
                  )
                  ? "参数通过 · 目录可见"
                  : "参数通过 · 目录未返回此模型"
                : "参数通过 · 模型权限未确认";
  async function verify() {
    if (lock.current) return;
    lock.current = true;
    setChecking(true);
    setResult(undefined);
    setError("");
    try {
      const saved = await checkModelConfiguration({ id });
      if (
        saved.model_version_id !== version ||
        saved.config_version_id !== config
      ) {
        setError("模型或连接配置已变化，请重新打开详情后验证。");
        return;
      }
      setResult(saved);
      if (saved.parameters_valid && saved.credential_id)
        await probe.run(saved.credential_id);
    } catch (failure) {
      setError(catalogMessage(failure));
    } finally {
      lock.current = false;
      setChecking(false);
    }
  }
  return (
    <div className="flex flex-col items-start gap-2">
      <Button
        type="button"
        variant="outline"
        disabled={
          checking ||
          probe.running ||
          probe.mutation.busy ||
          !!probe.mutation.pending
        }
        onClick={() => void verify()}
      >
        {checking || probe.running ? "正在验证配置…" : "验证配置"}
      </Button>
      {error || probe.error || message ? (
        <p className="text-sm text-muted-foreground" role="status">
          {error || (probe.error ? catalogMessage(probe.error) : message)}
        </p>
      ) : null}
      {!probe.mutation.busy && probe.mutation.pending ? (
        <MutationStatus mutation={probe.mutation} inline />
      ) : null}
      {!probe.mutation.pending &&
      probe.mutation.message &&
      !probe.data &&
      !probe.running &&
      probe.mutation.message !== "已保存。" ? (
        <p className="text-sm text-muted-foreground">
          {probe.mutation.message}
        </p>
      ) : null}
    </div>
  );
}
