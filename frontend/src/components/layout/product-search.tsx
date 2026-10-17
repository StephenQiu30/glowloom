"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";
import { Search, X, ArrowUpRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Command,
  CommandDialog,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command";
import { screens, screenHref } from "./routes";

export type PageSearch = {
  value: string;
  onValueChange: (value: string) => void;
  onSearch: (value: string) => void;
  label: string;
  placeholder?: string;
  disabled?: boolean;
};
const destinations = screens.filter((screen) =>
  ["home", "explore", "assets", "subjects", "projects"].includes(screen.id),
);
const subscribePlatform = () => () => {};
const getPlatformKey = () =>
  /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl";
const getServerPlatformKey = () => "Ctrl";

/** One command palette per BasicLayout; result data stays in its owning page. */
export function ProductSearch({
  search,
  isMobile = false,
}: {
  search?: PageSearch;
  isMobile?: boolean;
}) {
  const router = useRouter();
  const [value, setValue] = useState("");
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const modifier = useSyncExternalStore(
    subscribePlatform,
    getPlatformKey,
    getServerPlatformKey,
  );
  const current = search ?? {
    value,
    onValueChange: setValue,
    label: "搜索灵感",
    onSearch: (query: string) =>
      router.push(
        query ? `/explore?q=${encodeURIComponent(query)}` : "/explore",
      ),
  };
  const disabled = Boolean(current.disabled);
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (
        event.defaultPrevented ||
        event.repeat ||
        event.isComposing ||
        event.altKey ||
        !(event.metaKey || event.ctrlKey) ||
        event.key.toLowerCase() !== "k"
      )
        return;
      if (
        disabled ||
        (!open &&
          document.querySelector('[role="dialog"], [role="alertdialog"]')) ||
        (event.target instanceof Element &&
          event.target.closest('input[type="password"]'))
      )
        return;
      event.preventDefault();
      event.stopPropagation();
      if (!open)
        returnFocusRef.current =
          document.activeElement instanceof HTMLElement
            ? document.activeElement
            : null;
      setOpen((value) => !value);
    }
    document.addEventListener("keydown", onKeyDown, true);
    return () => document.removeEventListener("keydown", onKeyDown, true);
  }, [disabled, open]);
  function submit(query: string) {
    if (disabled) return;
    current.onSearch(query.trim());
    setOpen(false);
  }
  const matches = destinations.filter((screen) =>
    screen.name.includes(current.value.trim()),
  );
  return (
    <>
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-lg"
              className={isMobile ? "size-11" : undefined}
              aria-label={current.label}
              aria-haspopup="dialog"
              aria-expanded={open}
              aria-keyshortcuts="Meta+K Control+K"
              disabled={disabled}
              ref={triggerRef}
              onClick={(event) => {
                returnFocusRef.current = event.currentTarget;
                setOpen(true);
              }}
            >
              <Search />
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {current.label}（{modifier}+K）
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title={current.label}
        description="输入关键词后搜索，或选择页面跳转。"
        showCloseButton
        className="top-1/2 max-h-[calc(100dvh-2rem)] -translate-y-1/2"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          const previous = returnFocusRef.current;
          if (previous?.isConnected && previous !== document.body)
            previous.focus();
          else triggerRef.current?.focus();
        }}
      >
        <Command shouldFilter={false} label={current.label}>
          <CommandInput
            aria-label={current.label}
            placeholder={current.placeholder ?? current.label}
            value={current.value}
            onValueChange={current.onValueChange}
            disabled={disabled}
            maxLength={512}
            onKeyDown={(event) => {
              if (
                event.key === "Enter" &&
                (event.nativeEvent.isComposing || event.keyCode === 229)
              )
                event.preventDefault();
            }}
          />
          <CommandList className="max-h-[min(18rem,calc(100dvh-8rem))]">
            <CommandGroup heading="搜索">
              <CommandItem
                value="page-search"
                disabled={disabled}
                onSelect={() => submit(current.value)}
                className="min-h-11"
              >
                <Search />
                {current.label}
                <CommandShortcut>↵</CommandShortcut>
              </CommandItem>
              {current.value ? (
                <CommandItem
                  value="clear-search"
                  disabled={disabled}
                  className="min-h-11"
                  onSelect={() => {
                    current.onValueChange("");
                    submit("");
                  }}
                >
                  <X />
                  清除搜索
                </CommandItem>
              ) : null}
            </CommandGroup>
            {matches.length ? (
              <CommandGroup heading="跳转">
                {matches.map((screen) => (
                  <CommandItem
                    key={screen.id}
                    value={screen.id}
                    className="min-h-11"
                    onSelect={() => {
                      setOpen(false);
                      router.push(screenHref(screen.id));
                    }}
                  >
                    <ArrowUpRight />
                    {screen.name}
                  </CommandItem>
                ))}
              </CommandGroup>
            ) : null}
          </CommandList>
        </Command>
      </CommandDialog>
    </>
  );
}
