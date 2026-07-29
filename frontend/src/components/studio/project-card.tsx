import { ArrowUpRight, Clock3, MoreHorizontal } from "lucide-react";
import Image from "next/image";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import type { MockProject } from "@/lib/mock-studio-data";

export function ProjectCard({ project, compact = false }: { project: MockProject; compact?: boolean }) {
  return (
    <Link
      aria-label={`打开项目 ${project.name}`}
      className="group block min-w-0 overflow-hidden rounded-2xl border border-slate-200 bg-white transition-all hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-lg hover:shadow-slate-950/5"
      href={`/projects/${project.id}`}
    >
      <div className={compact ? "relative aspect-[16/10] overflow-hidden bg-slate-100" : "relative aspect-[4/3] overflow-hidden bg-slate-100"}>
        <Image alt={`${project.name}封面`} fill sizes="(min-width: 1024px) 30vw, 50vw" src={project.cover} className="object-cover transition duration-500 group-hover:scale-[1.025]" />
        <div className="absolute top-3 right-3 flex items-center gap-2">
          <Badge className="border-white/50 bg-white/90 text-slate-700 shadow-sm backdrop-blur" variant="outline">{project.currentStage}</Badge>
          <span className="grid size-7 place-items-center rounded-lg bg-white/90 text-slate-600 shadow-sm"><MoreHorizontal className="size-4" aria-hidden="true" /></span>
        </div>
      </div>
      <div className="p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h3 className="truncate font-semibold">{project.name}</h3>
            <p className="mt-1 line-clamp-1 text-sm text-slate-500">{project.tagline}</p>
          </div>
          <ArrowUpRight className="mt-0.5 size-4 shrink-0 text-slate-400 transition group-hover:text-[#079db3]" aria-hidden="true" />
        </div>
        <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-slate-100" aria-label={`${project.name}制作进度 ${project.progress}%`}>
          <div className="h-full rounded-full bg-[#079db3]" style={{ width: `${project.progress}%` }} />
        </div>
        <div className="mt-3 flex items-center justify-between text-xs text-slate-500">
          <span>{project.currentEpisode}/{project.episodes} 集 · {project.ratio}</span>
          <span className="flex items-center gap-1"><Clock3 className="size-3.5" aria-hidden="true" />{project.updatedAt}</span>
        </div>
      </div>
    </Link>
  );
}
