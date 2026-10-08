"use client";

import { useSyncExternalStore } from "react";
import { useTheme } from "next-themes";
import { Monitor, Moon, Sun } from "lucide-react";
import {
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
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

export function ThemeMenuItems() {
  const { theme, hydrated, setTheme } = useThemeSelection();
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
            disabled={!hydrated}
            className="h-9"
          >
            <Icon />
            {label}
          </DropdownMenuRadioItem>
        ))}
      </DropdownMenuRadioGroup>
    </>
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
