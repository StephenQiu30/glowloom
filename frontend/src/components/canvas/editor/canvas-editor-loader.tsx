"use client";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PageEmpty } from "@/components/canvas/runtime/components/feedback/page-state";
import { LoginPrompt } from "@/components/canvas/runtime/components/account/auth/login-prompt";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";

export default function CanvasEditorLoader() {
  const { account, ready, signingOut } = useAccount();
  if (!ready || !account || signingOut)
    return <LoginPrompt description="登录后访问画布。" />;
  return (
    <PageEmpty
      title="旧测试画布已停用"
      description="请从项目空间新建画布，或打开已保存的画布。"
    >
      <Button asChild>
        <Link href="/projects">返回项目空间</Link>
      </Button>
    </PageEmpty>
  );
}
