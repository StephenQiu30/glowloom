import type { AccessKey } from "@/components/canvas/runtime/components/account/lib/access";

export type WorkspaceRoute = {
  href: string;
  title: string;
  group: string;
  access: AccessKey;
};
export const workspaceRoutes = {
  home: { href: "/", title: "创作首页", group: "创作空间", access: "public" },
  projects: {
    href: "/projects",
    title: "项目空间",
    group: "创作空间",
    access: "public",
  },
  canvases: {
    href: "/canvases",
    title: "我的画布",
    group: "创作空间",
    // Canvas owns its identity gate so expiration cannot unmount an unsaved session.
    access: "public",
  },
  localCanvases: {
    href: "/local/canvases",
    title: "本机画布",
    group: "创作空间",
    access: "public",
  },
  teams: {
    href: "/teams",
    title: "团队空间",
    group: "创作空间",
    access: "readTeams",
  },
  assets: {
    href: "/assets",
    title: "素材中心",
    group: "创作空间",
    access: "public",
  },
  image: {
    href: "/image",
    title: "图片生成",
    group: "创作空间",
    access: "public",
  },
  tasks: {
    href: "/tasks",
    title: "任务记录",
    group: "创作空间",
    access: "authenticated",
  },
  notifications: {
    href: "/notifications",
    title: "通知中心",
    group: "创作空间",
    access: "authenticated",
  },
  workflows: {
    href: "/workflows",
    title: "工作流程",
    group: "创作空间",
    access: "authenticated",
  },
  dashboard: {
    href: "/admin/dashboard",
    title: "数据中心",
    group: "数据中心",
    access: "viewDashboard",
  },
  dashboardModels: {
    href: "/admin/dashboard/models",
    title: "模型与任务",
    group: "数据中心",
    access: "viewDashboard",
  },
  dashboardUsers: {
    href: "/admin/dashboard/users",
    title: "用户与授权",
    group: "数据中心",
    access: "viewDashboard",
  },
  dashboardConsumption: {
    href: "/admin/dashboard/consumption",
    title: "积分与消耗",
    group: "数据中心",
    access: "readCreditActivity",
  },
  settings: {
    href: "/account/settings",
    title: "账户与安全",
    group: "创作空间",
    access: "authenticated",
  },
  video: {
    href: "/video",
    title: "视频生成",
    group: "创作空间",
    access: "public",
  },
  account: {
    href: "/account",
    title: "个人中心",
    group: "创作空间",
    access: "authenticated",
  },
  credits: {
    href: "/account/credits",
    title: "我的积分",
    group: "账户",
    access: "authenticated",
  },
  creditManagement: {
    href: "/admin/credits",
    title: "积分管理",
    group: "管理",
    access: "manageCredits",
  },
  creditGrant: {
    href: "/admin/credits/grant",
    title: "发放积分",
    group: "管理",
    access: "manageCredits",
  },
  creditPrices: {
    href: "/admin/credits/prices",
    title: "模型接入",
    group: "管理",
    access: "manageCredits",
  },
  users: {
    href: "/admin/users",
    title: "用户管理",
    group: "管理",
    access: "manageUsers",
  },
  models: {
    href: "/admin/models",
    title: "模型接入",
    group: "管理",
    access: "readModels",
  },
  modelPresets: {
    href: "/admin/models/presets",
    title: "模型预设",
    group: "管理",
    access: "manageModels",
  },
  authority: {
    href: "/admin/authority",
    title: "权限设置",
    group: "管理",
    access: "manageAuthority",
  },
  assetReviews: {
    href: "/admin/assets",
    title: "素材审核",
    group: "管理",
    access: "manageAssets",
  },
  logs: {
    href: "/admin/logs",
    title: "日志中心",
    group: "管理",
    access: "viewLogs",
  },
} as const satisfies Record<string, WorkspaceRoute>;

export const managementRoutes = [
  workspaceRoutes.dashboard,
  workspaceRoutes.users,
  workspaceRoutes.authority,
  workspaceRoutes.models,
  workspaceRoutes.logs,
  workspaceRoutes.assetReviews,
  workspaceRoutes.creditManagement,
];
const editorPath = /^\/canvas\/[^/]+\/?$/;
export function resolveWorkspaceRoute(pathname: string): {
  mode: "workspace" | "canvas";
  route: WorkspaceRoute;
} {
  if (/^\/projects\/[1-9]\d*\/?$/.test(pathname)) {
    return {
      mode: "workspace",
      route: { ...workspaceRoutes.projects, access: "authenticated" },
    };
  }
  if (/^\/(local\/)?canvases\/[^/]+\/?$/.test(pathname)) {
    return {
      mode: "canvas",
      route: pathname.startsWith("/local/")
        ? workspaceRoutes.localCanvases
        : workspaceRoutes.canvases,
    };
  }
  if (editorPath.test(pathname))
    return { mode: "canvas", route: workspaceRoutes.projects };
  const normalizedPath = pathname.replace(/\/+$/, "") || "/";
  const route = Object.values(workspaceRoutes).find(
    (item) => normalizedPath === item.href,
  );
  return {
    mode: "workspace",
    route: route ?? {
      href: pathname,
      title: "页面",
      group: "工作空间",
      access: "denied",
    },
  };
}
