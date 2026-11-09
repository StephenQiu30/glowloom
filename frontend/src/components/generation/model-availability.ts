/** Consume the server projection; missing or stale legacy metadata fails closed. */
export function modelAvailableForMode(
  model: API.ModelResponse,
  mode?: string | null,
) {
  return (
    model.available === true &&
    (!mode ||
      model.mode_availability?.some(
        (item) => item.mode === mode && item.available === true,
      ) === true)
  );
}

export function availableModelModes(model?: API.ModelResponse) {
  if (model?.available !== true) return [];
  return (
    model?.mode_availability
      ?.filter((item) => item.available === true && item.mode)
      .map((item) => item.mode!) ?? []
  );
}

export function unavailableReasonLabel(reason: string) {
  const labels: Record<string, string> = {
    disabled: "连接或模型已停用，或供应商已紧急停用",
    credential_required: "请配置有效凭据",
    release_required: "请完成模型发布",
    unsupported_contract: "协议、参数或计量合同不支持",
    price_unverified: "采购价或销售价未核验，或不在有效范围",
    mode_unverified: "尚未完成对应模式的真实验证",
    version_changed: "连接配置或模型版本已变更，请重新验证并发布",
    no_available_model: "当前连接没有可用模型",
  };
  return labels[reason] ?? reason;
}
