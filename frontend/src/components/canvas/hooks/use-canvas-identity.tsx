"use client";
import { useState } from "react";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { LoginPrompt } from "@/components/canvas/runtime/components/account/auth/login-prompt";
import type { CanvasScope } from "./use-canvas-document";

export function useCanvasIdentity(scope: CanvasScope) {
  const { account, ready, signingOut } = useAccount();
  const current =
    scope === "LOCAL" ? "LOCAL" : account ? `USER:${account.id}` : null;
  const [identity, setIdentity] = useState(current);
  if (!identity && current) setIdentity(current);
  return {
    identity,
    enabled:
      scope === "LOCAL" ||
      Boolean(ready && !signingOut && identity && identity === current),
  };
}

export function CanvasIdentityLock({
  hasDraft = true,
}: {
  hasDraft?: boolean;
}) {
  return (
    <section className="flex h-full min-h-0 flex-1 flex-col" role="status">
      {hasDraft && (
        <p className="p-6">
          画布会话已暂停。草稿和原命令保留在当前页面，请使用打开画布时的账户恢复。
        </p>
      )}
      <LoginPrompt
        description={
          hasDraft ? "请登录原账户后继续保存。" : "登录后创建和编辑画布。"
        }
      />
    </section>
  );
}
