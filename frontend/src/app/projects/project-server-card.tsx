import { ArrowUpRight, Clock3, Layers3 } from "lucide-react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function ProjectServerCard({ project }: { project: API.ProjectResponse }) {
  return (
    <Card className="group relative transition hover:border-cyan-200 hover:shadow-md hover:shadow-cyan-950/5">
      <CardHeader>
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Badge className={project.status === "active" ? "border-cyan-100 bg-cyan-50 text-[#087f91]" : ""} variant="outline">
                {project.status === "active" ? "制作中" : "已归档"}
              </Badge>
              <Badge variant="outline">{project.aspect_ratio}</Badge>
            </div>
            <CardTitle className="mt-4 truncate text-xl">{project.name}</CardTitle>
          </div>
          <ArrowUpRight className="size-5 shrink-0 text-slate-300 transition group-hover:text-[#079db3]" aria-hidden="true" />
        </div>
      </CardHeader>
      <CardContent>
        <p className="line-clamp-2 min-h-12 text-sm leading-6 text-slate-500">{project.description || "尚未填写项目简介"}</p>
        <div className="mt-5 flex flex-wrap gap-4 border-t border-slate-100 pt-4 text-xs text-slate-500">
          <span className="flex items-center gap-1.5"><Layers3 className="size-3.5" aria-hidden="true" />{project.visual_style ?? "未设视觉风格"}</span>
          <span className="flex items-center gap-1.5"><Clock3 className="size-3.5" aria-hidden="true" />{Math.round(project.target_duration_ms / 1_000)} 秒/集</span>
        </div>
        <Link aria-label={`打开项目 ${project.name}`} className="absolute inset-0 rounded-xl focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#079db3]" href={`/projects/${project.id}`} />
      </CardContent>
    </Card>
  );
}
