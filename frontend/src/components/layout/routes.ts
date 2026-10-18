/** R6 当前设计入口；业务可用性由真实服务决定。 */
export const screens = [
  { id: "home", name: "创作", node: "271:3620", group: "创作" },
  { id: "explore", name: "探索", node: "271:3620", group: "创作" },
  { id: "projects", name: "我的画布", node: "347:6984", group: "创作" },
  { id: "canvas", name: "画布", node: "271:3302", group: "创作" },
  { id: "assets", name: "素材库", node: "645:24800", group: "创作" },
  { id: "subjects", name: "主体", node: "7:2", group: "创作" },
  { id: "login", name: "登录", node: "9:2", group: "账号" },
  { id: "register", name: "注册", node: "354:13103", group: "账号" },
  { id: "reset-password", name: "修改密码", node: "339:6455", group: "账号" },
  { id: "teams", name: "团队", node: "13:2", group: "账号" },
  { id: "invitations", name: "邀请与授权", node: "13:2", group: "账号" },
  { id: "platform/teams", name: "平台团队", node: "13:2", group: "管理" },
  { id: "platform/settings", name: "平台设置", node: "13:2", group: "管理" },
  { id: "account", name: "账号设置", node: "13:2", group: "账号" },
  { id: "users", name: "用户管理", node: "363:16818", group: "管理" },
  { id: "providers", name: "AI 平台接入", node: "16:2", group: "管理" },
  { id: "models", name: "模型管理", node: "17:2", group: "管理" },
  { id: "audit", name: "审计日志", node: "18:2", group: "管理" },
  { id: "health", name: "系统健康", node: "19:2", group: "管理" },
  { id: "bible", name: "主体", node: "7:2", group: "创作" },
  { id: "storyboard", name: "分镜", node: "5:2", group: "创作" },
  { id: "shot", name: "镜头", node: "4:2", group: "创作" },
  { id: "analytics", name: "统计", node: "6:2", group: "创作" },
  { id: "design-system", name: "设计规范", node: "365:17335", group: "规范" },
  { id: "layout", name: "布局规范", node: "365:17335", group: "规范" },
] as const;
export type Screen = (typeof screens)[number]["id"];
export const screenHref = (screen: string) =>
  screen === "home" ? "/" : `/${screen}`;
