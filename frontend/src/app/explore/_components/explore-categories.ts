type ExploreCategory = {
  id: string;
  label: string;
  kind: "work" | "skill" | "activity";
  scenes: { id: string; label: string }[];
};

function scenes(namespace: string, values: [string, string][]) {
  return values.map(([id, label]) => ({ id: namespace + "." + id, label }));
}

export const exploreCategories: ExploreCategory[] = [
  {
    id: "work.drama",
    label: "短剧漫剧",
    kind: "work",
    scenes: scenes("drama", [
      ["short", "短剧作品"],
      ["comic", "漫剧作品"],
    ]),
  },
  {
    id: "work.film",
    label: "影视作品",
    kind: "work",
    scenes: scenes("film", [
      ["short", "短片作品"],
      ["music", "音乐MV"],
      ["visual", "视觉设定"],
    ]),
  },
  {
    id: "work.animation",
    label: "动画动漫",
    kind: "work",
    scenes: scenes("animation", [
      ["film", "动画影片"],
      ["visual", "视觉设定"],
    ]),
  },
  {
    id: "work.advertising",
    label: "广告营销",
    kind: "work",
    scenes: scenes("advertising", [
      ["brand", "品牌广告"],
      ["social", "社媒营销"],
    ]),
  },
  {
    id: "work.graphic",
    label: "平面设计",
    kind: "work",
    scenes: scenes("graphic", [
      ["poster", "海报"],
      ["brand", "品牌设计"],
    ]),
  },
  {
    id: "work.commerce",
    label: "电商",
    kind: "work",
    scenes: scenes("commerce", [
      ["hero", "电商头图"],
      ["product", "商品展示"],
    ]),
  },
  {
    id: "work.other",
    label: "其他",
    kind: "work",
    scenes: scenes("other", [
      ["inspiration", "灵感"],
      ["short", "短片"],
    ]),
  },
  {
    id: "skill",
    label: "技能",
    kind: "skill",
    scenes: scenes("skill", [
      ["recommended", "热门推荐"],
      ["film", "影视短片"],
      ["commerce", "电商"],
      ["advertising", "商业广告"],
      ["general", "通用创作"],
      ["graphic", "平面设计"],
      ["social", "社媒营销"],
      ["entertainment", "娱乐"],
      ["animation", "动漫游戏"],
      ["master-style", "大师风格复刻"],
      ["camera", "专业运镜"],
      ["discover", "发现"],
    ]),
  },
  { id: "activity", label: "活动", kind: "activity", scenes: [] },
];
