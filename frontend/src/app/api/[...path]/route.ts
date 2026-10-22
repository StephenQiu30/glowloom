const identityUUID =
  "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
// Explicit current identity contracts. Adding a handler requires a reviewed entry here.
const identityRoutes: {
  pattern: string;
  methods: string[];
  query: string[];
}[] = [
  { pattern: `sessions`, methods: ["POST"], query: [] },
  { pattern: `session/(refresh|upgrade)`, methods: ["POST"], query: [] },
  { pattern: `session`, methods: ["GET"], query: [] },
  { pattern: `auth-policy`, methods: ["GET"], query: [] },
  { pattern: `registrations`, methods: ["POST"], query: [] },
  {
    pattern: `registrations/${identityUUID}/(resend|verify|complete)`,
    methods: ["POST"],
    query: [],
  },
  { pattern: `password-resets`, methods: ["POST"], query: [] },
  { pattern: `password-resets/complete`, methods: ["POST"], query: [] },
  { pattern: `me`, methods: ["GET", "PATCH"], query: [] },
  { pattern: `me/avatar`, methods: ["GET"], query: [] },
  { pattern: `me/avatar/uploads`, methods: ["POST"], query: [] },
  { pattern: `me/email-change`, methods: ["POST"], query: [] },
  {
    pattern: `me/email-change/${identityUUID}`,
    methods: ["DELETE"],
    query: [],
  },
  {
    pattern: `me/email-change/${identityUUID}/(verify|complete)`,
    methods: ["POST"],
    query: [],
  },
  { pattern: `me/preferences`, methods: ["GET", "PATCH"], query: [] },
  { pattern: `me/(reauthentication|password)`, methods: ["POST"], query: [] },
  { pattern: `me/sessions`, methods: ["GET"], query: [] },
  { pattern: `me/sessions/revoke-others`, methods: ["POST"], query: [] },
  { pattern: `sessions/${identityUUID}`, methods: ["DELETE"], query: [] },
  {
    pattern: `me/deletion(/(preview|challenge))?`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `me/security-events`,
    methods: ["GET"],
    query: ["cursor", "limit", "action", "from", "to"],
  },
  {
    pattern: `platform/user-statistics`,
    methods: ["GET"],
    query: [
      "search",
      "status",
      "platform_role",
      "verified",
      "in_team",
      "order",
      "from",
      "to",
    ],
  },
  { pattern: `platform/user-exports`, methods: ["POST"], query: [] },
  {
    pattern: `platform/user-exports/${identityUUID}(/download)?`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `teams/${identityUUID}/member-statistics`,
    methods: ["GET"],
    query: ["search", "status", "role", "from", "to"],
  },
  {
    pattern: `teams/${identityUUID}/member-exports`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `teams/${identityUUID}/member-exports/${identityUUID}(/download)?`,
    methods: ["GET"],
    query: [],
  },
  { pattern: `me/spaces`, methods: ["GET"], query: [] },
  {
    pattern: `me/authority-proposals`,
    methods: ["GET"],
    query: ["cursor", "limit", "status"],
  },
  {
    pattern: `me/authority-proposals/${identityUUID}/(accept|decline)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `platform/users`,
    methods: ["GET"],
    query: [
      "limit",
      "cursor",
      "search",
      "status",
      "platform_role",
      "order",
      "verified",
      "in_team",
      "from",
      "to",
    ],
  },
  { pattern: `platform/users/${identityUUID}`, methods: ["GET"], query: [] },
  {
    pattern: `platform/users/${identityUUID}/(disable|enable|unlock|revoke-admin|revoke-sessions|profile-reset|password-reset)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `platform/teams`,
    methods: ["GET"],
    query: [
      "cursor",
      "limit",
      "id",
      "search",
      "status",
      "administrator_id",
      "created_from",
      "created_to",
      "governance_blocked",
    ],
  },
  {
    pattern: `platform/teams/${identityUUID}`,
    methods: ["GET", "PATCH"],
    query: [],
  },
  {
    pattern: `platform/teams/${identityUUID}/avatar`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `platform/teams/${identityUUID}/(suspend|restore)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `platform/teams/${identityUUID}/invitations/${identityUUID}/revoke`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `platform/teams/${identityUUID}/invitations`,
    methods: ["GET", "POST"],
    query: ["cursor", "limit", "status"],
  },
  { pattern: `teams/${identityUUID}/avatar`, methods: ["GET"], query: [] },
  {
    pattern: `teams/${identityUUID}/members/${identityUUID}/avatar`,
    methods: ["GET"],
    query: [],
  },
  { pattern: `platform/settings`, methods: ["GET", "PATCH"], query: [] },
  {
    pattern: `platform/admin-grants`,
    methods: ["GET", "POST"],
    query: ["cursor", "limit", "status"],
  },
  {
    pattern: `platform/admin-grants/${identityUUID}/cancel`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `platform/teams/${identityUUID}/admin-transfers`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `platform/audit-events`,
    methods: ["GET"],
    query: [
      "cursor",
      "limit",
      "action",
      "from",
      "to",
      "actor_id",
      "target_user_id",
      "team_id",
    ],
  },
  {
    pattern: `platform/account-invitations`,
    methods: ["GET", "POST"],
    query: ["cursor", "limit", "status"],
  },
  {
    pattern: `platform/account-invitations/${identityUUID}/(resend|revoke)`,
    methods: ["POST"],
    query: [],
  },
  { pattern: `teams`, methods: ["POST"], query: [] },
  { pattern: `teams/${identityUUID}`, methods: ["GET", "PATCH"], query: [] },
  {
    pattern: `teams/${identityUUID}/members`,
    methods: ["GET"],
    query: ["limit", "cursor", "search", "role", "status", "from", "to"],
  },
  {
    pattern: `teams/${identityUUID}/members/${identityUUID}`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `teams/${identityUUID}/members/${identityUUID}/role`,
    methods: ["PATCH"],
    query: [],
  },
  {
    pattern: `teams/${identityUUID}/members/${identityUUID}/(suspend|restore|remove)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `teams/${identityUUID}/(leave|archive|restore)`,
    methods: ["POST"],
    query: [],
  },
  { pattern: `teams/${identityUUID}/settings`, methods: ["PATCH"], query: [] },
  {
    pattern: `teams/${identityUUID}/admin-transfers`,
    methods: ["GET", "POST"],
    query: ["cursor", "limit", "status"],
  },
  {
    pattern: `teams/${identityUUID}/admin-transfers/${identityUUID}/cancel`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `teams/${identityUUID}/audit-events`,
    methods: ["GET"],
    query: ["cursor", "limit", "action", "from", "to"],
  },
  {
    pattern: `teams/${identityUUID}/invitations`,
    methods: ["GET", "POST"],
    query: ["cursor", "limit", "status"],
  },
  {
    pattern: `teams/${identityUUID}/invitations/${identityUUID}/(resend|revoke)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `invitations/(preview|accept|decline)`,
    methods: ["POST"],
    query: [],
  },
];

// Explicit editor and catalog contracts; authorization remains with the backend.
const editorCatalogRoutes: typeof identityRoutes = [
  {
    pattern: `projects/${identityUUID}/prompt-drafts`,
    methods: ["GET", "POST"],
    query: ["cursor"],
  },
  {
    pattern: `projects/${identityUUID}/prompt-drafts/${identityUUID}`,
    methods: ["GET", "PUT"],
    query: [],
  },
  {
    pattern: `projects/${identityUUID}/prompt-references/validate`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `projects/${identityUUID}/prompt-uploads/${identityUUID}`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `projects/${identityUUID}/prompt-submissions/current`,
    methods: ["GET"],
    query: ["owner_kind", "draft_id", "canvas_id", "node_id"],
  },
  {
    pattern: `projects/${identityUUID}/prompt-submissions/${identityUUID}/resolve`,
    methods: ["POST"],
    query: [],
  },
  { pattern: `projects/${identityUUID}/quotes`, methods: ["POST"], query: [] },
  { pattern: `batches/${identityUUID}`, methods: ["GET"], query: [] },
  {
    pattern: `batches/${identityUUID}/(confirm|cancel|resume)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `projects/${identityUUID}/model-defaults`,
    methods: ["GET", "PATCH"],
    query: [],
  },
  { pattern: `settings/prompt-preferences`, methods: ["GET"], query: [] },
  {
    pattern: `settings/prompt-preferences/[a-z][a-z0-9_.-]{0,127}`,
    methods: ["PUT"],
    query: [],
  },
  { pattern: `admin/ai-trials`, methods: ["POST"], query: [] },
  { pattern: `admin/ai-trials/${identityUUID}`, methods: ["GET"], query: [] },
  {
    pattern: `admin/ai-trials/${identityUUID}/(turns|cancel|recover)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `admin/ai-trials/${identityUUID}/artifacts/1`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `admin/(providers|models)`,
    methods: ["GET", "POST"],
    query: ["limit", "cursor"],
  },
  {
    pattern: `admin/providers/${identityUUID}`,
    methods: ["GET", "PATCH"],
    query: [],
  },
  {
    pattern: `admin/providers/${identityUUID}/credentials`,
    methods: ["GET", "PUT"],
    query: [],
  },
  {
    pattern: `admin/providers/${identityUUID}/credentials/${identityUUID}/(disable|test)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `admin/providers/${identityUUID}/config`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `admin/providers/${identityUUID}/(upgrade-preview|apply-preset)`,
    methods: ["POST"],
    query: [],
  },
  { pattern: `admin/models/${identityUUID}`, methods: ["GET"], query: [] },
  {
    pattern: `admin/models/${identityUUID}/(versions|prices)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `admin/models/${identityUUID}/status`,
    methods: ["PATCH"],
    query: [],
  },
  {
    pattern: `admin/models/${identityUUID}/releases`,
    methods: ["GET", "POST"],
    query: [],
  },
  {
    pattern: `admin/model-releases/${identityUUID}/(activate|trial-quotes)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `admin/(capabilities|capability-defaults)`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `admin/provider-checks/${identityUUID}`,
    methods: ["GET"],
    query: [],
  },
  {
    pattern: `admin/config-requests/${identityUUID}`,
    methods: ["GET"],
    query: ["action", "target_id"],
  },
  { pattern: `admin/provider-presets`, methods: ["GET", "POST"], query: [] },
  {
    pattern: `admin/provider-presets/import-preview`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `admin/provider-presets/${identityUUID}/(versions|connections)`,
    methods: ["POST"],
    query: [],
  },
  {
    pattern: `admin/provider-presets/${identityUUID}/state`,
    methods: ["PATCH"],
    query: [],
  },
  {
    pattern: `admin/provider-presets/${identityUUID}/export`,
    methods: ["GET"],
    query: [],
  },
];

// The browser cannot choose an upstream host or use this slice as a general proxy.
const resources: Record<string, readonly string[]> = {
  projects: ["GET", "POST"],
  "media/library": ["GET"],
  "media/library/commands": ["POST"],
  "media/library/uploads": ["POST"],
};
const noStore = { "Cache-Control": "no-store" };
async function proxy(
  request: Request,
  context: { params: Promise<{ path: string[] }> },
) {
  const { path } = await context.params;
  const resource = path.join("/");
  const uuid = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
  const matches = (pattern: string) =>
    new RegExp(`^${pattern}$`, "i").test(resource);
  const flowMethods =
    resource === "models"
      ? ["GET"]
      : matches(`projects/${uuid}/(operations|media)`)
        ? ["GET"]
        : matches(`projects/${uuid}/(free-operations|media/uploads)`)
          ? ["POST"]
          : matches(`projects/${uuid}/media/${uuid}/preview`)
            ? ["GET"]
            : matches(`operations/${uuid}`)
              ? ["GET"]
              : matches(`operations/${uuid}/(confirm|cancel)`)
                ? ["POST"]
                : matches(`projects/${uuid}/bible/(character|location|prop)`)
                  ? ["GET", "POST"]
                  : matches(
                        `projects/${uuid}/bible/(character|location|prop)/${uuid}`,
                      )
                    ? ["GET", "PUT", "DELETE"]
                    : matches(
                          `projects/${uuid}/bible/(character|location|prop)/${uuid}/(restore|confirm|looks)`,
                        )
                      ? ["POST"]
                      : matches(
                            `projects/${uuid}/bible/character/${uuid}/looks/${uuid}/references`,
                          )
                        ? ["PUT"]
                        : matches(
                              `projects/${uuid}/bible/(character|location|prop)/${uuid}/versions`,
                            )
                          ? ["GET"]
                          : undefined;
  const boundedRoute = [
    {
      pattern: `projects/${identityUUID}`,
      methods: ["GET", "PATCH", "DELETE"],
      query: [] as string[],
    },
    {
      pattern: `projects/${identityUUID}/(archive|unarchive|restore)`,
      methods: ["POST"],
      query: [] as string[],
    },
    {
      pattern: `project-creations/${identityUUID}`,
      methods: ["GET"],
      query: [] as string[],
    },
    ...identityRoutes,
    ...editorCatalogRoutes,
  ].find((rule) => new RegExp(`^${rule.pattern}$`, "i").test(resource));
  const methods =
    boundedRoute?.methods ??
    flowMethods ??
    resources[resource] ??
    (matches(`projects/${uuid}/canvases`) ? ["GET", "POST"] : undefined) ??
    (/^media\/library\/items\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(\/(preview|download))?$/i.test(
      resource,
    )
      ? ["GET"]
      : undefined) ??
    (/^sessions\/[0-9a-f-]{36}$/i.test(resource)
      ? ["DELETE"]
      : /^(projects|canvases)\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
            resource,
          )
        ? ["GET"]
        : undefined);
  if (!methods)
    return Response.json(
      { code: "not_found" },
      { status: 404, headers: noStore },
    );
  if (!methods.includes(request.method))
    return Response.json(
      { code: "method_not_allowed" },
      { status: 405, headers: { ...noStore, Allow: methods.join(", ") } },
    );
  const incoming = new URL(request.url);
  if (boundedRoute) {
    for (const key of incoming.searchParams.keys()) {
      if (
        request.method !== "GET" ||
        !boundedRoute.query.includes(key) ||
        incoming.searchParams.getAll(key).length !== 1
      )
        return Response.json(
          { code: "invalid_request" },
          { status: 422, headers: noStore },
        );
    }
  }
  if (
    resource === "projects" &&
    incoming.searchParams.has("order") &&
    (request.method !== "GET" ||
      incoming.searchParams.getAll("order").length !== 1 ||
      incoming.searchParams.get("order") !== "recent_content")
  )
    return Response.json(
      { code: "invalid_request" },
      { status: 422, headers: noStore },
    );
  let uploadTooLarge = false;
  try {
    const upstream = new URL(
      `/api/${resource}`,
      process.env.GLOWLOOM_BACKEND_ORIGIN ??
        process.env.LV_BACKEND_ORIGIN ??
        "http://127.0.0.1:8080",
    );
    if (boundedRoute)
      for (const [key, value] of incoming.searchParams)
        upstream.searchParams.set(key, value);
    if (resource === "projects")
      for (const key of [
        "q",
        "limit",
        "cursor",
        "order",
        "status",
        "deleted",
        "include_expired",
      ])
        if (incoming.searchParams.has(key))
          upstream.searchParams.set(key, incoming.searchParams.get(key)!);
    if (flowMethods && request.method === "GET")
      for (const key of [
        "project_id",
        "kind",
        "capability",
        "mode",
        "limit",
        "cursor",
        "status",
        "canvas_id",
        "node_id",
        "origin",
        "row_id",
        "model_key",
        "deleted",
      ])
        if (incoming.searchParams.has(key))
          upstream.searchParams.set(key, incoming.searchParams.get(key)!);
    if (resource.startsWith("media/library") && request.method === "GET") {
      const scope = incoming.searchParams.get("scope") ?? "personal";
      const projectID = incoming.searchParams.get("project_id");
      if (
        incoming.searchParams.getAll("scope").length > 1 ||
        incoming.searchParams.getAll("project_id").length > 1 ||
        !["personal", "project"].includes(scope) ||
        (scope === "project"
          ? !projectID || !new RegExp(`^${identityUUID}$`, "i").test(projectID)
          : projectID !== null)
      )
        return Response.json(
          { code: "invalid_request" },
          { status: 422, headers: noStore },
        );
      const allowed =
        resource === "media/library"
          ? [
              "scope",
              "project_id",
              "page",
              "page_size",
              "kind",
              "category",
              "folder_id",
              "root_only",
              "favorite_only",
              "recent_only",
              "catalog_state",
              "search",
              "order",
            ]
          : ["scope", "project_id"];
      for (const key of allowed)
        if (incoming.searchParams.has(key))
          upstream.searchParams.set(key, incoming.searchParams.get(key)!);
      // The backend checks membership and asset ownership for the explicit library scope.
      upstream.searchParams.set("scope", scope);
    }
    const headers = new Headers();
    for (const key of [
      "Cookie",
      "Origin",
      "Idempotency-Key",
      "Content-Type",
      "X-Request-Id",
      "X-CSRF-Token",
      "X-Creation-Scope",
    ]) {
      const value = request.headers.get(key);
      if (value) headers.set(key, value);
    }
    if (
      resource === "me/avatar/uploads" ||
      resource === "media/library/uploads" ||
      matches(`projects/${uuid}/media/uploads`)
    ) {
      let bytes = 0;
      const body = request.body?.pipeThrough(
        new TransformStream({
          transform(chunk, controller) {
            bytes += chunk.byteLength;
            if (bytes > 501 * 1024 * 1024) {
              uploadTooLarge = true;
              controller.error(new Error("upload_too_large"));
              return;
            }
            controller.enqueue(chunk);
          },
        }),
      );
      const response = await fetch(upstream, {
        method: "POST",
        headers,
        body,
        duplex: "half",
        cache: "no-store",
        redirect: "manual",
        signal: AbortSignal.any([request.signal, AbortSignal.timeout(120000)]),
      } as RequestInit & { duplex: "half" });
      return new Response(response.body, {
        status: response.status,
        headers: {
          ...noStore,
          "Content-Type":
            response.headers.get("Content-Type") ?? "application/json",
        },
      });
    }
    // Read incrementally so a hostile body cannot allocate unbounded memory.
    const reader = request.body?.getReader();
    const chunks: Uint8Array[] = [];
    let size = 0;
    if (reader) {
      try {
        while (true) {
          const next = await reader.read();
          if (next.done) break;
          size += next.value.byteLength;
          if (size > 1048576) {
            await reader.cancel();
            return Response.json(
              { code: "request_too_large" },
              { status: 413, headers: noStore },
            );
          }
          chunks.push(next.value);
        }
      } finally {
        reader.releaseLock();
      }
    }
    const body = size ? Buffer.concat(chunks) : undefined;
    const response = await fetch(upstream, {
      method: request.method,
      headers,
      body,
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(10000)]),
    });
    const outgoing = new Headers(noStore);
    for (const key of [
      "Content-Type",
      "X-Request-Id",
      "Content-Disposition",
      "Retry-After",
      "X-Content-Type-Options",
    ]) {
      const value = response.headers.get(key);
      if (value) outgoing.set(key, value);
    }
    for (const cookie of response.headers.getSetCookie())
      outgoing.append("Set-Cookie", cookie);
    return new Response(response.body, {
      status: response.status,
      headers: outgoing,
    });
  } catch {
    if (uploadTooLarge)
      return Response.json(
        { code: "request_too_large" },
        { status: 413, headers: noStore },
      );
    return Response.json(
      { code: "dependency_unavailable" },
      { status: 503, headers: noStore },
    );
  }
}
export const GET = proxy;
export const POST = proxy;
export const DELETE = proxy;

export const PUT = proxy;

export const PATCH = proxy;
