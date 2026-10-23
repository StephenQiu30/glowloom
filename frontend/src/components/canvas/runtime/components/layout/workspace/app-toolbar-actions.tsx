"use client";

import { Button } from "@/components/ui/button";
import { Keyboard, Moon, Puzzle, Sun } from "lucide-react";
import type { CSSProperties } from "react";

import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";

type AppToolbarActionsProps = {
  variant?: "default" | "canvas";
  onOpenShortcuts?: () => void;
  onOpenPlugins?: () => void;
};

export function AppToolbarActions({
  variant = "default",
  onOpenShortcuts,
  onOpenPlugins,
}: AppToolbarActionsProps) {
  const theme = useThemeStore((state) => state.theme);
  const toggleTheme = useThemeStore((state) => state.toggleTheme);
  const canvasTheme = canvasThemes[theme];
  const iconStyle: CSSProperties | undefined =
    variant === "canvas" ? { color: canvasTheme.node.text } : undefined;

  return (
    <div className="inline-flex shrink-0 items-center gap-1">
      {onOpenPlugins ? (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          style={iconStyle}
          onClick={onOpenPlugins}
          aria-label={"节点插件"}
          title={"节点插件"}
        >
          <Puzzle />
        </Button>
      ) : null}
      <Button
        type="button"
        variant="ghost"
        size="icon-lg"
        className="transition-none active:translate-y-0"
        style={iconStyle}
        onClick={toggleTheme}
        aria-label="切换主题"
        title="切换主题"
      >
        <Sun className="hidden dark:block" />
        <Moon className="dark:hidden" />
      </Button>
      {onOpenShortcuts ? (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          style={iconStyle}
          onClick={onOpenShortcuts}
          aria-label={"快捷键"}
          title={"快捷键"}
        >
          <Keyboard />
        </Button>
      ) : null}
    </div>
  );
}
