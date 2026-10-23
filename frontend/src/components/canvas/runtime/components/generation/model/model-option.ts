type ModelOption = {
  connectionId?: number | string;
  modelId?: string;
  modelName?: string;
  channelName?: string;
  providerName?: string;
  providerModelId?: string;
};

/** 同名型号跨渠道时优先使用管理员命名的渠道区分，不向创作者暴露内部接入编号。 */
export function modelOptionLabel(
  model: ModelOption,
  peers: ModelOption[] = [],
) {
  const name = model.modelName?.trim() || "未命名模型";
  const provider = providerLabel(model);
  const label = !provider || provider === name ? name : `${provider} · ${name}`;
  const sameLabel = peers.filter((peer) => baseLabel(peer) === label);
  if (sameLabel.length < 2) return label;
  const channel = model.channelName?.trim();
  return channel && channel !== name ? `${label}（${channel}）` : label;
}

function baseLabel(model: ModelOption) {
  const name = model.modelName?.trim() || "未命名模型";
  const provider = providerLabel(model);
  return !provider || provider === name ? name : `${provider} · ${name}`;
}

export function modelOptionKey(model: {
  connectionId?: number | string;
  modelId?: string;
}) {
  return `${model.connectionId}:${model.modelId}`;
}

function providerLabel(model: ModelOption) {
  const explicit = model.providerName?.trim();
  if (explicit) return explicit;
  const identity =
    `${model.providerModelId ?? ""} ${model.modelName ?? ""}`.toLowerCase();
  if (
    identity.includes("seedream") ||
    identity.includes("seedance") ||
    identity.includes("doubao")
  )
    return "火山引擎";
  if (identity.includes("deepseek")) return "DeepSeek";
  return "";
}
