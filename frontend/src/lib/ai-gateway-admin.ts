// eslint-disable-next-line no-restricted-syntax -- 新网关 MVP 经授权使用现有 request；生成目录保持只读。
import { request } from "@/lib/request";

export type GatewayPrice = {
  input: number;
  cache_read: number;
  cache_write: number;
  output: number;
  request: number;
  currency: string;
  fx: string;
  multiplier: string;
  discount: string;
};
export type ParameterType = "string" | "integer" | "number" | "boolean";
export type ParameterValue = string | number | boolean;
export type GatewayParameter = {
  type: ParameterType;
  min?: number;
  max?: number;
  enum?: ParameterValue[];
  default?: ParameterValue;
};
export type MediaDriverOption = {
  key: string;
  name: string;
  base_url: string;
  region: "domestic" | "overseas";
  parameters: Record<string, string[]>;
  custom_url: boolean;
};
export type GatewayVideoMetering = {
  resolution: string;
  frames_per_second: number;
  pixels_per_token: number;
  minimum_tokens?: number;
  geometries: Record<string, { width: number; height: number }>;
};
export type GatewayModel = {
  id: string;
  name: string;
  protocol: string;
  price: GatewayPrice;
  max_input: number;
  max_output: number;
  purchase_cap: number;
  tools: boolean;
  stream: boolean;
  capability?: string;
  modes?: string[];
  parameters?: Record<string, GatewayParameter>;
  output_policy?: string;
  video_metering?: GatewayVideoMetering;
};
export type GatewayRoute = {
  id?: string;
  model_id: string;
  supplier: string;
  credential: string;
  group: string;
  base_url: string;
  upstream_model: string;
  protocol: string;
  purchase: GatewayPrice;
  priority: number;
  driver_config?: Record<string, unknown>;
};
export type CredentialState = "active" | "draining" | "revoked";
export type GatewayConfiguration = {
  models: { model: GatewayModel; enabled: boolean; draft?: boolean }[];
  onboarding: OnboardingSummary[];
  routes: { route: GatewayRoute; enabled: boolean }[];
  credentials: { id: string; supplier_id: string; state: CredentialState }[];
  quotas: { name: string; concurrency: number; rpm: number }[];
  media_drivers: MediaDriverOption[];
  unknown: {
    request_id: string;
    attempt_id: string;
    scope_id: string;
    state: string;
    upstream_id?: string;
  }[];
};
export type OnboardingSummary = {
  id: string;
  model_id: string;
  state:
    "prepared" | "verifying" | "active" | "failed" | "stale" | "rolled_back";
  upstream_model: string;
};

export type OnboardingReview = OnboardingSummary & {
  scope_id: string;
  project_id: string;
  plan: {
    credits: number;
    purchase_micros: number;
    probes: { name: string; input: unknown }[];
  };
  progress: { request_id: string; state: string }[];
};

export const arkEmbedding = {
  protocol: "ark_embedding",
  model: "doubao-embedding-vision-251215",
  baseURL: "https://ark.cn-beijing.volces.com/api/v3",
} as const;

const prefix = "/api/ai/admin";
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
function requireValue(ok: boolean, message: string): asserts ok {
  if (!ok) throw new Error(message);
}
function integer(value: number, min = 0, max = Number.MAX_SAFE_INTEGER) {
  return Number.isSafeInteger(value) && value >= min && value <= max;
}
function id(value: string) {
  requireValue(
    uuid.test(value) && !/^0{8}-0{4}-0{4}-0{4}-0{12}$/.test(value),
    "请填写有效 UUID",
  );
  return value;
}
export function validatePrice(price: GatewayPrice): GatewayPrice {
  const {
    input,
    cache_read,
    cache_write,
    output,
    request: fixed,
    currency,
    fx,
    multiplier,
    discount,
  } = price;
  requireValue(
    [input, cache_read, cache_write, output, fixed].every((n) => integer(n)) &&
      [input, cache_read, cache_write, output, fixed].some((n) => n > 0),
    "价格须为非负安全整数，且至少一项大于零",
  );
  requireValue(
    currency === "CNY" || currency === "USD",
    "币种只支持 CNY / USD",
  );
  requireValue(
    [fx, multiplier, discount].every(
      (n) =>
        typeof n === "string" &&
        /^\d+(?:\.\d+)?$/.test(n) &&
        Number(n) > 0 &&
        Number(n) <= 1000000,
    ) && Number(discount) <= 1,
    "汇率和倍率须为正十进制字符串；折扣不得超过 1",
  );
  return {
    input,
    cache_read,
    cache_write,
    output,
    request: fixed,
    currency,
    fx,
    multiplier,
    discount,
  };
}
export function validateModel(
  value: Omit<GatewayModel, "id"> | GatewayModel,
): Omit<GatewayModel, "id"> {
  requireValue(
    typeof value.name === "string" &&
      Boolean(value.name.trim()) &&
      [
        "chat",
        "responses",
        "messages",
        "media",
        arkEmbedding.protocol,
      ].includes(value.protocol),
    "请填写模型名称与有效协议",
  );
  requireValue(
    integer(value.max_input, 1, 1048576) &&
      integer(value.max_output, 1, 1048576) &&
      integer(value.purchase_cap),
    "输入/输出上限为 1–1048576，采购上限须为非负安全整数",
  );
  requireValue(
    typeof value.tools === "boolean" && typeof value.stream === "boolean",
    "工具与流式选项须为布尔值",
  );
  const price = validatePrice(value.price);
  const video = value.video_metering;
  if (video) {
    requireValue(
      value.protocol === "media" &&
        value.capability === "video.generate" &&
        value.output_policy === "provider_success_v1" &&
        video.resolution.trim().length > 0 &&
        integer(video.frames_per_second, 1) &&
        integer(video.pixels_per_token, 1) &&
        integer(video.minimum_tokens ?? 0, 0, value.max_output) &&
        Object.keys(video.geometries).length > 0 &&
        Object.entries(video.geometries).every(
          ([ratio, size]) =>
            Boolean(ratio.trim()) &&
            ratio !== "adaptive" &&
            integer(size.width, 1) &&
            integer(size.height, 1),
        ),
      "视频 Token 计费需要已审核的固定画幅、尺寸、帧率及估量规则",
    );
  }
  const result: Omit<GatewayModel, "id"> = {
    name: value.name.trim(),
    protocol: value.protocol,
    price,
    max_input: value.max_input,
    max_output: value.max_output,
    purchase_cap: value.purchase_cap,
    tools: value.tools,
    stream: value.stream,
  };
  if (value.protocol === arkEmbedding.protocol) {
    requireValue(
      !value.tools &&
        !value.stream &&
        price.input > 0 &&
        price.output === 0 &&
        price.cache_read === 0 &&
        price.cache_write === 0 &&
        price.request === 0,
      "豆包向量模型按输入 token 计价",
    );
    result.capability = "text.embed";
  }
  if (value.protocol === "media") {
    requireValue(
      ["image.generate", "video.generate", "audio.generate"].includes(
        value.capability ?? "",
      ) &&
        Array.isArray(value.modes) &&
        value.modes.length > 0 &&
        value.modes.every(
          (mode) => typeof mode === "string" && Boolean(mode.trim()),
        ),
      "媒体模型须填写能力和模式",
    );
    requireValue(
      !value.tools &&
        !value.stream &&
        price.input === 0 &&
        price.cache_read === 0 &&
        price.cache_write === 0 &&
        (video
          ? price.request === 0 && price.output > 0
          : price.request > 0 && price.output === 0),
      "请填写与媒体计费方式匹配的正数单价",
    );
    requireValue(
      !value.output_policy || value.output_policy === "provider_success_v1",
      "不支持的输出审核策略",
    );
    requireValue(
      value.parameters !== null &&
        typeof value.parameters === "object" &&
        !Array.isArray(value.parameters),
      "媒体模型缺少参数配置",
    );
    for (const parameter of Object.values(value.parameters ?? {})) {
      requireValue(
        parameter !== null &&
          typeof parameter === "object" &&
          !Array.isArray(parameter),
        "参数定义须为对象",
      );
      const p = parameter as Record<string, unknown>;
      requireValue(
        ["string", "integer", "number", "boolean"].includes(String(p.type)) &&
          (p.min === undefined || typeof p.min === "number") &&
          (p.max === undefined || typeof p.max === "number") &&
          (p.min === undefined ||
            p.max === undefined ||
            Number(p.min) <= Number(p.max)),
        "媒体参数类型或范围无效",
      );
    }
    Object.assign(result, {
      capability: value.capability,
      modes: value.modes,
      parameters: value.parameters,
      ...(value.output_policy ? { output_policy: value.output_policy } : {}),
      ...(video ? { video_metering: video } : {}),
    });
  }
  return result;
}
export async function loadGatewayConfiguration(
  signal?: AbortSignal,
): Promise<GatewayConfiguration> {
  const result = await request<GatewayConfiguration>(
    `${prefix}/configuration`,
    { signal },
  );
  requireValue(
    result !== null && typeof result === "object",
    "网关配置响应无效",
  );
  for (const key of [
    "models",
    "routes",
    "onboarding",
    "credentials",
    "quotas",
    "media_drivers",
    "unknown",
  ] as const) {
    requireValue(
      result[key] == null || Array.isArray(result[key]),
      "网关配置列表无效",
    );
  }
  const readablePrice = (price: GatewayPrice) =>
    price !== null &&
    typeof price === "object" &&
    [
      price.input,
      price.cache_read,
      price.cache_write,
      price.output,
      price.request,
    ].every((n) => integer(n)) &&
    typeof price.currency === "string";
  requireValue(
    (result.models ?? []).every(
      (row) =>
        row?.model &&
        typeof row.enabled === "boolean" &&
        uuid.test(row.model.id) &&
        typeof row.model.name === "string" &&
        typeof row.model.protocol === "string" &&
        readablePrice(row.model.price),
    ),
    "销售模型配置无效",
  );
  requireValue(
    (result.routes ?? []).every(
      (row) =>
        row?.route &&
        typeof row.enabled === "boolean" &&
        typeof row.route.model_id === "string" &&
        typeof row.route.upstream_model === "string" &&
        typeof row.route.group === "string" &&
        readablePrice(row.route.purchase),
    ),
    "供应商路由配置无效",
  );
  requireValue(
    (result.credentials ?? []).every(
      (row) =>
        row &&
        uuid.test(row.id) &&
        uuid.test(row.supplier_id) &&
        ["active", "draining", "revoked"].includes(row.state),
    ),
    "供应商凭据元数据无效",
  );
  requireValue(
    (result.quotas ?? []).every(
      (row) =>
        row &&
        typeof row.name === "string" &&
        integer(row.concurrency, 1, 1000) &&
        integer(row.rpm, 1),
    ),
    "共享配额配置无效",
  );
  requireValue(
    (result.media_drivers ?? []).every(
      (driver) =>
        driver != null &&
        typeof driver.key === "string" &&
        typeof driver.name === "string" &&
        typeof driver.base_url === "string" &&
        (driver.region === "domestic" || driver.region === "overseas") &&
        typeof driver.custom_url === "boolean" &&
        driver.parameters != null &&
        typeof driver.parameters === "object" &&
        !Array.isArray(driver.parameters) &&
        Object.values(driver.parameters).every(
          (fields) =>
            Array.isArray(fields) &&
            fields.every((field) => typeof field === "string"),
        ),
    ),
    "媒体驱动列表无效",
  );
  requireValue(
    (result.unknown ?? []).every(
      (row) =>
        row &&
        uuid.test(row.request_id) &&
        uuid.test(row.attempt_id) &&
        uuid.test(row.scope_id),
    ),
    "待对账记录无效",
  );
  return {
    models: result.models ?? [],
    routes: result.routes ?? [],
    quotas: result.quotas ?? [],
    media_drivers: result.media_drivers ?? [],
    onboarding: result.onboarding ?? [],
    unknown: result.unknown ?? [],
    credentials: (result.credentials ?? []).map(
      ({ id, supplier_id, state }) => ({ id, supplier_id, state }),
    ),
  };
}
export async function prepareModel(
  value: Omit<GatewayModel, "id"> | GatewayModel,
  modelID = crypto.randomUUID(),
) {
  const model = validateModel(value);
  return request<void>(`${prefix}/models`, {
    method: "POST",
    data: { ...model, id: modelID },
  });
}
export async function prepareRoute(
  value: Omit<GatewayRoute, "credential">,
  options: {
    scope: string;
    project?: string;
    replacement?: string;
    mediaTests?: unknown[];
    concurrency: number;
    rpm: number;
  },
) {
  const { scope, project, replacement, mediaTests, concurrency, rpm } = options;
  id(value.model_id);
  id(value.supplier);
  id(scope);
  if (project) id(project);
  if (replacement) id(replacement);
  let endpoint: URL;
  try {
    endpoint = new URL(value.base_url);
  } catch {
    throw new Error("供应商地址须为公开 HTTPS 基础地址");
  }
  requireValue(
    endpoint.protocol === "https:" &&
      !endpoint.username &&
      !endpoint.password &&
      !endpoint.search &&
      !endpoint.hash &&
      endpoint.hostname.includes(".") &&
      !/^(localhost|127\.|0\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(
        endpoint.hostname,
      ) &&
      !endpoint.hostname.endsWith(".local"),
    "供应商地址须为公开 HTTPS 基础地址",
  );
  requireValue(
    Boolean(value.group.trim()) &&
      Boolean(value.upstream_model.trim()) &&
      [
        "chat",
        "responses",
        "messages",
        "media",
        arkEmbedding.protocol,
      ].includes(value.protocol),
    "请填写配额组、供应商模型号与协议",
  );
  requireValue(
    integer(value.priority) && integer(concurrency, 1, 1000) && integer(rpm, 1),
    "优先级须非负，并发为 1–1000，RPM 须为正整数",
  );
  if (value.protocol === arkEmbedding.protocol) {
    requireValue(
      value.base_url === arkEmbedding.baseURL &&
        value.upstream_model === arkEmbedding.model &&
        value.purchase.input > 0 &&
        value.purchase.output === 0 &&
        value.purchase.cache_write === 0 &&
        value.purchase.cache_read === 0 &&
        value.purchase.request === 0,
      "请使用方舟文本向量路由与输入单价",
    );
  }
  if (value.driver_config) {
    requireValue(
      value.protocol === "media" &&
        typeof value.driver_config === "object" &&
        !Array.isArray(value.driver_config),
      "驱动配置仅支持媒体协议的 JSON 对象",
    );
    // ProviderConfig contains only these seven nonsecret fields. adapter_key
    // identifies a driver; reject unknown fields rather than guessing by suffix.
    const config = value.driver_config;
    const fields = [
      "adapter_key",
      "base_url",
      "region",
      "capabilities",
      "timeout_seconds",
      "concurrency_limit",
      "rate_limit_per_min",
    ];
    requireValue(
      Object.keys(config).every((field) => fields.includes(field)),
      "驱动配置不得包含密钥或额外字段；请使用供应商密钥专用输入框",
    );
    requireValue(
      typeof config.adapter_key === "string" &&
        /^[a-z][a-z0-9_]*$/.test(config.adapter_key) &&
        config.base_url === value.base_url &&
        (config.region === "domestic" || config.region === "overseas") &&
        Array.isArray(config.capabilities) &&
        config.capabilities.length > 0 &&
        config.capabilities.every(
          (capability) =>
            typeof capability === "string" && Boolean(capability.trim()),
        ) &&
        new Set(config.capabilities).size === config.capabilities.length &&
        typeof config.timeout_seconds === "number" &&
        integer(config.timeout_seconds, 5, 300) &&
        typeof config.concurrency_limit === "number" &&
        integer(config.concurrency_limit, 1, 1000) &&
        typeof config.rate_limit_per_min === "number" &&
        integer(config.rate_limit_per_min, 1, 100000),
      "媒体驱动配置字段类型或范围无效；地址须与路由一致",
    );
  }
  const {
    model_id,
    supplier,
    group,
    base_url,
    upstream_model,
    protocol,
    priority,
    driver_config,
  } = value;
  const route = {
    id: value.id ?? crypto.randomUUID(),
    model_id,
    supplier,
    group: group.trim(),
    base_url,
    upstream_model: upstream_model.trim(),
    protocol,
    purchase: validatePrice(value.purchase),
    priority,
    ...(driver_config ? { driver_config } : {}),
  };
  return request<OnboardingReview>(`${prefix}/routes`, {
    method: "POST",
    data: {
      route,
      scope_id: scope,
      ...(project ? { project_id: project } : {}),
      ...(replacement ? { replaces_route_id: replacement } : {}),
      ...(mediaTests ? { media_tests: mediaTests } : {}),
      concurrency,
      rpm,
    },
  });
}

export function loadOnboarding(candidate: string, signal?: AbortSignal) {
  id(candidate);
  return request<OnboardingReview>(`${prefix}/onboarding/${candidate}`, {
    signal,
  });
}

export function verifyOnboarding(review: OnboardingReview, key: string) {
  id(review.id);
  return request<OnboardingReview>(`${prefix}/onboarding/${review.id}/verify`, {
    method: "POST",
    data: {
      key,
      credits: review.plan.credits,
      purchase_micros: review.plan.purchase_micros,
    },
  });
}

export function rollbackOnboarding(candidate: string) {
  id(candidate);
  return request<void>(`${prefix}/onboarding/${candidate}/rollback`, {
    method: "POST",
  });
}
export async function setCredentialState(
  credential: string,
  state: CredentialState,
) {
  id(credential);
  requireValue(
    ["active", "draining", "revoked"].includes(state),
    "凭据状态无效",
  );
  return request<void>(`${prefix}/credentials/${credential}`, {
    method: "PUT",
    data: { state },
  });
}
export async function fundCredits(
  scope: string,
  reference: string,
  credits: number,
) {
  id(scope);
  requireValue(
    Boolean(reference.trim()) && integer(credits, 1, 1000000000),
    "请填写唯一业务凭据，积分为 1–1000000000 的整数",
  );
  return request<void>(`${prefix}/credits`, {
    method: "POST",
    data: { scope_id: scope, key: reference.trim(), credits },
  });
}
export async function reconcileGateway(value: {
  request_id: string;
  attempt_id: string;
  reference: string;
  reason: string;
  not_charged: boolean;
  usage?: {
    input: number;
    cache_read: number;
    cache_write: number;
    output: number;
  };
}) {
  id(value.request_id);
  id(value.attempt_id);
  requireValue(
    Boolean(value.reference.trim()) && Boolean(value.reason.trim()),
    "必须填写原始供应商账单凭据与核对原因",
  );
  requireValue(
    value.not_charged ? !value.usage : Boolean(value.usage),
    "未收费与实际用量互斥",
  );
  if (value.usage) {
    const u = value.usage;
    requireValue(
      Object.values(u).every((n) => integer(n, 0, 2 ** 40)) &&
        u.cache_read + u.cache_write <= u.input,
      "用量须为有效整数，缓存用量不能超过输入用量",
    );
  }
  return request(`${prefix}/reconciliations`, { method: "POST", data: value });
}
