'use client';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useState, type RefObject } from 'react';
import {
  BookOpen,
  Boxes,
  ChevronRight,
  FolderGit2,
  GitBranch,
  ListChecks,
  Search,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarSeparator,
  useSidebar,
} from '@/components/ui/sidebar';

export interface DirectoryItem {
  readonly id: string;
  readonly title: string;
  readonly category: string;
}
/** Stable destinations are shared with the breadcrumb; document content stays server-rendered. */
export const workspaceSections = [
  { href: '/', label: '知识库', icon: BookOpen },
  { href: '/skills/', label: 'Skill', icon: Boxes },
  { href: '/workflows/', label: '工作流', icon: GitBranch },
  { href: '/tasks/', label: '任务与交接', icon: ListChecks },
  { href: '/engineering/', label: '工程引用', icon: FolderGit2 },
  { href: '/graph/', label: '知识关系', icon: GitBranch },
];
/** Project-owned shadcn Sidebar: icon collapse, document submenus and mobile Sheet. */
export function WorkspaceSidebar({
  items,
  categories,
  online,
  preview,
  trigger,
}: {
  readonly items: DirectoryItem[];
  readonly categories: Record<string, string>;
  readonly online: boolean;
  readonly preview: boolean;
  readonly trigger: RefObject<HTMLButtonElement | null>;
}) {
  const pathname = usePathname();
  const { setOpenMobile } = useSidebar();
  const [filter, setFilter] = useState('');
  const current = items.find((item) => pathname === `/read/${item.id}/`);
  function closeMobile() {
    setOpenMobile(false);
  }
  return (
    <Sidebar
      collapsible="icon"
      mobileTitle="项目目录"
      mobileDescription="浏览项目知识、Skill、工作流与工程规范。"
      onMobileCloseAutoFocus={(event) => {
        event.preventDefault();
        trigger.current?.focus();
      }}
    >
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild tooltip="织光 Workspace">
              <Link href="/" onClick={closeMobile}>
                <BookOpen aria-hidden="true" />
                <div className="flex flex-col gap-0.5">
                  <span className="font-semibold">织光</span>
                  <span className="text-xs text-muted-foreground">
                    Workspace
                  </span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>工作空间</SidebarGroupLabel>
          <SidebarMenu>
            {workspaceSections.map(({ href, label, icon: Icon }) => (
              <SidebarMenuItem key={href}>
                <SidebarMenuButton
                  asChild
                  isActive={pathname === href}
                  tooltip={label}
                >
                  <Link
                    href={href}
                    onClick={closeMobile}
                    aria-current={pathname === href ? 'page' : undefined}
                  >
                    <Icon aria-hidden="true" />
                    <span>{label}</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
            {online && (
              <SidebarMenuItem>
                <SidebarMenuButton
                  asChild
                  isActive={pathname === '/operations/'}
                  tooltip="在线协作台"
                >
                  <Link href="/operations/" onClick={closeMobile}>
                    <ListChecks aria-hidden="true" />
                    <span>在线协作台</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
          </SidebarMenu>
        </SidebarGroup>
        <SidebarSeparator className="group-data-[collapsible=icon]:hidden" />
        <SidebarGroup className="group-data-[collapsible=icon]:hidden">
          <SidebarGroupLabel>项目文档</SidebarGroupLabel>
          <InputGroup className="mb-3">
            <InputGroupAddon>
              <Search aria-hidden="true" />
            </InputGroupAddon>
            <InputGroupInput
              aria-label="筛选目录"
              placeholder="筛选文档…"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              autoComplete="off"
            />
          </InputGroup>
          <nav aria-label="项目目录">
            <SidebarMenu>
              {Object.entries(categories).map(([category, label]) => {
                const documents = items.filter(
                  (item) =>
                    item.category === category &&
                    item.title
                      .toLocaleLowerCase()
                      .includes(filter.toLocaleLowerCase()),
                );
                if (!documents.length) {
                  return null;
                }
                const active = current?.category === category;
                return (
                  <Collapsible
                    asChild
                    key={`${category}:${Boolean(filter)}:${active}`}
                    defaultOpen={Boolean(filter) || active}
                    className="group/category"
                  >
                    <SidebarMenuItem>
                      <CollapsibleTrigger asChild>
                        <SidebarMenuButton>
                          <ChevronRight
                            aria-hidden="true"
                            className="transition-transform group-data-[state=open]/category:rotate-90"
                          />
                          <span>{label}</span>
                        </SidebarMenuButton>
                      </CollapsibleTrigger>
                      <SidebarMenuBadge>{documents.length}</SidebarMenuBadge>
                      <CollapsibleContent>
                        <SidebarMenuSub>
                          {documents.map((item) => (
                            <SidebarMenuSubItem key={item.id}>
                              <SidebarMenuSubButton
                                asChild
                                isActive={current?.id === item.id}
                                className="h-auto min-h-9 py-2"
                              >
                                <Link
                                  href={`/read/${item.id}/`}
                                  title={item.title}
                                  aria-current={
                                    current?.id === item.id ? 'page' : undefined
                                  }
                                  onClick={closeMobile}
                                >
                                  <span className="truncate">{item.title}</span>
                                </Link>
                              </SidebarMenuSubButton>
                            </SidebarMenuSubItem>
                          ))}
                        </SidebarMenuSub>
                      </CollapsibleContent>
                    </SidebarMenuItem>
                  </Collapsible>
                );
              })}
            </SidebarMenu>
          </nav>
          {filter &&
            !items.some((item) =>
              item.title
                .toLocaleLowerCase()
                .includes(filter.toLocaleLowerCase()),
            ) && (
              <p
                role="status"
                className="px-2 py-4 text-sm text-muted-foreground"
              >
                没有匹配的文档
              </p>
            )}
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="group-data-[collapsible=icon]:hidden">
        <SidebarSeparator />
        <div className="flex items-center justify-between gap-2 p-2">
          <span className="text-xs text-muted-foreground">
            同仓知识 · 独立部署
          </span>
          <Badge variant="outline">
            {preview ? '本地预览' : online ? '在线协作' : '静态阅读'}
          </Badge>
        </div>
      </SidebarFooter>
    </Sidebar>
  );
}
