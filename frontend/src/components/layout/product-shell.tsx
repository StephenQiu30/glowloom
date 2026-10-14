"use client";
import {
  createContext,
  useContext,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import Link from "next/link";
import { cn } from "cn";
import { useRouter } from "next/navigation";
import {
  Bell,
  Contact,
  Compass,
  Folder,
  Gift,
  KeyRound,
  LogOut,
  Menu,
  PanelLeftClose,
  Plus,
  Shield,
  Sparkles,
  UserRound,
} from "lucide-react";
import { Brand } from "./brand";
import {
  SessionGate,
  useOptionalSession,
  useSessionUpdate,
  revokeSession,
} from "@/components/auth/session";
import { AuthDialog, type AuthMode } from "@/components/auth/auth-dialog";
import { UnavailableDialog } from "@/components/availability/unavailable";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  useSidebar,
} from "@/components/ui/sidebar";
import type { Screen } from "./routes";
const adminScreens = ["users", "models", "providers", "audit", "health"];
const links = [
  { id: "home", label: "创作", icon: Sparkles, href: "/" },
  { id: "explore", label: "探索", icon: Compass, href: "/explore" },
  { id: "assets", label: "资产", icon: Folder, href: "/assets" },
  { id: "subjects", label: "主体", icon: Contact, href: "/subjects" },
];
const LoginContext = createContext<() => void>(() => {});
export function useLoginDialog() {
  return useContext(LoginContext);
}
function Navigation({
  screen,
  mobile = false,
}: {
  screen: string;
  mobile?: boolean;
}) {
  const { setOpenMobile } = useSidebar();
  return (
    <nav
      aria-label={mobile ? "移动主导航" : "主导航"}
      className={cn(
        mobile && "mobile-navigation",
        !mobile && "primary-navigation",
      )}
    >
      <SidebarMenu>
        {links.map(({ id, label, icon: Icon, href }) => (
          <SidebarMenuItem key={id}>
            <SidebarMenuButton
              asChild
              isActive={
                screen === id || (id === "assets" && screen === "projects")
              }
            >
              <Link
                href={href}
                onClick={() => setOpenMobile(false)}
                aria-current={
                  screen === id || (id === "assets" && screen === "projects")
                    ? "page"
                    : undefined
                }
              >
                <Icon data-icon="inline-start" />
                <span>{label}</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        ))}
      </SidebarMenu>
    </nav>
  );
}
function ProductNavigation({
  screen,
  onLogin,
}: {
  screen: string;
  onLogin: () => void;
}) {
  const session = useOptionalSession();
  const { setOpen, setOpenMobile, isMobile } = useSidebar();
  return (
    <Sidebar collapsible="offcanvas" className="border-0">
      <SidebarHeader className="sidebar-brand">
        <Brand wordmark />
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="收起导航"
          onClick={() => (isMobile ? setOpenMobile(false) : setOpen(false))}
        >
          <PanelLeftClose data-icon="inline-start" />
        </Button>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup className="p-0">
          <SidebarGroupContent>
            <Navigation screen={screen} />
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup className="sidebar-history">
          <SidebarGroupLabel>对话</SidebarGroupLabel>
          <SidebarGroupAction asChild>
            <Link href="/" aria-label="新建创作">
              <Plus />
            </Link>
          </SidebarGroupAction>
          <SidebarGroupContent>
            <p>暂无历史对话</p>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup className="sidebar-history">
          <SidebarGroupLabel>画布</SidebarGroupLabel>
          <SidebarGroupAction asChild>
            <Link href="/projects" aria-label="我的画布">
              <Plus />
            </Link>
          </SidebarGroupAction>
          <SidebarGroupContent>
            <Link href="/projects">查看我的画布</Link>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="sidebar-footer">
        {!session && (
          <>
            <p>登录后保存你的创作</p>
            <Button variant="ghost" onClick={onLogin}>
              登录 / 注册
            </Button>
          </>
        )}
        <Link className="sidebar-legal" href="/licenses">
          第三方许可
        </Link>
      </SidebarFooter>
    </Sidebar>
  );
}
function Shell({
  screen,
  children,
  fullBleed,
  initialAuthMode,
}: {
  screen: string;
  children: ReactNode;
  fullBleed?: boolean;
  initialAuthMode?: AuthMode;
}) {
  const session = useOptionalSession();
  const updateSession = useSessionUpdate();
  const router = useRouter();
  const { open, setOpen, isMobile, setOpenMobile } = useSidebar();
  const [feature, setFeature] = useState<string | null>(null);
  const [auth, setAuth] = useState(Boolean(initialAuthMode));
  const [logout, setLogout] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  async function leave() {
    if (pending || !session) return;
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
    <LoginContext.Provider value={() => setAuth(true)}>
      <a className="sr-only focus:not-sr-only" href="#main-content">
        跳转到主要内容
      </a>
      <ProductNavigation screen={screen} onLogin={() => setAuth(true)} />
      <div className="product-content">
        <header className="product-header">
          {isMobile && <Brand wordmark />}
          {!open && !isMobile && (
            <Button
              variant="ghost"
              size="icon"
              aria-label="展开导航"
              onClick={() => setOpen(true)}
            >
              <Menu />
            </Button>
          )}
          <div className="account-controls">
            {session ? (
              <>
                <Button
                  variant="outline"
                  size="lg"
                  onClick={() => setFeature("积分商城")}
                >
                  <Gift data-icon="inline-start" />
                  积分商城
                </Button>
                <div className="account-cluster">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="消息中心"
                    onClick={() => setFeature("消息中心")}
                  >
                    <Bell />
                  </Button>
                  <Button variant="ghost" onClick={() => setFeature("会员")}>
                    开通会员
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="账号菜单"
                      >
                        <Avatar className="size-6">
                          <AvatarFallback>
                            {Array.from(session.actor.display_name)[0]}
                          </AvatarFallback>
                        </Avatar>
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-60">
                      <DropdownMenuGroup>
                        <DropdownMenuLabel>
                          {session.actor.display_name}
                          <p className="mt-1 text-xs font-normal text-muted-foreground">
                            {session.actor.login_name}
                          </p>
                        </DropdownMenuLabel>
                      </DropdownMenuGroup>
                      <DropdownMenuSeparator />
                      <DropdownMenuGroup>
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
                      </DropdownMenuGroup>
                      <DropdownMenuSeparator />
                      <DropdownMenuGroup>
                        <DropdownMenuItem onSelect={() => setLogout(true)}>
                          <LogOut />
                          退出登录
                        </DropdownMenuItem>
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </>
            ) : (
              <>
                <span className="guest-label">游客模式</span>
                <Button size="lg" onClick={() => setAuth(true)}>
                  登录 / 注册
                </Button>
              </>
            )}
          </div>
        </header>
        {isMobile && <Navigation screen={screen} mobile />}
        <main id="main-content" className={cn(!fullBleed && "workspace-page")}>
          {children}
        </main>
        {isMobile && (
          <div className="mobile-footer">
            <Button variant="ghost" onClick={() => setOpenMobile(true)}>
              <Menu data-icon="inline-start" />
              更多导航
            </Button>
          </div>
        )}
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
      <AuthDialog
        open={auth}
        initialMode={initialAuthMode}
        onOpenChange={(value) => {
          setAuth(value);
          if (!value && initialAuthMode) router.replace("/");
        }}
        onAuthenticated={(value) => {
          updateSession(value);
          setAuth(false);
          if (initialAuthMode) router.replace("/");
        }}
      />
      <Dialog
        open={logout}
        onOpenChange={(value) => {
          if (!pending) setLogout(value);
        }}
      >
        <DialogContent>
          <DialogTitle>退出登录？</DialogTitle>
          <DialogDescription>
            将结束当前设备的登录会话。已保存的画布仍保留在你的账号中。
          </DialogDescription>
          {error && (
            <Alert variant="danger">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
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
    </LoginContext.Provider>
  );
}
export function ProductShell({
  screen,
  children,
  fullBleed = false,
  allowGuest = false,
  initialAuthMode,
}: {
  screen: Screen;
  children: ReactNode;
  fullBleed?: boolean;
  contextual?: boolean;
  allowGuest?: boolean;
  initialAuthMode?: AuthMode;
}) {
  return (
    <SessionGate
      admin={adminScreens.includes(screen)}
      allowGuest={allowGuest && (screen === "home" || screen === "explore")}
    >
      <SidebarProvider
        className="product-shell"
        style={{ "--sidebar-width": "240px" } as CSSProperties}
      >
        <Shell
          screen={screen}
          fullBleed={fullBleed}
          initialAuthMode={initialAuthMode}
        >
          {children}
        </Shell>
      </SidebarProvider>
    </SessionGate>
  );
}
