import { getCSRFToken, type RequestOptions } from "@/lib/request";
import { getSession } from "@/api/identity";
import { listProjectModels } from "@/api/models";
import { getMediaPreview } from "@/api/media";
import { isProjectID } from "@/components/projects/project";

type Result<T> = {
  code?: number;
  data?: T;
  message?: string;
  requestId?: string;
};
export class ApiError extends Error {
  constructor(
    message: string,
    public readonly code: number,
    public readonly requestId?: string,
    public readonly retryAfterMs?: number,
    public readonly retrySafe = false,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
// The application session owns CSRF. Imported editor modules cannot replace it.
export function setCsrfToken(_value?: string) {
  if (_value !== undefined && _value !== getCSRFToken())
    throw new ApiError("会话身份已变化，请重新打开画布", 40100);
}
export function browserConnectionCsrf() {
  return getCSRFToken() ?? undefined;
}
function projectID(options: RequestOptions) {
  const explicit = options.params?.project_id ?? options.params?.projectId;
  const value =
    typeof explicit === "string"
      ? explicit
      : typeof window !== "undefined"
        ? new URLSearchParams(window.location.search).get("project_id")
        : null;
  if (!isProjectID(value)) throw new ApiError("缺少画布项目上下文", 40000);
  return value;
}
function modelView(model: API.ModelResponse): CanvasContract.ModelExecutionVO {
  return {
    connectionId: model.id,
    modelId: model.current_version?.id ?? model.id,
    modelName: model.display_name,
    channelName: model.provider_name,
    modality: model.capability?.split(".")[0],
    providerModelId: model.key,
  };
}
/** Editor host operations use the generated Go clients and current session/scope. */
export async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  const path = url.replace(/^\/api(?=\/)/, "");
  let data: unknown;
  if (path === "/account/session") {
    const session = await getSession(options);
    data = {
      account: {
        id: session.actor.id,
        userName: session.actor.display_name,
        userEmail: session.actor.email,
        userRole: session.actor.platform_role,
        revision: session.actor.revision,
      },
      csrfToken: session.csrf_token,
    };
  } else if (/^\/generation\/models\/(image|text|video)$/.test(path)) {
    const capability = path.split("/").at(-1)!;
    const page = await listProjectModels(
      {
        project_id: projectID(options),
        capability: `${capability}.generate`,
        limit: 200,
      },
      options,
    );
    data = (page.items ?? []).map(modelView);
  } else if (path === "/generation/options") {
    const capability = String(options.params?.modality ?? "image");
    const page = await listProjectModels(
      {
        project_id: projectID(options),
        capability: `${capability}.generate`,
        limit: 200,
      },
      options,
    );
    const fields = new Map<string, CanvasContract.Parameter>();
    for (const model of page.items ?? [])
      for (const field of model.current_version?.param_schema ?? []) {
        const key =
          typeof field.key === "string"
            ? field.key
            : typeof field.name === "string"
              ? field.name
              : undefined;
        if (!key || fields.has(key)) continue;
        fields.set(key, {
          key,
          label: typeof field.label === "string" ? field.label : key,
          numeric: field.type === "integer" || field.type === "number",
          values: Array.isArray(field.enum)
            ? field.enum.map(String)
            : undefined,
          defaultValue:
            field.default == null ? undefined : String(field.default),
        });
      }
    data = { parameters: [...fields.values()] };
  } else if (
    path === "/files/transfer/get" ||
    path === "/files/transfer/read"
  ) {
    const id = String(options.params?.fileId ?? "");
    const preview = await getMediaPreview(
      { pid: projectID(options), asset_id: id },
      options,
    );
    data = {
      ...preview.asset,
      id,
      fileId: id,
      version: String(preview.asset?.revision),
      status: "READY",
      url: preview.url,
    };
  } else {
    // Never send an imported endpoint to a different business service or report fake success.
    throw new ApiError("此画布能力尚未完成服务接入，当前修改已保留", 50100);
  }
  return { code: 20000, data } as T;
}
export function unwrap<T>(result: Result<T>): T {
  if (!result || result.code !== 20000 || result.data == null)
    throw new ApiError(
      result?.message ?? "服务返回无效数据",
      result?.code ?? 50300,
      result?.requestId,
    );
  return result.data;
}
