"use client";

import { useId } from "react";

import { Button } from "@/components/ui/button";

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

function projectStyle(styleType: ProjectListDisplayItem["styleType"]) {
  return styleType === "realistic" ? "写实" : "风格化";
}

function StatusLabel({ project }: { project: ProjectListDisplayItem }) {
  return (
    <span className="inline-flex rounded-full bg-background px-3 py-1 text-xs font-medium text-foreground">
      {projectStatus(project)}
    </span>
  );
}

export function ProjectList({
  projects,
  view,
  onViewChange,
}: ProjectListProps) {
  const headingId = useId();

  return (
    <section aria-labelledby={headingId} className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 id={headingId} className="text-2xl font-semibold tracking-tight">
          项目
        </h2>
        <div
          role="group"
          aria-label="项目列表视图"
          className="flex gap-1 rounded-xl bg-muted/65 p-1"
        >
          <Button
            type="button"
            variant={view === "cards" ? "secondary" : "ghost"}
            aria-label="卡片视图"
            aria-pressed={view === "cards"}
            onClick={() => onViewChange("cards")}
            className="focus-visible:ring-blue-500/60"
          >
            卡片
          </Button>
          <Button
            type="button"
            variant={view === "table" ? "secondary" : "ghost"}
            aria-label="表格视图"
            aria-pressed={view === "table"}
            onClick={() => onViewChange("table")}
            className="focus-visible:ring-blue-500/60"
          >
            表格
          </Button>
        </div>
      </div>

      {projects.length === 0 ? (
        <p
          role="status"
          className="rounded-2xl bg-muted/65 px-6 py-12 text-sm text-muted-foreground"
        >
          当前列表没有项目。
        </p>
      ) : view === "cards" ? (
        <ul className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((project) => (
            <li
              key={project.id}
              className="min-w-0 rounded-2xl bg-muted/65 p-6"
            >
              <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
                <h3 className="min-w-0 flex-1 text-lg font-semibold tracking-tight break-words">
                  {project.name}
                </h3>
                <StatusLabel project={project} />
              </div>
              <dl className="mt-6 flex flex-wrap gap-x-8 gap-y-3 text-sm">
                <div>
                  <dt className="text-muted-foreground">画幅</dt>
                  <dd className="mt-1 font-medium tabular-nums">
                    {project.aspectRatio}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">风格</dt>
                  <dd className="mt-1 font-medium">
                    {projectStyle(project.styleType)}
                  </dd>
                </div>
              </dl>
            </li>
          ))}
        </ul>
      ) : (
        <div className="overflow-x-auto rounded-2xl bg-muted/40">
          <table className="w-full min-w-[34rem] table-fixed text-left text-sm">
            <caption className="sr-only">项目列表</caption>
            <thead className="text-muted-foreground">
              <tr>
                <th scope="col" className="w-2/5 px-5 py-4 font-medium">
                  名称
                </th>
                <th scope="col" className="w-1/5 px-5 py-4 font-medium">
                  状态
                </th>
                <th scope="col" className="w-1/5 px-5 py-4 font-medium">
                  画幅
                </th>
                <th scope="col" className="w-1/5 px-5 py-4 font-medium">
                  风格
                </th>
              </tr>
            </thead>
            <tbody>
              {projects.map((project) => (
                <tr key={project.id}>
                  <td className="px-5 py-4 font-medium break-words">
                    {project.name}
                  </td>
                  <td className="px-5 py-4">
                    <StatusLabel project={project} />
                  </td>
                  <td className="px-5 py-4 tabular-nums">
                    {project.aspectRatio}
                  </td>
                  <td className="px-5 py-4">
                    {projectStyle(project.styleType)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
