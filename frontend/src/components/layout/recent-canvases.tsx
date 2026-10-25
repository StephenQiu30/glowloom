"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import Link from "next/link";
import { Box } from "lucide-react";
import { listProjects } from "@/api/projects";
import {
  canvasHref,
  requireProjectList,
  type Project,
} from "@/components/projects/project";
import { subscribeProjectInvalidation } from "@/components/projects/project-presentation-store";
import { ContentState } from "./content-state";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

/** Mounted with an actor/space key. Never retain another scope's private list. */
export function RecentCanvases({
  onNavigate,
  presentation = "sidebar",
  children,
}: {
  onNavigate?: () => void;
  presentation?: "sidebar" | "cards";
  children?: ReactNode;
}) {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const pagination = useRef<AbortController | null>(null);
  useEffect(() => {
    const refresh = () => setRevision((n) => n + 1);
    const unsubscribe = subscribeProjectInvalidation(refresh);
    window.addEventListener("focus", refresh);
    return () => {
      unsubscribe();
      window.removeEventListener("focus", refresh);
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    pagination.current?.abort();
    void listProjects(
      { limit: 20, status: "active", order: "recent_content" },
      { signal: controller.signal },
    )
      .then(requireProjectList)
      .then((page) => {
        if (controller.signal.aborted) return;
        setProjects(
          page.items.filter(
            (p) => p.canvas_id && !p.is_delete && p.status !== "copying",
          ),
        );
        setCursor(page.next_cursor);
        setBusy(false);
        setError(false);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError(true);
      });
    return () => {
      controller.abort();
      pagination.current?.abort();
    };
  }, [revision]);
  async function more() {
    if (
      !cursor ||
      busy ||
      (pagination.current && !pagination.current.signal.aborted)
    )
      return;
    const controller = new AbortController();
    pagination.current = controller;
    setBusy(true);
    try {
      const page = requireProjectList(
        await listProjects(
          {
            limit: 20,
            status: "active",
            order: "recent_content",
            cursor,
          },
          { signal: controller.signal },
        ),
      );
      if (controller.signal.aborted) return;
      setProjects((current) => [
        ...(current ?? []),
        ...page.items.filter(
          (p) =>
            p.canvas_id &&
            !p.is_delete &&
            p.status !== "copying" &&
            !current?.some((c) => c.id === p.id),
        ),
      ]);
      setCursor(page.next_cursor);
      setError(false);
    } catch {
      if (!controller.signal.aborted) setError(true);
    } finally {
      if (pagination.current === controller) {
        pagination.current = null;
        if (!controller.signal.aborted) setBusy(false);
      }
    }
  }
  if (presentation === "cards")
    return (
      <div className="home-canvas-list" aria-label="最近画布">
        {children}
        {projects?.slice(0, 3).map((project) => (
          <Button key={project.id} variant="tile" asChild>
            <Link
              href={canvasHref(project)}
              onClick={onNavigate}
              title={project.name}
            >
              <Box aria-hidden="true" />
              <span className="max-w-full truncate">{project.name}</span>
            </Link>
          </Button>
        ))}
        {error ? (
          <ContentState
            className="home-canvas-state"
            state="error"
            title="暂时无法读取画布"
            actions={
              <Button variant="ghost" onClick={() => setRevision((n) => n + 1)}>
                重试读取画布
              </Button>
            }
          />
        ) : projects === null ? (
          [0, 1, 2].map((key) => (
            <Skeleton
              key={key}
              className="home-canvas-skeleton"
              aria-label="正在读取画布"
            />
          ))
        ) : projects.length === 0 ? (
          <ContentState className="home-canvas-state" title="暂无画布" />
        ) : null}
      </div>
    );
  return (
    <SidebarMenu aria-label="最近画布">
      {projects?.map((project) => (
        <SidebarMenuItem key={project.id}>
          <SidebarMenuButton asChild tooltip={project.name}>
            <Link
              href={canvasHref(project)}
              onClick={onNavigate}
              title={project.name}
            >
              <Box />
              <span>{project.name}</span>
            </Link>
          </SidebarMenuButton>
        </SidebarMenuItem>
      ))}
      {error ? (
        <li>
          <Button
            variant="ghost"
            aria-label="重试读取画布"
            onClick={() => setRevision((n) => n + 1)}
          >
            读取失败 · 重试
          </Button>
        </li>
      ) : projects === null ? (
        <li className="sidebar-history-note" role="status">
          正在读取…
        </li>
      ) : projects.length === 0 ? (
        <li className="sidebar-history-note">暂无画布</li>
      ) : null}
      {cursor ? (
        <li>
          <Button variant="ghost" disabled={busy} onClick={() => void more()}>
            {busy ? "正在读取…" : "加载更多"}
          </Button>
        </li>
      ) : null}
    </SidebarMenu>
  );
}
