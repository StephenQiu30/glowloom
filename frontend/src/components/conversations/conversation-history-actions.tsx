"use client";

import { useRef, useState } from "react";
import { Ellipsis, Pencil, Pin, PinOff, Trash2 } from "lucide-react";
import { updateAgentSession } from "@/api/agent";
import { readErrorMessage } from "@/lib/read-error";
import { RequestError } from "@/lib/request";
import { SidebarMenuAction, SidebarMenuItem } from "@/components/ui/sidebar";
import { SidebarNavLink } from "@/components/layout/sidebar-nav-link";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function ConversationHistoryItem({
  item,
  projectID,
  active,
  writable,
  onNavigate,
  onChange,
}: {
  item: API.SessionView;
  projectID: string;
  active: boolean;
  writable: boolean;
  onNavigate: () => void;
  onChange: (
    action: API.UpdateSessionRequest["action"],
    updated: API.SessionView,
  ) => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [rename, setRename] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const menu = useRef<HTMLButtonElement>(null);
  const mutation = useRef(false);
  const restoreFocus = useRef(false);
  async function update(
    action: API.UpdateSessionRequest["action"],
    title?: string,
  ) {
    if (mutation.current) return;
    mutation.current = true;
    setPending(true);
    setError("");
    try {
      const updated = await updateAgentSession(
        { pid: projectID, sid: item.id },
        { action, ...(title === undefined ? {} : { title }) },
      );
      setRename(null);
      setDeleting(false);
      onChange(action, updated);
    } catch (e) {
      setError(
        e instanceof RequestError && e.code === "session_has_active_run"
          ? "对话仍有进行中或待核对的任务，请处理后再删除。"
          : readErrorMessage(e),
      );
    } finally {
      mutation.current = false;
      setPending(false);
    }
  }
  function closeDelete() {
    if (mutation.current) return;
    setDeleting(false);
    setError("");
  }
  function edit(input: HTMLInputElement | null) {
    if (!input || input === document.activeElement) return;
    input.focus();
    input.select();
  }
  const href = `/conversations/${item.id}?projectId=${projectID}`;
  const title = rename?.trim();
  return (
    <>
      {rename !== null ? (
        <SidebarMenuItem>
          <form
            className="flex min-h-8 items-center px-2"
            onSubmit={(event) => {
              event.preventDefault();
              if (!title || Array.from(title).length > 240) {
                setError("请输入 1–240 个字符的对话名称。");
                return;
              }
              if (title === item.title) setRename(null);
              else void update("rename", title);
            }}
          >
            <Input
              ref={edit}
              aria-label="请输入名称"
              className="h-7 min-w-0 text-sm"
              value={rename}
              readOnly={pending}
              onChange={(event) => setRename(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Escape" && !pending) {
                  event.stopPropagation();
                  setRename(null);
                  setError("");
                  requestAnimationFrame(() => menu.current?.focus());
                }
              }}
            />
            <Button type="submit" className="sr-only" tabIndex={-1}>
              保存名称
            </Button>
          </form>
        </SidebarMenuItem>
      ) : (
        <SidebarNavLink
          href={href}
          label={item.title}
          icon={item.pinned ? Pin : undefined}
          active={active}
          variant="history"
          size="history"
          onNavigate={onNavigate}
          actions={
            writable ? (
              <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuAction
                    ref={menu}
                    aria-label={`更多：${item.title}`}
                    showOnHover
                    disabled={pending}
                  >
                    <Ellipsis />
                  </SidebarMenuAction>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  side="right"
                  align="start"
                  onCloseAutoFocus={(event) => {
                    if (restoreFocus.current) {
                      event.preventDefault();
                      restoreFocus.current = false;
                    }
                  }}
                >
                  <DropdownMenuItem
                    onSelect={() => {
                      setMenuOpen(false);
                      void update(item.pinned ? "unpin" : "pin");
                    }}
                  >
                    {item.pinned ? <PinOff /> : <Pin />}
                    {item.pinned ? "取消置顶" : "置顶"}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() => {
                      setMenuOpen(false);
                      restoreFocus.current = true;
                      setError("");
                      setRename(item.title);
                    }}
                  >
                    <Pencil />
                    重命名
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setMenuOpen(false);
                      restoreFocus.current = true;
                      setError("");
                      setDeleting(true);
                    }}
                  >
                    <Trash2 />
                    删除
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : undefined
          }
        />
      )}
      {error && !deleting && (
        <li role="alert" className="px-2 text-xs text-destructive">
          {error}
        </li>
      )}
      <AlertDialog
        open={deleting}
        onOpenChange={(open) => {
          if (!open) closeDelete();
        }}
      >
        <AlertDialogContent
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            menu.current?.focus();
          }}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>删除对话</AlertDialogTitle>
            <AlertDialogDescription>
              确定删除「{item.title}」？
            </AlertDialogDescription>
          </AlertDialogHeader>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={pending} onClick={closeDelete}>
              取消
            </AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={pending}
              onClick={() => void update("delete")}
            >
              {pending ? "正在删除…" : "删除"}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
