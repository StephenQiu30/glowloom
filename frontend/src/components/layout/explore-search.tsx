"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Search, Trash2 } from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupButton,
} from "@/components/ui/input-group";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
} from "@/components/ui/popover";
import type { PageSearch } from "./product-search";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";

function readHistory(key: string): string[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(key) ?? "[]");
    return Array.isArray(value)
      ? [
          ...new Set(
            value
              .filter((v): v is string => typeof v === "string" && !!v.trim())
              .map((v) => v.trim().slice(0, 800)),
          ),
        ].slice(0, 20)
      : [];
  } catch {
    return [];
  }
}

export function ExploreSearch({ search }: { search: PageSearch }) {
  const session = useOptionalSession();
  const owner = session?.actor.id ?? "guest";
  return (
    <ExploreSearchControl
      key={owner}
      search={search}
      storageKey={`glowloom:explore-search:${owner}`}
    />
  );
}

function ExploreSearchControl({
  search,
  storageKey,
}: {
  search: PageSearch;
  storageKey: string;
}) {
  const [expanded, setExpanded] = useState(
    Boolean(search.alwaysExpanded || search.value),
  );
  const [open, setOpen] = useState(false);
  const [history, setHistory] = useState<string[]>([]);
  const input = useRef<HTMLInputElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panelId = useId();
  const inputId = useId();
  function show() {
    setExpanded(true);
    setHistory(readHistory(storageKey));
    setOpen(true);
    requestAnimationFrame(() => input.current?.focus());
  }
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
      setHistory(readHistory(storageKey));
      setOpen(true);
      requestAnimationFrame(() => input.current?.focus());
    }
    document.addEventListener("keydown", keydown);
    return () => document.removeEventListener("keydown", keydown);
  }, [search.disabled, storageKey]);
  function save(next: string[]) {
    setHistory(next);
    try {
      localStorage.setItem(storageKey, JSON.stringify(next));
    } catch {
      /* Search remains available if storage is blocked. */
    }
  }
  function submit(value: string) {
    const query = value.trim();
    if (!query || search.disabled) return;
    save(
      [
        query,
        ...readHistory(storageKey).filter((item) => item !== query),
      ].slice(0, 20),
    );
    search.onValueChange(query);
    setOpen(false);
    search.onSearch(query);
  }
  if (!expanded)
    return (
      <Button
        ref={trigger}
        variant="outline"
        size="toolbar-icon"
        aria-label={search.label}
        aria-expanded={false}
        aria-keyshortcuts="Meta+K Control+K"
        onClick={show}
        disabled={search.disabled}
      >
        <Search />
      </Button>
    );
  return (
    <Popover
      open={open && !search.value.trim() && history.length > 0}
      onOpenChange={setOpen}
    >
      <PopoverAnchor asChild>
        <form
          role="search"
          aria-label={search.label}
          className="w-74 max-w-full min-w-0 flex-none"
          data-explore-search
          onSubmit={(event) => {
            event.preventDefault();
            submit(search.value);
          }}
        >
          <FieldGroup>
            <Field data-disabled={search.disabled || undefined}>
              <FieldLabel htmlFor={inputId} className="sr-only">
                {search.label}
              </FieldLabel>
              <InputGroup>
                <InputGroupAddon>
                  <Search aria-hidden="true" />
                </InputGroupAddon>
                <InputGroupInput
                  ref={input}
                  id={inputId}
                  aria-label={search.label}
                  aria-expanded={
                    open && !search.value.trim() && history.length > 0
                  }
                  aria-controls={panelId}
                  placeholder={search.placeholder ?? "搜索"}
                  value={search.value}
                  maxLength={800}
                  disabled={search.disabled}
                  onChange={(event) => {
                    search.onValueChange(event.target.value);
                    setOpen(true);
                  }}
                  onFocus={() => {
                    setHistory(readHistory(storageKey));
                    setOpen(true);
                  }}
                  onBlur={(event) => {
                    if (
                      !search.value &&
                      !search.alwaysExpanded &&
                      !(
                        event.relatedTarget instanceof Element &&
                        event.relatedTarget.closest(
                          "[data-explore-search], [data-explore-search-history]",
                        )
                      )
                    )
                      setExpanded(false);
                  }}
                  onKeyDown={(event) => {
                    if (
                      event.key === "Enter" &&
                      (event.nativeEvent.isComposing || event.keyCode === 229)
                    )
                      event.preventDefault();
                    if (event.key === "Escape") {
                      event.preventDefault();
                      setOpen(false);
                      if (!search.value && !search.alwaysExpanded) {
                        setExpanded(false);
                        requestAnimationFrame(() => trigger.current?.focus());
                      }
                    }
                  }}
                />
                <InputGroupAddon align="inline-end">
                  <InputGroupButton
                    type="submit"
                    variant="secondary"
                    size="xs"
                    disabled={search.disabled}
                  >
                    搜索
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </Field>
          </FieldGroup>
        </form>
      </PopoverAnchor>
      <PopoverContent
        id={panelId}
        align="start"
        sideOffset={8}
        className="w-[var(--radix-popover-trigger-width)] max-w-[calc(100vw-1.5rem)]"
        data-explore-search-history
        aria-label="搜索历史"
        onOpenAutoFocus={(event) => event.preventDefault()}
        onCloseAutoFocus={(event) => event.preventDefault()}
        onInteractOutside={(event) => {
          if (
            event.target instanceof Element &&
            event.target.closest("[data-explore-search]")
          ) {
            event.preventDefault();
            return;
          }
          if (!search.value && !search.alwaysExpanded) setExpanded(false);
        }}
      >
        <PopoverHeader className="flex-row items-center justify-between">
          <PopoverTitle>搜索历史</PopoverTitle>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              save([]);
              setOpen(false);
              input.current?.focus();
            }}
          >
            <Trash2 data-icon="inline-start" />
            清空
          </Button>
        </PopoverHeader>
        <div className="flex flex-wrap gap-2">
          {history.map((item) => (
            <Button
              key={item}
              className="max-w-full"
              variant="secondary"
              size="sm"
              onClick={() => submit(item)}
            >
              <span className="truncate">{item}</span>
            </Button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
