"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ChevronDown,
  Search,
  History,
  Share2,
  MoreHorizontal,
  Save,
  Keyboard,
  UserRound,
} from "lucide-react";
import { Brand } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
} from "@/components/ui/command";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { AccountMenu } from "@/components/account/account-menu";
import { useOptionalSession, revokeSession } from "@/components/auth/session";
import type { Project } from "@/components/projects/project";
import { ProjectRenameDialog } from "@/components/projects/project-rename-dialog";
import type { Node } from "../document";

const ProjectHistory = dynamic(() =>
  import("../history/project-history").then((m) => m.CanvasProjectHistory),
);

export function CanvasHeader({
  project,
  nodes,
  status,
  saveLabel,
  saving,
  canSave,
  onSave,
  onSelect,
  onMessage,
  onShortcuts,
  canRename = false,
}: {
  project: Project;
  nodes: Node[];
  status: string;
  saveLabel: string;
  saving: boolean;
  canSave: boolean;
  onSave: () => void;
  onSelect: (id: string) => void;
  onMessage: (message: string) => void;
  onShortcuts: () => void;
  canRename?: boolean;
}) {
  const [search, setSearch] = useState(false);
  const [history, setHistory] = useState(false);
  const [rename, setRename] = useState(false);
  const [share, setShare] = useState(false);
  const session = useOptionalSession();
  async function copyLink() {
    try {
      await navigator.clipboard.writeText(window.location.href);
      onMessage("画布链接已复制；访问者仍需具备项目权限。");
    } catch {
      onMessage("无法访问剪贴板，请复制浏览器地址栏中的画布链接。");
    }
  }
  return (
    <>
      <ProjectRenameDialog
        project={project}
        open={rename}
        onOpenChange={setRename}
      />
      <div className="canvas-project-cluster">
        <Brand />
        <h1 className="canvas-project-heading" aria-label={project.name}>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                aria-label="项目选项"
                className="canvas-project-title"
              >
                <span className="truncate">{project.name}</span>
                <ChevronDown data-icon="inline-end" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuGroup>
                <DropdownMenuItem
                  disabled={!canRename}
                  onSelect={() => setRename(true)}
                >
                  重命名画布
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/projects">返回项目</Link>
                </DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() =>
                    onMessage(
                      `画幅 ${project.aspect_ratio} · ${project.style_key ?? "未设风格"}`,
                    )
                  }
                >
                  项目信息
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </h1>
        <span role="status" aria-live="polite" className="canvas-save-status">
          {status.replace(/ · 版本 \d+$/, "")}
        </span>
      </div>
      <div className="canvas-header-actions">
        <div className="canvas-topbar-action-group">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="搜索画布"
            onClick={() => setSearch(true)}
          >
            <Search data-icon="inline-start" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="生成历史"
            onClick={() => setHistory(true)}
          >
            <History data-icon="inline-start" />
          </Button>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="canvas-share-trigger"
          onClick={() => setShare(true)}
        >
          <Share2 data-icon="inline-start" />
          分享
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="画布更多操作">
              <MoreHorizontal data-icon="inline-start" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuGroup>
              <DropdownMenuItem disabled={!canSave || saving} onSelect={onSave}>
                <Save />
                {saveLabel}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={onShortcuts}>
                <Keyboard />
                快捷键
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        {session ? (
          <CanvasAccountMenu session={session} onMessage={onMessage} />
        ) : (
          <Button variant="ghost" size="icon-sm" asChild>
            <Link href="/account" aria-label="账号设置">
              <UserRound data-icon="inline-start" />
            </Link>
          </Button>
        )}
        <Button
          className="canvas-save-button"
          data-error={saveLabel === "重试原保存"}
          variant="ghost"
          size="sm"
          disabled={!canSave || saving}
          onClick={onSave}
        >
          {saveLabel}
        </Button>
      </div>
      <CommandDialog
        open={search}
        onOpenChange={setSearch}
        title="搜索画布"
        description="按名称搜索并定位画布节点"
      >
        <Command>
          <CommandInput placeholder="搜索节点名称…" />
          <CommandList>
            <CommandEmpty>没有匹配的节点</CommandEmpty>
            <CommandGroup heading="画布节点">
              {nodes.map((n) => (
                <CommandItem
                  key={n.id}
                  value={`${n.title} ${n.id}`}
                  onSelect={() => {
                    onSelect(n.id);
                    setSearch(false);
                  }}
                >
                  {n.title}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </CommandDialog>
      {history && (
        <ProjectHistory
          key={project.id}
          projectID={project.id}
          onClose={() => setHistory(false)}
        />
      )}
      <Dialog open={share} onOpenChange={setShare}>
        <DialogContent>
          <DialogTitle>分享画布</DialogTitle>
          <DialogDescription>
            复制当前画布链接。链接保留项目现有权限，仅具备访问权限的成员可以打开。
          </DialogDescription>
          <DialogFooter>
            <Button onClick={() => void copyLink()}>复制链接</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function CanvasAccountMenu({
  session,
  onMessage,
}: {
  session: API.EmailSessionView;
  onMessage: (message: string) => void;
}) {
  const router = useRouter();
  const [logout, setLogout] = useState(false);
  const [pending, setPending] = useState(false);
  async function leaveAccount() {
    if (!session || pending) return;
    setPending(true);
    try {
      await revokeSession(session);
      router.replace("/login");
    } catch {
      onMessage("退出结果未确认，请重试。");
    } finally {
      setPending(false);
    }
  }
  return (
    <>
      <AccountMenu actor={session.actor} onLogout={() => setLogout(true)} />
      <Dialog open={logout} onOpenChange={setLogout}>
        <DialogContent>
          <DialogTitle>退出登录</DialogTitle>
          <DialogDescription>
            确认退出当前设备？请先保存画布编辑。
          </DialogDescription>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setLogout(false)}
            >
              取消
            </Button>
            <Button disabled={pending} onClick={() => void leaveAccount()}>
              确认退出
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
