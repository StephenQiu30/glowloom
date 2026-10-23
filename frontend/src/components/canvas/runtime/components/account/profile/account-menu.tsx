"use client";

import Link from "next/link";
import { useState } from "react";
import {
  FileText,
  KeyRound,
  LogOut,
  Moon,
  ShieldCheck,
  UserRound,
  X,
} from "lucide-react";
import { useAccount } from "../account-provider";
import { avatarSource } from "../lib/avatar-source";
import { useAccess } from "../hooks/use-access";
import { managementRoutes } from "@/components/canvas/runtime/components/layout/lib/workspace-routes";
import { AuthEntry } from "../auth/auth-entry";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverTitle,
} from "@/components/ui/popover";
import { toast } from "sonner";

export function AccountMenu() {
  const [open, setOpen] = useState(false);
  const toggleTheme = useThemeStore((state) => state.toggleTheme);
  const auth = useAccount();
  const access = useAccess();
  const visibleManagementRoutes = managementRoutes.filter(
    (route) => route.href === "/admin/users" && access[route.access],
  );
  if (!auth.ready)
    return (
      <Skeleton aria-label="正在读取账户" className="size-8 rounded-full" />
    );

  const account = auth.account;

  async function logout() {
    try {
      await auth.signOut();
      toast.success("已退出登录");
    } catch (error) {
      notifyError(error, "退出登录未完成");
    }
  }

  if (!account) return <AuthEntry disabled={auth.signingOut} />;

  const name = account.userName || account.userAccount || "账户";
  const imageSource = avatarSource(account.userAvatar);
  const avatar = (
    <Avatar>
      {imageSource ? (
        <AvatarImage src={imageSource} alt="" referrerPolicy="no-referrer" />
      ) : null}
      <AvatarFallback>{Array.from(name)[0]?.toUpperCase()}</AvatarFallback>
    </Avatar>
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={`${name}的账户菜单`}
          disabled={auth.signingOut}
        >
          {avatar}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="account-popover" aria-label="账户">
        <div className="account-popover-heading">
          <PopoverTitle>账户</PopoverTitle>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="关闭账户菜单"
            onClick={() => setOpen(false)}
          >
            <X />
          </Button>
        </div>
        <div className="account-popover-identity">
          {avatar}
          <div className="grid min-w-0 gap-0.5">
            <span className="truncate">{name}</span>
            <span className="truncate text-xs font-normal text-muted-foreground">
              Glowloom · 创作者
            </span>
          </div>
        </div>
        <div className="account-popover-actions">
          <Button asChild variant="ghost" onClick={() => setOpen(false)}>
            <Link href="/account/credits">
              <FileText />
              我的积分
            </Link>
          </Button>
          <Button asChild variant="ghost" onClick={() => setOpen(false)}>
            <Link href="/account">
              <UserRound />
              个人中心
            </Link>
          </Button>
          <Button asChild variant="ghost" onClick={() => setOpen(false)}>
            <Link href="/account/settings">
              <KeyRound />
              账户与安全
            </Link>
          </Button>
          <Button variant="ghost" onClick={toggleTheme}>
            <Moon />
            模式切换
          </Button>
          {visibleManagementRoutes.length > 0 && (
            <>
              {visibleManagementRoutes.map((route) => (
                <Button
                  key={route.href}
                  asChild
                  variant="ghost"
                  onClick={() => setOpen(false)}
                >
                  <Link href={route.href}>
                    <ShieldCheck />
                    {route.title}
                  </Link>
                </Button>
              ))}
            </>
          )}
          <Button
            variant="ghost"
            disabled={auth.signingOut}
            onClick={() => void logout()}
          >
            <LogOut />
            {auth.signingOut ? "正在退出" : "退出登录"}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
