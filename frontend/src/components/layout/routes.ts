/** 全站主稿对应的设计入口；业务可用性由真实服务决定。 */
export const screens = [
  { id: "notifications", name: "通知中心", node: "4109:136868", group: "账号" },
  {
    id: "platform/notifications",
    name: "通知管理",
    node: "4108:128603",
    group: "管理",
  },
  { id: "conversations", name: "对话", node: "3494:92779", group: "创作" },
  { id: "home", name: "创作", node: "4186:142205", group: "创作" },
  { id: "explore", name: "探索", node: "4186:142460", group: "创作" },
  { id: "ai-tools", name: "AI工具", node: "4186:142356", group: "创作" },
  { id: "projects", name: "我的画布", node: "3713:110684", group: "创作" },
  { id: "canvas", name: "画布", node: "4170:141815", group: "创作" },
  { id: "assets", name: "资产和主体", node: "4186:142399", group: "创作" },
  { id: "subjects", name: "主体", node: "3713:110935", group: "创作" },
  {
    id: "motion-studio",
    name: "动效工作室",
    node: "4166:141667",
    group: "创作",
  },
  {
    id: "marketing-studio",
    name: "营销工作室",
    node: "4166:141836",
    group: "创作",
  },
  { id: "short-drama", name: "短剧工作室", node: "4166:140819", group: "创作" },
  { id: "login", name: "登录", node: "4099:126474", group: "账号" },
  { id: "register", name: "注册", node: "4099:126511", group: "账号" },
  { id: "reset-password", name: "修改密码", node: "1467:11857", group: "账号" },
  { id: "teams", name: "团队", node: "3393:73660", group: "账号" },
  { id: "invitations", name: "邀请与授权", node: "3393:73935", group: "账号" },
  {
    id: "platform/teams",
    name: "平台团队",
    node: "4108:127110",
    group: "管理",
  },
  {
    id: "platform/settings",
    name: "平台设置",
    node: "4108:128289",
    group: "管理",
  },
  { id: "account", name: "个人空间", node: "4166:141002", group: "账号" },
  { id: "billing", name: "积分与会员", node: "4166:141347", group: "账号" },
  { id: "users", name: "用户管理", node: "4108:126578", group: "管理" },
  { id: "providers", name: "AI 服务接入", node: "4146:137773", group: "管理" },
  { id: "models", name: "AI 服务", node: "4146:137978", group: "管理" },
  { id: "audit", name: "审计日志", node: "4108:129597", group: "管理" },
  { id: "health", name: "运行诊断", node: "4108:129086", group: "管理" },
  { id: "bible", name: "主体", node: "3713:110935", group: "创作" },
  { id: "analytics", name: "数据分析", node: "3993:119616", group: "创作" },
] as const;
export type Screen = (typeof screens)[number]["id"];
export const screenHref = (screen: string) =>
  screen === "home" ? "/" : `/${screen}`;

/** Match exact routes before nested routes; never treat / as a prefix of every page. */
export function resolveScreen(pathname: string) {
  return (
    screens.find((screen) => screenHref(screen.id) === pathname) ??
    screens.find(
      (screen) =>
        screen.id !== "home" &&
        pathname.startsWith(`${screenHref(screen.id)}/`),
    )
  );
}

/** Resolve the active workspace navigation item for each management route. */
export function resolveAdminSection(screen: Screen, platformAdmin = false) {
  switch (screen) {
    case "providers":
      return "models";
    case "users":
    case "models":
    case "health":
    case "audit":
      return screen;
    case "analytics":
      return platformAdmin ? screen : undefined;
    case "platform/teams":
      return "teams";
    case "platform/settings":
      return "settings";
    case "platform/notifications":
      return "notifications";
    default:
      return undefined;
  }
}
export type AdminSection = NonNullable<ReturnType<typeof resolveAdminSection>>;
