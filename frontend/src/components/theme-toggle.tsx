"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";

import { Button } from "@/components/ui/button";

export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label="切换明暗主题"
      title="切换明暗主题"
      onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
    >
      <Moon
        aria-hidden="true"
        data-icon="inline-start"
        className="dark:hidden"
      />
      <Sun
        aria-hidden="true"
        data-icon="inline-start"
        className="hidden dark:block"
      />
    </Button>
  );
}
