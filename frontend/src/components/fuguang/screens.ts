/** 当前设计页面。页面内示例数据尚未连接业务接口。 */
export const screens = [
  { id: "home", name: "首页与项目库", node: "11:2", group: "创作" },
  { id: "canvas", name: "无限画布", node: "3:2", group: "创作" },
  { id: "shot", name: "镜头详情", node: "4:2", group: "创作" },
  { id: "storyboard", name: "分镜故事板", node: "5:2", group: "创作" },
  { id: "analytics", name: "数据分析", node: "6:2", group: "创作" },
  { id: "bible", name: "设定集", node: "7:2", group: "创作" },
  { id: "assets", name: "素材库", node: "8:2", group: "创作" },
  { id: "login", name: "登录", node: "9:2", group: "账号" },
  { id: "register", name: "注册", node: "10:2", group: "账号" },
  { id: "reset-password", name: "设置新密码", node: "12:2", group: "账号" },
  { id: "account", name: "账号设置", node: "13:2", group: "账号" },
  { id: "users", name: "管理账号", node: "14:2", group: "管理" },
  { id: "providers", name: "供应商凭据", node: "16:2", group: "管理" },
  { id: "models", name: "模型注册表", node: "17:2", group: "管理" },
  { id: "audit", name: "审计日志", node: "18:2", group: "管理" },
  { id: "health", name: "系统健康", node: "19:2", group: "管理" },
  { id: "design-system", name: "设计语言", node: "1:2", group: "规范" },
  { id: "layout", name: "布局规范", node: "2:2", group: "规范" },
] as const;
export type Screen = (typeof screens)[number]["id"];
export const screenHref = (screen: string) =>
  screen === "home" ? "/" : `/${screen}`;
