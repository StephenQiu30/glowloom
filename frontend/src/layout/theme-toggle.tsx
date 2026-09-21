"use client";

import { Moon, Sun, SunMoon } from "lucide-react";
import { useTheme } from "next-themes";
import { useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";

const subscribeToHydration = () => () => undefined;
const getClientHydrationSnapshot = () => true;
const getServerHydrationSnapshot = () => false;

export function ThemeToggle() {
  const mounted = useSyncExternalStore(
    subscribeToHydration,
    getClientHydrationSnapshot,
    getServerHydrationSnapshot,
  );
  const { resolvedTheme, setTheme } = useTheme();

  const dark = resolvedTheme === "dark";
  const CurrentIcon = !mounted ? SunMoon : dark ? Sun : Moon;
  const title = !mounted ? "切换主题" : dark ? "切换为浅色模式" : "切换为深色模式";

  return (
    <Button
      aria-label="切换主题"
      disabled={!mounted}
      onClick={() => setTheme(dark ? "light" : "dark")}
      size="icon"
      title={title}
      variant="ghost"
    >
      <CurrentIcon aria-hidden="true" />
    </Button>
  );
}
