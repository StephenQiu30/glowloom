"use client";

import { useEffect, useRef, useState } from "react";
import { Search, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { PageSearch } from "./product-search";

/** Page-owned results, inline input in the existing common header search slot. */
export function ProductInlineSearch({
  search,
  isMobile,
}: {
  search: PageSearch;
  isMobile: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const [composing, setComposing] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const applied = useRef(search.value.trim());
  const onSearch = search.onSearch;
  const query = search.value.trim();
  useEffect(() => {
    if (composing || search.disabled || query === applied.current) return;
    if (!query) {
      applied.current = "";
      onSearch("");
      return;
    }
    const timer = setTimeout(() => {
      if (applied.current !== query) {
        applied.current = query;
        onSearch(query);
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [query, composing, search.disabled, onSearch]);
  useEffect(() => {
    if (expanded) input.current?.focus();
  }, [expanded]);
  useEffect(() => {
    function keydown(event: KeyboardEvent) {
      if (
        event.defaultPrevented ||
        event.repeat ||
        event.isComposing ||
        event.altKey ||
        !(event.metaKey || event.ctrlKey) ||
        event.key.toLowerCase() !== "k" ||
        search.disabled ||
        document.querySelector('[role="dialog"], [role="alertdialog"]')
      )
        return;
      event.preventDefault();
      setExpanded(true);
      input.current?.focus();
    }
    document.addEventListener("keydown", keydown);
    return () => document.removeEventListener("keydown", keydown);
  }, [search.disabled]);
  if (!expanded && !search.value)
    return (
      <Button
        ref={trigger}
        variant="ghost"
        size="icon-lg"
        className={isMobile ? "size-11" : undefined}
        aria-label={search.label}
        aria-keyshortcuts="Meta+K Control+K"
        disabled={search.disabled}
        onClick={() => setExpanded(true)}
      >
        <Search />
      </Button>
    );
  return (
    <div
      role="search"
      aria-label={search.label}
      className="flex h-11 w-[min(240px,45vw)] min-w-0 items-center gap-1 rounded-md bg-secondary px-2"
    >
      <Search
        className="size-4 shrink-0 text-muted-foreground"
        aria-hidden="true"
      />
      <Input
        ref={input}
        className="h-full min-w-0 border-0 bg-transparent px-1 shadow-none focus-visible:ring-0"
        aria-label={search.label}
        placeholder={search.placeholder ?? "搜索"}
        value={search.value}
        maxLength={512}
        disabled={search.disabled}
        onChange={(event) => search.onValueChange(event.target.value)}
        onCompositionStart={() => setComposing(true)}
        onCompositionEnd={() => setComposing(false)}
        onBlur={(event) => {
          if (
            !search.value &&
            !event.currentTarget.parentElement?.contains(
              event.relatedTarget as Node,
            )
          )
            setExpanded(false);
        }}
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing || event.keyCode === 229) return;
          if (event.key === "Escape") {
            event.preventDefault();
            if (!search.value) {
              setExpanded(false);
              requestAnimationFrame(() => trigger.current?.focus());
            } else input.current?.blur();
          }
          if (event.key === "Enter") {
            event.preventDefault();
            applied.current = query;
            onSearch(query);
          }
        }}
      />
      {search.value ? (
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className="size-9 shrink-0 max-sm:size-11"
          aria-label="清空搜索"
          disabled={search.disabled}
          onClick={() => {
            search.onValueChange("");
            applied.current = "";
            onSearch("");
            input.current?.focus();
          }}
        >
          <X />
        </Button>
      ) : null}
    </div>
  );
}
