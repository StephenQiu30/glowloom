import type { Metadata } from "next";

import {
  type GovernancePrefill,
  GovernanceWorkspace,
} from "./governance-workspace";

export const metadata: Metadata = {
  title: "授权治理 · Lanverse",
  description: "管理 AI 漫剧固定版本的授权范围、证明与撤销历史",
};

function first(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

export default async function GovernancePage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const params = await searchParams;
  const subjectId = first(params.subjectId);
  const prefill: GovernancePrefill | undefined =
    first(params.subjectType) === "ASSET_VERSION" && subjectId
      ? {
          subjectType: "ASSET_VERSION",
          subjectId,
          proofMediaVersionId: first(params.proofMediaVersionId),
        }
      : undefined;
  return <GovernanceWorkspace prefill={prefill} />;
}
