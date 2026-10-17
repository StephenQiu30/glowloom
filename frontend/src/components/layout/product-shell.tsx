"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  createContext,
  useContext,
  useState,
  useRef,
  useSyncExternalStore,
  type CSSProperties,
  type ReactNode,
} from "react";
import { cn } from "cn";
import { useRouter } from "next/navigation";
import { Bell, Gift, Menu } from "lucide-react";
import {
  SpaceProvider,
  SpaceSwitcher,
} from "@/components/spaces/space-context";
import { AccountMenu } from "@/components/account/account-menu";
import { Brand } from "@/components/brand/brand";
import {
  SessionGate,
  useOptionalSession,
  useSessionUpdate,
  revokeSession,
} from "@/components/auth/session";
import { AuthDialog, type AuthMode } from "@/components/auth/auth-dialog";
import { UnavailableDialog } from "@/components/availability/unavailable";
import { Button } from "@/components/ui/button";
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
const adminScreens = [
  "users",
  "models",
  "providers",
  "audit",
  "health",
  "platform/teams",
  "platform/settings",
];
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
                  <AccountMenu
                    actor={session.actor}
                    onLogout={() => setLogout(true)}
                  />
                </div>
              </>
            ) : (
              <Button size="lg" onClick={() => setAuth(true)}>
                登录 / 注册
              </Button>
            )}
          </div>
        </header>
        {session ? (
          <div className="border-b px-4 py-2">
            <SpaceSwitcher />
          </div>
        ) : null}
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
          {error && <FeedbackToast message={error} />}
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
      allowGuest={
        allowGuest &&
        (screen === "home" || screen === "explore" || screen === "invitations")
      }
    >
      <SpaceProvider>
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
      </SpaceProvider>
    </SessionGate>
  );
}
