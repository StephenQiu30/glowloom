import {
  activateModelRelease,
  createModelRelease,
  getAdminModel,
  getAdminProvider,
  getProviderCheck,
  getProviderConfig,
  listCapabilityDefaults,
  listModelReleases,
  listProviderChecks,
  setAdminModelStatus,
  testAdminCredential,
} from "@/api/settings";
import { RequestError } from "@/lib/request";

type RunMutation = <T>(
  action: string,
  target: string,
  execute: (requestID: string) => Promise<T>,
) => Promise<T | undefined>;

export function savedModelBinding(detail: API.AdminModelDetail) {
  const sale = detail.prices?.find(
    (price) => typeof price.rule?.procurement_price_version_id === "string",
  );
  return {
    model_version_id:
      detail.model?.current_version_id ?? detail.versions?.[0]?.id,
    price_version_id:
      sale?.id ??
      (!detail.candidate?.model ? detail.prices?.[0]?.id : undefined),
  };
}

export function releaseMatchesSaved(
  release: API.ModelRelease,
  detail: API.AdminModelDetail,
  configID: string | undefined,
) {
  const saved = savedModelBinding(detail);
  return (
    release.model_version_id === saved.model_version_id &&
    release.price_version_id === saved.price_version_id &&
    release.config_version_id === configID
  );
}

function verifiedCheck(
  check: API.ProviderCheck,
  configID: string,
  credentialID: string,
) {
  return (
    check.current === true &&
    check.kind === "credential_auth" &&
    check.status === "completed" &&
    check.auth_result === "ok" &&
    check.adapter_result === "ok" &&
    check.config_version_id === configID &&
    check.credential_id === credentialID
  );
}

/** Activates a pinned binding; resuming never selects a newer saved draft. */
export async function changeModelActivation(
  id: string,
  intent: "enable" | "disable" | "update",
  run: RunMutation,
  signal: AbortSignal,
) {
  const detail = await getAdminModel({ id }, { signal });
  const model = detail.model;
  if (!model?.revision || !model.provider_id || model.status === "archived")
    throw new Error("模型已变化，请刷新后重试。");
  if (intent === "disable") {
    if (model.status !== "active") return;
    return run("model.status", id, (requestID) =>
      setAdminModelStatus(
        { id },
        { status: "disabled", expected_revision: model.revision },
        { signal, headers: { "Idempotency-Key": requestID } },
      ),
    );
  }
  if (intent === "enable" && model.status === "active") return;
  const [releases, defaults, connection, config] = await Promise.all([
    listModelReleases({ id }, { signal }),
    listCapabilityDefaults({ signal }),
    getAdminProvider({ id: model.provider_id }, { signal }),
    getProviderConfig({ id: model.provider_id }, { signal }),
  ]);
  const active = releases.items?.find((release) => release.status === "active");
  const binding =
    intent === "enable" && active
      ? active
      : {
          ...savedModelBinding(detail),
          config_version_id: config.id,
        };
  if (!binding.model_version_id)
    throw new Error("请先在「模型设置」保存配置。");
  if (!binding.price_version_id)
    throw new Error("请先在「价格」保存有效价格。");
  if (!config.id || connection.provider?.status === "archived")
    throw new Error("请先恢复服务连接并保存连接配置。");
  if (binding.config_version_id !== config.id)
    throw new Error("连接配置已变更，请在模型详情核对并应用更新。");
  const credential = connection.credential;
  if (!credential?.id || credential.status !== "active")
    throw new Error("请先在 AI 服务中保存有效 Key。");
  const checks = await listProviderChecks(
    { id: model.provider_id },
    { signal },
  );
  let check = checks.current_checks?.find((item) =>
    verifiedCheck(item, config.id!, credential.id!),
  );
  if (!check?.id) {
    const accepted = await run(
      "credential.test",
      model.provider_id,
      async (requestID) => {
        const result = await testAdminCredential(
          { id: model.provider_id!, credential_id: credential.id! },
          { signal, headers: { "Idempotency-Key": requestID } },
        );
        if (!result.accepted || !result.check_id)
          throw new RequestError("request_unconfirmed");
        return result;
      },
    );
    if (!accepted?.check_id) return;
    for (let attempt = 0; attempt < 30; attempt++) {
      signal.throwIfAborted();
      check = await getProviderCheck({ id: accepted.check_id }, { signal });
      if (check.status !== "pending" && check.status !== "running") break;
      await new Promise<void>((resolve, reject) => {
        const abort = () => {
          clearTimeout(timer);
          reject(signal.reason);
        };
        const timer = setTimeout(() => {
          signal.removeEventListener("abort", abort);
          resolve();
        }, 1000);
        signal.addEventListener("abort", abort, { once: true });
      });
    }
    if (!check || !verifiedCheck(check, config.id, credential.id))
      throw new Error("Key 或服务验证未通过，请在 AI 服务中核对后重试。");
  }
  signal.throwIfAborted();
  const existing =
    intent === "enable" && active
      ? active
      : releases.items?.find(
          (release) =>
            release.status === "draft" &&
            releaseMatchesSaved(release, detail, config.id),
        );
  const release =
    existing ??
    (await run("release.create", id, (requestID) =>
      createModelRelease(
        { id },
        {
          request_id: requestID,
          model_version_id: binding.model_version_id,
          price_version_id: binding.price_version_id,
          config_version_id: binding.config_version_id,
        },
        { signal },
      ),
    ));
  if (!release?.id) return;
  const currentDefault = defaults.items?.find(
    (item) => item.capability === model.capability,
  );
  return run("release.activate", release.id, (requestID) =>
    activateModelRelease(
      { id: release.id! },
      {
        request_id: requestID,
        credential_check_id: check!.id,
        expected_revision: release.revision,
        expected_model_revision: model.revision,
        expected_active_release_id: active?.id,
        enable_provider: connection.provider?.status === "disabled",
        set_default:
          !!active?.id &&
          currentDefault?.release_id === active.id &&
          active.id !== release.id,
        expected_default_revision: currentDefault?.revision ?? 0,
      },
      { signal },
    ),
  );
}
