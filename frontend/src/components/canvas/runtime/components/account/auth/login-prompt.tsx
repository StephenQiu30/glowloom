"use client";

import {
  PageEmpty,
  PageLoading,
} from "@/components/canvas/runtime/components/feedback/page-state";
import { Button } from "@/components/ui/button";
import { useAuthDialog } from "./auth-dialog-provider";
import { useAccount } from "../account-provider";
import { getAccessState } from "../lib/access";

/** 可在受保护页面或局部 Access fallback 中展示。 */
export function LoginPrompt({
  description = "登录后访问此内容。",
}: {
  description?: string;
}) {
  const openAuth = useAuthDialog();
  const auth = useAccount();
  const state = getAccessState(auth, "authenticated");
  if (state === "loading") return <PageLoading label="正在读取账户" />;
  if (state !== "anonymous" && state !== "error") return null;
  return (
    <>
      <PageEmpty
        title={state === "error" ? "请登录以继续" : "请先登录"}
        description={description}
      >
        <Button size="lg" className="h-10 px-6" onClick={() => openAuth()}>
          登录
        </Button>
      </PageEmpty>
    </>
  );
}
