export type MockProject = {
  id: string;
  name: string;
  tagline: string;
  cover: string;
  style: string;
  ratio: string;
  episodes: number;
  currentEpisode: number;
  progress: number;
  currentStage: string;
  updatedAt: string;
  status: "active" | "draft" | "review";
};

export const mockProjects: MockProject[] = [
  {
    id: "painting-girl",
    name: "她从画中来",
    tagline: "一卷失传古画，连接两个时代的命运。",
    cover: "/assets/lanverse-studio/painting-girl-cover.png",
    style: "水墨幻想",
    ratio: "9:16",
    episodes: 16,
    currentEpisode: 8,
    progress: 46,
    currentStage: "资产",
    updatedAt: "刚刚更新",
    status: "active",
  },
  {
    id: "changan-night",
    name: "长安夜行录",
    tagline: "雨夜城门下，一场跨越十年的追凶。",
    cover: "/assets/lanverse-studio/changan-night-cover.png",
    style: "国漫电影感",
    ratio: "9:16",
    episodes: 24,
    currentEpisode: 12,
    progress: 68,
    currentStage: "生成",
    updatedAt: "2 小时前",
    status: "review",
  },
  {
    id: "wasteland-inn",
    name: "我在末世开客栈",
    tagline: "当文明坍塌，最后一间客栈仍亮着灯。",
    cover: "/assets/lanverse-studio/wasteland-inn-cover.png",
    style: "末世厚涂",
    ratio: "9:16",
    episodes: 18,
    currentEpisode: 3,
    progress: 22,
    currentStage: "剧本",
    updatedAt: "昨天",
    status: "draft",
  },
];

export const mockEpisodes = [
  { id: "ep-08", index: 8, title: "画中人", duration: "01:34", shots: 26, ready: 18, status: "资产确认中" },
  { id: "ep-07", index: 7, title: "雨夜故人", duration: "01:42", shots: 28, ready: 28, status: "已交付" },
  { id: "ep-06", index: 6, title: "墨痕", duration: "01:28", shots: 24, ready: 24, status: "已交付" },
  { id: "ep-05", index: 5, title: "旧画新主", duration: "01:37", shots: 27, ready: 27, status: "已交付" },
];

export const mockWorkspaces = [
  { id: "personal", name: "Stephen 的创作空间", role: "所有者", projects: 3, members: 1, storage: "18.4 GB", active: true },
  { id: "ink-studio", name: "青墨漫剧工作室", role: "管理员", projects: 12, members: 8, storage: "126.8 GB", active: false },
  { id: "archive", name: "2025 存档空间", role: "所有者", projects: 7, members: 2, storage: "64.2 GB", active: false, archived: true },
];

export const mockProductionStages = ["剧本", "资产", "分镜", "生成", "审核", "交付"];
