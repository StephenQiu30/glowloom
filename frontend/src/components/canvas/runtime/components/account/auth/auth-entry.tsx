"use client";

import { Button } from "@/components/ui/button";
import { useAuthDialog } from "./auth-dialog-provider";

/** 页头账户入口共用同一弹窗，直接选择登录或注册。 */
export function AuthEntry({ disabled = false }: { disabled?: boolean }) {
  const openAuth = useAuthDialog();
  return (
    <div
      className="auth-entry flex items-center gap-2"
      role="group"
      aria-label="账户"
    >
      <Button
        variant="ghost"
        size="sm"
        disabled={disabled}
        onClick={() => openAuth("login")}
      >
        登录
      </Button>
      <Button
        size="sm"
        disabled={disabled}
        onClick={() => openAuth("register")}
      >
        注册
      </Button>
    </div>
  );
}
