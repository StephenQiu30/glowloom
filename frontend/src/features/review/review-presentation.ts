
export type TaskFilter = "active" | API.HumanTaskBaseResponse["status"];

export type DecisionValue = API.HumanTaskDecisionRequest["decision"];

export const taskFilterLabels: Record<TaskFilter, string> = {
  active: "待处理",
  OPEN: "未领取",
  CLAIMED: "处理中",
  COMPLETED: "已决议",
  CANCELLED: "已取消",
  STALE: "已过期",
};

export const taskStatusLabels: Record<API.HumanTaskBaseResponse["status"], string> = {
  OPEN: "待领取",
  CLAIMED: "处理中",
  COMPLETED: "已决议",
  CANCELLED: "已取消",
  STALE: "事实已过期",
};

export const decisionLabels: Record<DecisionValue, string> = {
  approved: "接受",
  rejected: "拒绝",
  changes_requested: "要求修改",
  selected: "确认选择",
};

export const knownSubjectTypes = new Set([
  "workflow_node_output",
  "generation_candidate_selection",
]);

export function subjectLabel(value: string): string {
  switch (value) {
    case "workflow_node_output":
      return "工作流节点输出";
    case "generation_candidate_selection":
      return "生成候选选择";
    default:
      return value;
  }
}

export function shortId(value: string): string {
  return value.length > 16 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value;
}

export function shortHash(value: string): string {
  return value ? `${value.slice(0, 12)}…${value.slice(-8)}` : "尚未生成";
}

export function formatTimestamp(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
    hour12: false,
  }).format(new Date(value));
}
