'use client';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import {
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type CSSProperties,
} from 'react';
import { BookOpen, Search } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Button } from '@/components/ui/button';
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import { Kbd } from '@/components/ui/kbd';
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from '@/components/ui/sidebar';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { ThemeToggle } from '@/components/theme';
import {
  WorkspaceSidebar,
  workspaceSections,
  type DirectoryItem,
} from '@/components/workspace-sidebar';

const sidebarStyle: CSSProperties & { '--sidebar-width': string } = {
  '--sidebar-width': '15rem',
};

interface NavigationProps {
  readonly items: DirectoryItem[];
  readonly categories: Record<string, string>;
  readonly children: ReactNode;
  readonly online: boolean;
  readonly preview: boolean;
}
/** Shares one accessible sidebar and search palette across both build modes. */
export function Navigation(props: NavigationProps) {
  return (
    <SidebarProvider style={sidebarStyle}>
      <WorkspaceShell {...props} />
    </SidebarProvider>
  );
}

function WorkspaceShell({
  items,
  categories,
  children,
  online,
  preview,
}: NavigationProps) {
  const pathname = usePathname();
  const router = useRouter();
  const { setOpenMobile, isMobile, state } = useSidebar();
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState('');
  const searchTrigger = useRef<HTMLButtonElement>(null);
  const menuTrigger = useRef<HTMLButtonElement>(null);
  const current = items.find((item) => pathname === `/read/${item.id}/`);
  const title =
    current?.title ??
    workspaceSections.find((section) => section.href === pathname)?.label ??
    (pathname === '/operations/' ? '在线协作台' : '搜索知识库');
  const matching = items
    .filter((item) =>
      item.title.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()),
    )
    .slice(0, 20);
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key.toLowerCase() === 'k' && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        setSearchOpen((open) => !open);
      }
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, []);
  function navigate(href: string) {
    const event = new CustomEvent('workspace:navigate', {
      cancelable: true,
      detail: { href },
    });
    if (!document.dispatchEvent(event)) {
      setSearchOpen(false);
      return;
    }
    setSearchOpen(false);
    setOpenMobile(false);
    router.push(href);
  }
  return (
    <>
      <WorkspaceSidebar
        items={items}
        categories={categories}
        online={online}
        preview={preview}
        trigger={menuTrigger}
      />
      <SidebarInset id="main" tabIndex={-1} className="min-w-0">
        <header
          className="flex h-16 shrink-0 items-center gap-3 px-4 md:px-8"
          data-pagefind-ignore
        >
          <SidebarTrigger
            ref={menuTrigger}
            aria-label={
              isMobile
                ? '打开项目目录'
                : state === 'expanded'
                  ? '折叠侧栏'
                  : '展开侧栏'
            }
          />
          <Breadcrumb className="min-w-0 flex-1">
            <BreadcrumbList className="flex-nowrap">
              <BreadcrumbItem>
                <BreadcrumbLink asChild>
                  <Link href="/">Workspace</Link>
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem className="min-w-0">
                <BreadcrumbPage className="truncate">
                  {current ? categories[current.category] : title}
                </BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
          <Badge variant="outline" className="hidden sm:inline-flex">
            {online ? '在线协作' : '静态阅读'}
            {preview ? ' · 预览' : ''}
          </Badge>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                ref={searchTrigger}
                variant="ghost"
                size="icon"
                aria-label="搜索知识库"
                onClick={() => setSearchOpen(true)}
              >
                <Search />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              搜索知识库 <Kbd>⌘ / Ctrl K</Kbd>
            </TooltipContent>
          </Tooltip>
          <ThemeToggle />
        </header>
        <div className="min-w-0 flex-1">{children}</div>
        <footer
          className="px-4 py-6 text-xs text-muted-foreground md:px-8"
          data-pagefind-ignore
        >
          织光 Workspace · 知识与工程共用同一版本
        </footer>
      </SidebarInset>
      <CommandDialog
        open={searchOpen}
        onOpenChange={setSearchOpen}
        title="搜索知识库"
        description="查找文档标题，或打开全文搜索。"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          searchTrigger.current?.focus();
        }}
      >
        <Command label="知识库搜索">
          <CommandInput
            placeholder="搜索文档标题…"
            value={query}
            onValueChange={setQuery}
          />
          <CommandList>
            <CommandEmpty>没有找到匹配的标题，试试全文搜索。</CommandEmpty>
            <CommandGroup heading="全文搜索">
              <CommandItem
                value={`全文搜索 ${query}`}
                onSelect={() =>
                  navigate(`/search/?q=${encodeURIComponent(query)}`)
                }
              >
                <Search aria-hidden="true" />
                全文搜索{query ? `「${query}」` : ''}
              </CommandItem>
            </CommandGroup>
            <CommandGroup heading="项目文档">
              {matching.map((item) => (
                <CommandItem
                  key={item.id}
                  value={item.title}
                  onSelect={() => navigate(`/read/${item.id}/`)}
                >
                  <BookOpen aria-hidden="true" />
                  <span>{item.title}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </CommandDialog>
    </>
  );
}
