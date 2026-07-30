import type { Metadata } from "next";

import { GovernanceWorkspace } from "./governance-workspace";

export const metadata: Metadata = {
  title: "授权治理 · Lanverse",
  description: "管理 AI 漫剧固定版本的授权范围、证明与撤销历史",
};

export default function GovernancePage() {
  return <GovernanceWorkspace />;
}
