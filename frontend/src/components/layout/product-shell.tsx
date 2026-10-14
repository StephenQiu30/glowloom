"use client";
import {
  createContext,
  useContext,
  useState,
  useRef,
  useSyncExternalStore,
  type CSSProperties,
  type ReactNode,
} from "react";
import Link from "next/link";
import { cn } from "cn";
import { useRouter } from "next/navigation";
import {
  Bell,
  Folder,
  Gift,
  KeyRound,
  LogOut,
  Menu,
  Shield,
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
import { SidebarProvider, useSidebar } from "@/components/ui/sidebar";
import { Navigation, ProductNavigation } from "./product-navigation";
import type { Screen } from "./routes";
const adminScreens = ["users", "models", "providers", "audit", "health"];
const LoginContext = createContext<() => void>(() => {});
export function useLoginDialog() {
  return useContext(LoginContext);
}
const sidebarPreferenceEvent = "glowloom:sidebar-preference";
function subscribeSidebarPreference(onChange: () => void) {
  window.addEventListener(sidebarPreferenceEvent, onChange);
  return () => window.removeEventListener(sidebarPreferenceEvent, onChange);
}
function getSidebarPreference() {
  return !document.cookie.split("; ").includes("sidebar_state=false");
}
function setSidebarPreference(open: boolean) {
  document.cookie = `sidebar_state=${open}; path=/; max-age=604800; SameSite=Lax`;
  window.dispatchEvent(new Event(sidebarPreferenceEvent));
}
function getServerSidebarPreference() {
  return true;
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
  const { isMobile, openMobile, setOpenMobile } = useSidebar();
  const mobileNavigationTrigger = useRef<HTMLButtonElement>(null);
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
      <ProductNavigation
        screen={screen}
        onLogin={() => setAuth(true)}
        onMobileCloseAutoFocus={(event) => {
          event.preventDefault();
          if (!auth) mobileNavigationTrigger.current?.focus();
        }}
      />
      <div className="product-content">
        <header className="product-header">
          {isMobile && (
            <>
              <Button
                variant="ghost"
                size="icon"
                ref={mobileNavigationTrigger}
                className="mobile-navigation-trigger"
                aria-label="打开导航"
                aria-expanded={openMobile}
                onClick={() => setOpenMobile(true)}
              >
                <Menu />
              </Button>
              <Brand wordmark />
            </>
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
              <Button size="lg" onClick={() => setAuth(true)}>
                登录 / 注册
              </Button>
            )}
          </div>
        </header>
        {isMobile && <Navigation screen={screen} mobile />}
        <main id="main-content" className={cn(!fullBleed && "workspace-page")}>
          {children}
        </main>
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
      <AuthDialog
        fallbackFocusRef={mobileNavigationTrigger}
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
  const sidebarOpen = useSyncExternalStore(
    subscribeSidebarPreference,
    getSidebarPreference,
    getServerSidebarPreference,
  );
  return (
    <SessionGate
      admin={adminScreens.includes(screen)}
      allowGuest={allowGuest && (screen === "home" || screen === "explore")}
    >
      <SidebarProvider
        className="product-shell"
        open={sidebarOpen}
        onOpenChange={setSidebarPreference}
        style={
          {
            "--sidebar-width": "240px",
            "--sidebar-width-icon": "64px",
          } as CSSProperties
        }
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
