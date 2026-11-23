export function payerLabel(payer?: string) {
  if (payer === "team") return "团队付款账户";
  if (payer === "personal") return "个人付款账户";
  return "当前空间付款账户";
}

const movementLabels: Record<string, string> = {
  grant: "入账",
  issue: "入账",
  freeze: "冻结",
  hold: "冻结",
  settle: "实扣",
  redeem: "实扣",
  release: "解冻",
  expire: "到期",
  restore: "到期返还",
  compensate: "消费返还",
};

const purposes: Record<string, string> = {
  promotional: "赠送积分",
  paid: "购买积分",
  subscription: "会员积分",
  expiry_return: "返还积分",
  "image.generate": "图片生成",
  "video.generate": "视频生成",
  "audio.generate": "音频生成",
  "subject.voice": "主体配音",
  transcription: "语音转写",
  creation: "工作台创作",
  workbench: "工作台创作",
  canvas: "画布创作",
  sdk: "API 调用",
  agent: "创作助手",
};

const dateFormat = new Intl.DateTimeFormat("zh-CN", {
  timeZone: "Asia/Shanghai",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});

function movementAmount(kind: string, credits?: string) {
  if (!credits || !/^[+-]?\d+(\.\d+)?$/.test(credits)) return "—";
  const value = credits.replace(/^[+-]/, "");
  if (/^0+(\.0+)?$/.test(value)) return "0";
  switch (kind) {
    case "hold":
    case "freeze":
      return "冻结 " + value;
    case "release":
      return "返还 " + value;
    case "redeem":
    case "settle":
    case "expire":
      return "−" + value;
    case "grant":
    case "issue":
    case "restore":
    case "compensate":
      return "+" + value;
    default:
      return credits;
  }
}

export function presentMovement(entry: API.WalletEntry) {
  const kind = entry.kind ?? "";
  const purpose =
    purposes[entry.capability ?? ""] ?? purposes[entry.source ?? ""];
  const model = entry.model_key?.trim();
  const displayModel =
    model &&
    !/(?:candidate-|[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12})/i.test(model)
      ? model
      : undefined;
  const date = entry.created_at ? new Date(entry.created_at) : undefined;
  const amount = movementAmount(kind, entry.credits);
  return {
    label: movementLabels[kind] ?? "积分变动",
    purpose:
      [purpose, displayModel].filter(Boolean).join(" · ") ||
      (kind === "grant" ? "积分发放" : "账户变动"),
    amount: amount === "—" ? amount : amount + " 积分",
    time:
      date && Number.isFinite(date.getTime())
        ? dateFormat.format(date).replaceAll("/", "-")
        : undefined,
  };
}
