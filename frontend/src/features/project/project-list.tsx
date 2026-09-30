"use client";

import { useId } from "react";
import Link from "next/link";
import { ArrowUpRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  TableCaption,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

export type ProjectListDisplayItem = Readonly<{
  id: string;
  name: string;
  aspectRatio: "9:16" | "16:9";
  styleType: "realistic" | "stylized";
  status: "active" | "archived";
  isDeleted: boolean;
}>;
export type ProjectListView = "cards" | "table";
type ProjectListProps = {
  projects: readonly ProjectListDisplayItem[];
  view: ProjectListView;
  onViewChange: (view: ProjectListView) => void;
};
function projectStatus(project: ProjectListDisplayItem) {
  if (project.isDeleted) return "回收中";
  return project.status === "archived" ? "已归档" : "进行中";
}
function projectStyle(project: ProjectListDisplayItem) {
  if (project.styleType === "realistic") return "写实";
  return "风格化";
}
function StatusLabel({ project }: { project: ProjectListDisplayItem }) {
  return <Badge variant="secondary">{projectStatus(project)}</Badge>;
}
function ProjectName({ project }: { project: ProjectListDisplayItem }) {
  return project.isDeleted ? (
    <span>{project.name}</span>
  ) : (
    <Link
      className="rounded-sm break-words hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      href={`/projects/${project.id}/canvas`}
    >
      {project.name}
    </Link>
  );
}
export function ProjectList({
  projects,
  view,
  onViewChange,
}: ProjectListProps) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 id={headingId} className="text-lg font-semibold tracking-tight">
          项目
        </h2>
        <ToggleGroup
          type="single"
          value={view}
          onValueChange={(value) => {
            if (value === "cards" || value === "table") onViewChange(value);
          }}
          aria-label="项目列表视图"
        >
          <ToggleGroupItem value="cards" aria-label="卡片视图">
            卡片
          </ToggleGroupItem>
          <ToggleGroupItem value="table" aria-label="表格视图">
            表格
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
      {projects.length === 0 ? (
        <Empty role="status" className="bg-muted/40 py-16">
          <EmptyHeader>
            <EmptyTitle>当前列表没有项目。</EmptyTitle>
            <EmptyDescription>
              新建项目，或调整搜索和状态筛选。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : view === "cards" ? (
        <ul className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((project) => (
            <li key={project.id} className="min-w-0">
              <Card className="h-full gap-5 border-0 bg-muted/40 p-6 shadow-none ring-0">
                <CardHeader className="px-0">
                  <CardTitle>
                    <h3 className="text-xl break-words">
                      <ProjectName project={project} />
                    </h3>
                  </CardTitle>
                  <CardDescription>
                    {project.aspectRatio} · {projectStyle(project)}
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-wrap items-center gap-3 px-0">
                  <StatusLabel project={project} />
                </CardContent>
                <CardFooter className="mt-auto justify-between gap-3 border-0 bg-transparent px-0 py-0">
                  {!project.isDeleted && (
                    <Link
                      href={`/projects/${project.id}/canvas`}
                      className="inline-flex items-center gap-1 rounded-sm text-sm hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                      aria-label={`打开 ${project.name} 的画布`}
                    >
                      进入画布
                      <ArrowUpRight aria-hidden="true" className="size-4" />
                    </Link>
                  )}
                </CardFooter>
              </Card>
            </li>
          ))}
        </ul>
      ) : (
        <Table className="min-w-[34rem] table-fixed">
          <TableCaption className="sr-only">项目列表</TableCaption>
          <TableHeader className="[&_tr]:border-0">
            <TableRow className="border-0">
              <TableHead className="w-2/5">名称</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>画幅</TableHead>
              <TableHead>风格</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {projects.map((project) => (
              <TableRow key={project.id} className="border-0">
                <TableCell className="py-4 font-medium break-words whitespace-normal">
                  <ProjectName project={project} />
                </TableCell>
                <TableCell>
                  <StatusLabel project={project} />
                </TableCell>
                <TableCell className="tabular-nums">
                  {project.aspectRatio}
                </TableCell>
                <TableCell>{projectStyle(project)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  );
}
