"use client";
import { useState, type ReactNode } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Sparkles,
  Compass,
  Folder,
  Contact,
  Plus,
  PanelLeftClose,
  Menu,
  Bell,
  Gift,
  LogOut,
  UserRound,
  Shield,
  KeyRound,
} from "lucide-react";
import { Brand } from "./brand";
import { Button } from "@/components/ui/button";
import {
  SessionGate,
  useSession,
  revokeSession,
} from "@/components/auth/session";
import { UnavailableDialog } from "@/components/availability/unavailable";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import type { Screen } from "./routes";
const adminScreens = ["users", "models", "providers", "audit", "health"];
function Navigation({ screen, close }: { screen: string; close?: () => void }) {
  return (
    <>
      <div className="sidebar-brand">
        <Brand wordmark />
        {close && (
          <Button
            variant="ghost"
            size="icon"
            aria-label="收起导航"
            onClick={close}
          >
            <PanelLeftClose />
          </Button>
        )}
      </div>
      <nav aria-label="主导航" className="primary-navigation">
        {[
          ["home", "创作", Sparkles, "/"],
          ["explore", "探索", Compass, "/explore"],
          ["assets", "资产", Folder, "/assets"],
          ["subjects", "主体", Contact, "/subjects"],
        ].map(([id, label, Icon, href]) => {
          const Symbol = Icon as typeof Sparkles;
          return (
            <Link
              key={String(id)}
              href={String(href)}
              onClick={close}
              aria-current={
                screen === id || (id === "assets" && screen === "projects")
                  ? "page"
                  : undefined
              }
            >
              <Symbol size={20} strokeWidth={1.65} />
              <span>{String(label)}</span>
            </Link>
          );
        })}
      </nav>
      <div className="sidebar-history">
        <div>
          对话
          <Button variant="ghost" size="icon-sm" aria-label="新建创作" asChild>
            <Link href="/" onClick={close}>
              <Plus size={14} />
            </Link>
          </Button>
        </div>
        <p>暂无历史对话</p>
        <div>
          画布
          <Button variant="ghost" size="icon-sm" aria-label="我的画布" asChild>
            <Link href="/projects" onClick={close}>
              <Plus size={14} />
            </Link>
          </Button>
        </div>
        <Link href="/projects" onClick={close}>
          查看我的画布
        </Link>
      </div>
      <Link className="sidebar-legal" href="/licenses">
        第三方许可
      </Link>
    </>
  );
}
function Shell({
  screen,
  children,
  fullBleed,
}: {
  screen: string;
  children: ReactNode;
  fullBleed?: boolean;
}) {
  const session = useSession();
  const router = useRouter();
  const [collapsed, setCollapsed] = useState(false);
  const [mobile, setMobile] = useState(false);
  const [feature, setFeature] = useState<string | null>(null);
  const [logout, setLogout] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  async function leave() {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      await revokeSession(session);
      router.replace("/login");
    } catch (e) {
      setError(e instanceof Error ? e.message : "退出结果未确认，请重试。");
    } finally {
      setPending(false);
    }
  }
  return (
    <div className={`product-shell ${collapsed ? "sidebar-collapsed" : ""}`}>
      <a className="sr-only focus:not-sr-only" href="#main-content">
        跳转到主要内容
      </a>
      <aside className="product-sidebar">
        <Navigation screen={screen} close={() => setCollapsed(true)} />
      </aside>
      <Sheet open={mobile} onOpenChange={setMobile}>
        <SheetContent side="left" className="w-64 bg-sidebar p-0">
          <SheetTitle className="sr-only">导航</SheetTitle>
          <Navigation screen={screen} close={() => setMobile(false)} />
        </SheetContent>
      </Sheet>
      <div className="product-content">
        <header className="product-header">
          <Button
            className="mobile-menu"
            variant="ghost"
            size="icon"
            aria-label="打开导航"
            onClick={() => setMobile(true)}
          >
            <Menu />
          </Button>
          {collapsed && (
            <Button
              className="desktop-menu"
              variant="ghost"
              size="icon"
              aria-label="展开导航"
              onClick={() => setCollapsed(false)}
            >
              <Menu />
            </Button>
          )}
          <div className="account-controls">
            <Button variant="outline" onClick={() => setFeature("积分商城")}>
              <Gift size={16} />
              积分商城
            </Button>
            <div className="account-cluster">
              <Button
                variant="ghost"
                size="icon"
                aria-label="消息中心"
                onClick={() => setFeature("消息中心")}
              >
                <Bell size={17} />
              </Button>
              <Button
                variant="ghost"
                className="text-sky-400"
                onClick={() => setFeature("会员")}
              >
                <Sparkles size={14} />
                开通会员
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" aria-label="账号菜单">
                    <span className="account-avatar">
                      {Array.from(session.actor.display_name)[0]}
                    </span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-60">
                  <DropdownMenuLabel>
                    {session.actor.display_name}
                    <p className="mt-1 text-xs font-normal text-muted-foreground">
                      {session.actor.login_name}
                    </p>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem asChild>
                    <Link href="/account">
                      <UserRound />
                      账号设置
                    </Link>
                  </DropdownMenuItem>
                  <DropdownMenuItem asChild>
                    <Link href="/assets">
                      <Folder />
                      我的资产
                    </Link>
                  </DropdownMenuItem>
                  <DropdownMenuItem asChild>
                    <Link href="/reset-password">
                      <KeyRound />
                      修改密码
                    </Link>
                  </DropdownMenuItem>
                  {session.actor.role === "admin" && (
                    <DropdownMenuItem asChild>
                      <Link href="/users">
                        <Shield />
                        用户管理
                      </Link>
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onSelect={() => setLogout(true)}>
                    <LogOut />
                    退出登录
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </header>
        <main id="main-content" className={fullBleed ? "" : "workspace-page"}>
          {children}
        </main>
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
      <Dialog
        open={logout}
        onOpenChange={(open) => {
          if (!pending) setLogout(open);
        }}
      >
        <DialogContent>
          <DialogTitle>退出登录？</DialogTitle>
          <DialogDescription>
            将结束当前设备的登录会话。已保存的画布仍保留在你的账号中。
          </DialogDescription>
          {error && <p role="alert">{error}</p>}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setLogout(false)}
            >
              取消
            </Button>
            <Button disabled={pending} onClick={() => void leave()}>
              {pending ? "正在退出…" : "退出登录"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
export function ProductShell({
  screen,
  children,
  fullBleed = false,
}: {
  screen: Screen;
  children: ReactNode;
  fullBleed?: boolean;
  contextual?: boolean;
}) {
  return (
    <SessionGate admin={adminScreens.includes(screen)}>
      <Shell screen={screen} fullBleed={fullBleed}>
        {children}
      </Shell>
    </SessionGate>
  );
}
