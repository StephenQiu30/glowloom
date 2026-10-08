"use client";

import { useState, type ReactNode, type CSSProperties } from "react";
import Link from "next/link";
import { cn } from "cn";
import { toast } from "sonner";
import {
  Activity,
  Bell,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronsUpDown,
  CircleHelp,
  Clapperboard,
  Clock3,
  FileText,
  Folder,
  Grid2X2,
  ImageIcon,
  KeyRound,
  Layers,
  ListTodo,
  Menu,
  Monitor,
  MoreHorizontal,
  PanelLeft,
  Plus,
  Search,
  Settings2,
  Shield,
  SlidersHorizontal,
  Star,
  UserRound,
  Waves,
  X,
  type LucideIcon,
} from "lucide-react";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

import {
  Sidebar,
  SidebarProvider,
  SidebarHeader,
  SidebarContent,
  SidebarFooter,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
} from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { screenHref, screens, type Screen } from "./screens";

export const demoNotice = (message: string): void => {
  toast.success(`${message}（本地演示）`);
};
export function IconButton({
  icon: Icon,
  label,
  onClick,
  href,
  ...props
}: {
  icon: LucideIcon;
  label: string;
  onClick?: () => void;
  href?: string;
} & Pick<
  React.ComponentProps<typeof Button>,
  "variant" | "size" | "disabled"
>) {
  const content = href ? (
    <Button variant="ghost" size="icon" aria-label={label} asChild {...props}>
      <Link href={href}>
        <Icon data-icon="inline-start" />
      </Link>
    </Button>
  ) : (
    <Button
      variant="ghost"
      size="icon"
      aria-label={label}
      onClick={onClick}
      {...props}
    >
      <Icon data-icon="inline-start" />
    </Button>
  );
  return (
    <Tooltip>
      <TooltipTrigger asChild>{content}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
export function Brand({ wordmark = false }: { wordmark?: boolean }) {
  return (
    <Link
      href={screenHref("home")}
      aria-label="浮光首页"
      className="inline-flex items-center gap-2 text-sm font-medium"
    >
      <span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
        <Waves className="size-4" />
      </span>
      {wordmark ? "浮光" : null}
    </Link>
  );
}
export function SearchInput({
  value,
  onChange,
  placeholder = "搜索",
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}) {
  return (
    <InputGroup className={className}>
      <InputGroupAddon>
        <Search />
      </InputGroupAddon>
      <InputGroupInput
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
      />
    </InputGroup>
  );
}
export function Choice({
  value,
  onChange,
  options,
  label,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  options: readonly string[];
  label: string;
  className?: string;
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger aria-label={label} className={className}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {options.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
export function PageHeading({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children?: ReactNode;
}) {
  return (
    <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="text-xl leading-7 font-semibold tracking-tight">
          {title}
        </h1>
        {description ? (
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {description}
          </p>
        ) : null}
      </div>
      {children ? (
        <div className="flex flex-wrap items-center gap-2">{children}</div>
      ) : null}
    </header>
  );
}
export function Placeholder({
  className,
  label,
  kind = "image",
  showIcon = true,
  children,
}: {
  className?: string;
  label?: string;
  kind?: "image" | "video" | "audio";
  showIcon?: boolean;
  children?: ReactNode;
}) {
  const Icon =
    kind === "video" ? Clapperboard : kind === "audio" ? Activity : ImageIcon;
  return (
    <div
      className={cn(
        "relative flex items-center justify-center overflow-hidden rounded-lg bg-surface-2 text-subtle-foreground",
        className,
      )}
    >
      {showIcon ? (
        <Icon className="size-5 opacity-40" aria-hidden="true" />
      ) : null}
      {label ? (
        <span className="absolute bottom-3 left-3 text-[10px] tracking-widest text-muted-foreground">
          {label}
        </span>
      ) : null}
      {children}
    </div>
  );
}
export function LocalDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  onConfirm,
  confirmLabel = "保存",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  onConfirm?: () => void | boolean;
  confirmLabel?: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {description ?? "此操作仅更新本地演示，不会修改真实数据。"}
          </DialogDescription>
        </DialogHeader>
        {children}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            onClick={() => {
              if (onConfirm?.() !== false) onOpenChange(false);
            }}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
export function MoreMenu({
  label = "更多操作",
  items,
}: {
  label?: string;
  items: { label: string; action: () => void }[];
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="icon-sm" variant="ghost" aria-label={label}>
          <MoreHorizontal data-icon="inline-start" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          {items.map((item) => (
            <DropdownMenuItem key={item.label} onSelect={item.action}>
              {item.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
const adminLinks: [Screen, string, LucideIcon][] = [
  ["users", "账号", UserRound],
  ["providers", "供应商凭据", KeyRound],
  ["models", "模型注册表", Layers],
  ["audit", "审计日志", FileText],
  ["health", "系统健康", Activity],
];
const projectLinks: [Screen, string, LucideIcon][] = [
  ["analytics", "概览", Grid2X2],
  ["storyboard", "剧本", FileText],
  ["bible", "设定集", UserRound],
];
function NavigationLink({
  href,
  label,
  icon: Icon,
  active,
  onClick,
}: {
  href: string;
  label: string;
  icon: LucideIcon;
  active?: boolean;
  onClick?: () => void;
}) {
  return (
    <Button
      variant="navigation"
      asChild
      data-active={active || undefined}
      className="w-full justify-start gap-2.5"
    >
      <Link
        href={href}
        aria-current={active ? "page" : undefined}
        onClick={onClick}
      >
        <Icon data-icon="inline-start" />
        {label}
      </Link>
    </Button>
  );
}
function ContextNavigation({
  screen,
  close,
}: {
  screen: Screen;
  close?: () => void;
}) {
  const [episode, setEpisode] = useState("第 3 集");
  const admin = adminLinks.some(([id]) => id === screen);
  const account = screen === "account";
  return (
    <div className="flex h-full flex-col gap-5 p-4 text-sm">
      <div className="flex items-center justify-between gap-2 py-2">
        <div className="font-medium text-foreground">
          {admin ? "管理" : account ? "设置" : "雾港来信"}
          {!admin && !account ? (
            <p className="mt-1 text-[10px] font-normal text-muted-foreground">
              9:16 · 写实电影感
            </p>
          ) : null}
        </div>
        {!admin && !account ? (
          <ChevronsUpDown className="size-3 text-muted-foreground" />
        ) : null}
      </div>
      <nav className="flex flex-col gap-1" aria-label="上下文导航">
        {admin ? (
          adminLinks.map(([id, label, icon]) => (
            <NavigationLink
              key={id}
              href={screenHref(id)}
              label={label}
              icon={icon}
              active={screen === id}
              onClick={close}
            />
          ))
        ) : account ? (
          [
            ["profile", "个人资料", UserRound],
            ["sessions", "密码与会话", Monitor],
            ["preferences", "偏好", SlidersHorizontal],
            ["licenses", "模型来源", Shield],
          ].map(([id, label, icon]) => (
            <NavigationLink
              key={id as string}
              href={`#${id}`}
              label={label as string}
              icon={icon as LucideIcon}
              active={id === "profile"}
              onClick={close}
            />
          ))
        ) : (
          <>
            {projectLinks.map(([id, label, icon]) => (
              <NavigationLink
                key={label}
                href={screenHref(id)}
                label={label}
                icon={icon}
                active={screen === id && label !== "剧本"}
                onClick={close}
              />
            ))}
            <div className="my-4 flex items-center justify-between text-xs text-muted-foreground">
              <span>分集</span>
              <Choice
                label="当前分集"
                value={episode}
                onChange={setEpisode}
                options={["第 1 集", "第 2 集", "第 3 集", "第 4 集"]}
              />
            </div>
            <NavigationLink
              href={screenHref("bible")}
              label="资产定稿"
              icon={UserRound}
              onClick={close}
            />
            <NavigationLink
              href={screenHref("storyboard")}
              label="分镜"
              icon={Clapperboard}
              active={screen === "storyboard"}
              onClick={close}
            />
            <NavigationLink
              href={screenHref("shot")}
              label="配音"
              icon={Activity}
              onClick={close}
            />
            <NavigationLink
              href={screenHref("canvas")}
              label="画布"
              icon={Grid2X2}
              active={screen === "canvas"}
              onClick={close}
            />
            <div className="mt-5 flex flex-col gap-1">
              <NavigationLink
                href={screenHref("assets")}
                label="素材"
                icon={Folder}
                onClick={close}
              />
              <NavigationLink
                href={screenHref("account")}
                label="项目设置"
                icon={Settings2}
                onClick={close}
              />
            </div>
          </>
        )}
      </nav>
      {!admin && !account ? (
        <p className="mt-auto flex items-center gap-2 pt-10 text-[10px] text-muted-foreground">
          <Check className="size-3" />
          所有更改已保存
        </p>
      ) : null}
    </div>
  );
}
function ProductNavigation({
  screen,
  contextual,
  onToggleContext,
}: {
  screen: Screen;
  contextual: boolean;
  onToggleContext: () => void;
}) {
  const { setOpenMobile } = useSidebar();
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState("");
  const close = () => setOpenMobile(false);
  const admin = adminLinks.some(([id]) => id === screen);
  const navs: [string, string, LucideIcon, boolean][] = [
    ["home", "项目", Grid2X2, !admin && screen !== "assets"],
    ["assets", "素材", Folder, screen === "assets"],
    ["analytics", "任务", ListTodo, false],
    ["users", "管理", Shield, admin],
  ];
  return (
    <Sidebar collapsible="offcanvas">
      <SidebarHeader className="items-center gap-4 py-4">
        <Button
          className="md:hidden"
          variant="ghost"
          size="icon"
          aria-label="关闭导航"
          onClick={close}
        >
          <X data-icon="inline-start" />
        </Button>
        <Brand />
        <IconButton
          label="新建项目"
          icon={Plus}
          href={screenHref("home")}
          variant="outline"
        />
      </SidebarHeader>
      <SidebarContent>
        <SidebarMenu className="gap-1 px-2">
          {navs.map(([id, label, Icon, active]) => (
            <SidebarMenuItem key={id}>
              <SidebarMenuButton size="rail" isActive={active} asChild>
                <Link href={screenHref(id)} onClick={close}>
                  <Icon />
                  <span>{label}</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
        <div className="md:hidden">
          {contextual ? (
            <ContextNavigation screen={screen} close={close} />
          ) : null}
          <nav className="flex flex-col gap-1 p-4">
            {screens.map((item) => (
              <Link
                key={item.id}
                href={screenHref(item.id)}
                onClick={close}
                className="py-1 text-sm"
              >
                {item.name}
              </Link>
            ))}
          </nav>
        </div>
      </SidebarContent>
      <SidebarFooter className="items-center gap-3 pb-4">
        <IconButton
          label="切换上下文导航"
          icon={PanelLeft}
          onClick={onToggleContext}
          disabled={!contextual}
        />
        <IconButton
          label="搜索"
          icon={Search}
          onClick={() => setSearchOpen(true)}
        />
        <Dialog open={searchOpen} onOpenChange={setSearchOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>搜索页面</DialogTitle>
              <DialogDescription>
                按名称查找创作、账号或管理页面。
              </DialogDescription>
            </DialogHeader>
            <SearchInput
              value={query}
              onChange={setQuery}
              placeholder="输入页面名称"
            />
            <nav
              className="flex max-h-72 flex-col gap-1 overflow-y-auto"
              aria-label="搜索结果"
            >
              {screens
                .filter((item) => item.name.includes(query.trim()))
                .map((item) => (
                  <Link
                    key={item.id}
                    href={screenHref(item.id)}
                    className="rounded-md px-3 py-2 text-sm hover:bg-accent focus-visible:outline-ring"
                    onClick={() => {
                      setSearchOpen(false);
                      close();
                    }}
                  >
                    {item.name}
                  </Link>
                ))}
            </nav>
          </DialogContent>
        </Dialog>
        <IconButton
          label="通知"
          icon={Bell}
          onClick={() => demoNotice("暂无新的通知")}
        />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="icon" variant="ghost" aria-label="账号菜单">
              <Avatar>
                <AvatarFallback>陈</AvatarFallback>
              </Avatar>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="right" align="end">
            <DropdownMenuGroup>
              <DropdownMenuItem disabled>陈导</DropdownMenuItem>
            </DropdownMenuGroup>

            <DropdownMenuGroup>
              <DropdownMenuItem asChild>
                <Link href={screenHref("account")}>账号设置</Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link href={screenHref("login")}>退出登录</Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
function MobileHeader() {
  const { toggleSidebar } = useSidebar();
  return (
    <header className="flex h-14 shrink-0 items-center justify-between px-4 md:hidden">
      <Brand wordmark />
      <IconButton label="打开导航" icon={Menu} onClick={toggleSidebar} />
    </header>
  );
}
export function ProductShell({
  screen,
  children,
  contextual = false,
  fullBleed = false,
}: {
  screen: Screen;
  children: ReactNode;
  contextual?: boolean;
  fullBleed?: boolean;
}) {
  const [contextOpen, setContextOpen] = useState(true);
  return (
    <TooltipProvider>
      <SidebarProvider style={{ "--sidebar-width": "72px" } as CSSProperties}>
        <a href="#main-content" className="sr-only focus:not-sr-only">
          跳转到主要内容
        </a>
        <ProductNavigation
          screen={screen}
          contextual={contextual}
          onToggleContext={() => setContextOpen(!contextOpen)}
        />
        {contextual && contextOpen ? (
          <aside className="sticky top-0 hidden h-svh w-[220px] shrink-0 bg-surface-1 md:block">
            <ContextNavigation screen={screen} />
          </aside>
        ) : null}
        <div className="flex min-w-0 flex-1 flex-col">
          <MobileHeader />
          <main
            id="main-content"
            className={cn("min-w-0 flex-1", !fullBleed && "p-4 md:p-6")}
          >
            {children}
          </main>
        </div>
      </SidebarProvider>
    </TooltipProvider>
  );
}
export function EmptySearch({
  label = "没有符合条件的结果",
}: {
  label?: string;
}) {
  return (
    <Empty role="status">
      <EmptyHeader>
        <EmptyTitle>{label}</EmptyTitle>
      </EmptyHeader>
    </Empty>
  );
}

export { Check, ChevronDown, ChevronLeft, CircleHelp, Clock3, Plus, Star, X };
