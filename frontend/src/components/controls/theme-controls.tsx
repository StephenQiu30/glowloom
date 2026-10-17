"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useTheme } from "next-themes";
import { Monitor, Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useOptionalSession } from "@/components/auth/session";
import { getMyPreferences, updateMyPreferences } from "@/api/identity";
import { toast } from "sonner";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

const themeOptions = [
  { value: "light", label: "浅色", icon: Sun },
  { value: "dark", label: "深色", icon: Moon },
  { value: "system", label: "跟随系统", icon: Monitor },
];

const subscribe = () => () => {};
const getSnapshot = () => true;
const getServerSnapshot = () => false;

function useThemeSelection() {
  const { theme, setTheme } = useTheme();
  // Storage is unavailable during SSR; keep the initial hydration render identical.
  const hydrated = useSyncExternalStore(
    subscribe,
    getSnapshot,
    getServerSnapshot,
  );
  return {
    theme: hydrated ? (theme ?? "dark") : "",
    hydrated,
    setTheme: (value: string) => {
      if (themeOptions.some((option) => option.value === value)) {
        setTheme(value);
      }
    },
  };
}

export function useThemeMenuSelection() {
  const selection = useThemeSelection();
  const session = useOptionalSession();
  const [pending, setPending] = useState(false);
  const flight = useRef<AbortController | null>(null);
  useEffect(() => () => flight.current?.abort(), [session?.actor.id]);
  async function changeTheme(value: string) {
    if (!selection.hydrated || flight.current || value === selection.theme)
      return;
    if (!themeOptions.some((option) => option.value === value)) return;
    if (!session) {
      selection.setTheme(value);
      return;
    }
    const controller = new AbortController();
    flight.current = controller;
    setPending(true);
    try {
      const current = await getMyPreferences({ signal: controller.signal });
      if (controller.signal.aborted) return;
      if (current.theme !== value)
        await updateMyPreferences(
          {
            expected_revision: current.revision,
            theme: value,
            prompt_suffix: current.prompt_suffix,
          },
          { signal: controller.signal },
        );
      const saved = await getMyPreferences({ signal: controller.signal });
      if (controller.signal.aborted) return;
      if (saved.theme !== value) throw new Error("theme_conflict");
      selection.setTheme(saved.theme);
    } catch {
      if (!controller.signal.aborted) toast("主题未能保存，请重试");
    } finally {
      if (flight.current === controller) flight.current = null;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  return { ...selection, pending, setTheme: changeTheme };
}

export function ThemeMenuItems({
  selection,
}: {
  selection: ReturnType<typeof useThemeMenuSelection>;
}) {
  const { theme, hydrated, pending, setTheme } = selection;
  return (
    <>
      <DropdownMenuLabel>主题</DropdownMenuLabel>
      <DropdownMenuRadioGroup
        aria-label="主题"
        value={theme}
        onValueChange={setTheme}
      >
        {themeOptions.map(({ value, label, icon: Icon }) => (
          <DropdownMenuRadioItem
            key={value}
            value={value}
            disabled={!hydrated || pending}
            className="h-9 max-md:h-11"
          >
            <Icon />
            {label}
          </DropdownMenuRadioItem>
        ))}
      </DropdownMenuRadioGroup>
    </>
  );
}

export function ThemeToggle() {
  const selection = useThemeMenuSelection();
  const { theme, hydrated, pending } = selection;
  const Icon =
    themeOptions.find((option) => option.value === theme)?.icon ?? Moon;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-lg"
          className="max-md:size-11"
          aria-label="切换主题"
          title="切换主题"
          disabled={!hydrated || pending}
          aria-busy={pending}
        >
          {pending ? <Spinner /> : <Icon />}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        sideOffset={8}
        collisionPadding={16}
        className="w-48 max-w-[calc(100vw-2rem)]"
      >
        <ThemeMenuItems selection={selection} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function ThemePreference() {
  const { theme, hydrated, setTheme } = useThemeSelection();
  return (
    <ToggleGroup
      type="single"
      aria-label="主题偏好"
      value={theme}
      onValueChange={setTheme}
      className="flex-wrap"
      disabled={!hydrated}
    >
      {themeOptions.map(({ value, label }) => (
        <ToggleGroupItem key={value} value={value}>
          {label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
