"use client";
import { Fragment } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useSWRConfig } from "swr";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { Button } from "@/components/ui/button";
import {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
  DropdownMenuItem,
  DropdownMenuGroup,
} from "@/components/ui/dropdown-menu";
import { useProjectActions } from "./project-action-provider";
import { invalidateWorkspaceResource } from "../lib/workspace-commands";
export function ProjectBreadcrumbs({
  project,
  compact = false,
}: {
  project: CanvasContract.ProjectWorkspaceVO;
  compact?: boolean;
}) {
  const path = project.breadcrumbs ?? [
    { projectId: project.projectId, title: project.title },
  ];
  const hidden = path.length > 3 ? path.slice(1, -2) : [];
  const visible = hidden.length ? [path[0], ...path.slice(-2)] : path;
  return (
    <>
      {!compact && (
        <Breadcrumb aria-label="项目路径" className="hidden min-w-0 md:block">
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link href="/projects">全部项目</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            {visible.map((item, index) => (
              <Fragment key={String(item.projectId)}>
                <BreadcrumbSeparator>/</BreadcrumbSeparator>
                {index === 1 && hidden.length > 0 ? (
                  <>
                    <BreadcrumbItem>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label="展开中间项目路径"
                          >
                            …
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent>
                          <DropdownMenuGroup>
                            {hidden.map((part) => (
                              <DropdownMenuItem
                                key={String(part.projectId)}
                                asChild
                              >
                                <Link href={`/projects/${part.projectId}`}>
                                  {part.title}
                                </Link>
                              </DropdownMenuItem>
                            ))}
                          </DropdownMenuGroup>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </BreadcrumbItem>
                    <BreadcrumbSeparator>/</BreadcrumbSeparator>
                  </>
                ) : null}
                <BreadcrumbItem className="min-w-0">
                  {String(item.projectId) === String(project.projectId) ? (
                    <BreadcrumbPage
                      className="max-w-48 truncate"
                      title={item.title}
                    >
                      {item.title}
                    </BreadcrumbPage>
                  ) : (
                    <BreadcrumbLink asChild>
                      <Link
                        className="max-w-48 truncate"
                        title={item.title}
                        href={`/projects/${item.projectId}`}
                      >
                        {item.title}
                      </Link>
                    </BreadcrumbLink>
                  )}
                </BreadcrumbItem>
              </Fragment>
            ))}
          </BreadcrumbList>
        </Breadcrumb>
      )}
      <Sheet>
        <SheetTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className={compact ? "max-w-48 truncate" : "self-start md:hidden"}
          >
            查看完整项目路径
          </Button>
        </SheetTrigger>
        <SheetContent className="overflow-hidden">
          <SheetHeader className="shrink-0 pr-12">
            <SheetTitle>所属项目</SheetTitle>
            <SheetDescription>
              按顺序显示当前项目及其上级，可选择上级项目返回。
            </SheetDescription>
          </SheetHeader>
          <Breadcrumb
            aria-label="完整项目路径"
            className="min-h-0 flex-1 overflow-y-auto px-4 pb-4"
          >
            <BreadcrumbList className="flex-col items-start gap-3">
              <BreadcrumbItem>
                <BreadcrumbLink asChild>
                  <Link href="/projects">全部项目</Link>
                </BreadcrumbLink>
              </BreadcrumbItem>
              {path.map((item) => (
                <BreadcrumbItem
                  key={String(item.projectId)}
                  className="min-w-0 wrap-anywhere"
                >
                  {String(item.projectId) === String(project.projectId) ? (
                    <BreadcrumbPage>{item.title}</BreadcrumbPage>
                  ) : (
                    <BreadcrumbLink asChild>
                      <Link href={`/projects/${item.projectId}`}>
                        {item.title}
                      </Link>
                    </BreadcrumbLink>
                  )}
                </BreadcrumbItem>
              ))}
            </BreadcrumbList>
          </Breadcrumb>
        </SheetContent>
      </Sheet>
    </>
  );
}

export function ProjectMetadataActions({
  project,
  onChanged,
  compact = false,
}: {
  project: CanvasContract.ProjectWorkspaceVO;
  onChanged: () => void;
  compact?: boolean;
}) {
  const actions = useProjectActions();
  const { account, ready, signingOut } = useAccount();
  const { mutate } = useSWRConfig();
  const router = useRouter();
  const identity =
    ready && account && !signingOut ? `USER:${account.id}` : null;
  const target = {
    kind: "project" as const,
    id: String(project.projectId),
    title: project.title,
    revision: project.revision,
    coverFileId: project.coverFileId,
    coverFileVersion: project.coverFileVersion,
    parentId:
      project.parentProjectId == null
        ? undefined
        : String(project.parentProjectId),
  };
  return (
    <div className="flex shrink-0 gap-2">
      {(["rename", "delete"] as const).map((operation) => (
        <Button
          key={operation}
          variant={compact ? "ghost" : "outline"}
          disabled={!identity || actions.active}
          onClick={(event) =>
            actions.openResourceAction(
              operation,
              [target],
              event.currentTarget,
              undefined,
              (command) => {
                if (identity)
                  invalidateWorkspaceResource(mutate, identity, command);
                onChanged();
                if (command.operation === "delete")
                  router.push(
                    project.parentProjectId
                      ? `/projects/${project.parentProjectId}`
                      : "/projects",
                  );
              },
            )
          }
        >
          {operation === "rename" ? "编辑" : "删除"}
        </Button>
      ))}
    </div>
  );
}
