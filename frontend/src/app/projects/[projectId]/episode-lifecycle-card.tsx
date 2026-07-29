"use client";

import { Archive, ArrowDown, ArrowUp, RotateCcw, Save, Trash2 } from "lucide-react";
import { type FormEvent, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  appApiErrorMessage,
  useDeleteEpisodeMutation,
  useEpisodeDeletePreflightMutation,
  useReorderEpisodesMutation,
  useSetEpisodeArchivedMutation,
  useUpdateEpisodeMutation,
} from "@/lib/server-state";

type EpisodeLifecycleCardProps = {
  activeEpisodes: API.EpisodeResponse[];
  episode: API.EpisodeResponse;
  episodeSnapshot?: API.EpisodeProductionSnapshot;
  project: API.ProjectResponse;
};

export function EpisodeLifecycleCard({
  activeEpisodes,
  episode,
  episodeSnapshot,
  project,
}: EpisodeLifecycleCardProps) {
  const [message, setMessage] = useState<string | null>(null);
  const [commandError, setCommandError] = useState<string | null>(null);
  const [deleteCheck, setDeleteCheck] = useState<API.DeletePreflightResponse | null>(null);
  const [updateEpisode, updateState] = useUpdateEpisodeMutation();
  const [setArchived, archiveState] = useSetEpisodeArchivedMutation();
  const [reorderEpisodes, reorderState] = useReorderEpisodesMutation();
  const [deletePreflight] = useEpisodeDeletePreflightMutation();
  const [deleteEpisode, deleteState] = useDeleteEpisodeMutation();
  const activeIndex = activeEpisodes.findIndex((item) => item.id === episode.id);

  async function runCommand(command: () => Promise<unknown>, success: string) {
    setCommandError(null);
    setMessage(null);
    try {
      await command();
      setMessage(success);
      return true;
    } catch (error: unknown) {
      setCommandError(appApiErrorMessage(error));
      return false;
    }
  }

  async function handleUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runCommand(
      () =>
        updateEpisode({
          projectId: project.id,
          episodeId: episode.id,
          body: {
            name: String(form.get("name")),
            target_duration_ms: Number(form.get("durationSeconds")) * 1000,
            expected_revision: episode.revision,
          },
        }).unwrap(),
      "单集信息已更新。",
    );
  }

  async function handleState() {
    await runCommand(
      () =>
        setArchived({
          projectId: project.id,
          episodeId: episode.id,
          expectedRevision: episode.revision,
          archived: episode.status === "active",
        }).unwrap(),
      episode.status === "active" ? "单集已归档。" : "单集已恢复。",
    );
  }

  async function move(direction: -1 | 1) {
    const activeIds = activeEpisodes.map((item) => item.id);
    const nextIndex = activeIndex + direction;
    if (activeIndex < 0 || nextIndex < 0 || nextIndex >= activeIds.length) return;
    [activeIds[activeIndex], activeIds[nextIndex]] = [activeIds[nextIndex], activeIds[activeIndex]];
    await runCommand(
      () =>
        reorderEpisodes({
          projectId: project.id,
          body: { episode_ids: activeIds, expected_revision: project.revision },
        }).unwrap(),
      "单集顺序已更新。",
    );
  }

  async function checkDeletion() {
    setDeleteCheck(null);
    const succeeded = await runCommand(async () => {
      setDeleteCheck(await deletePreflight(episode.id).unwrap());
    }, "单集删除条件已检查。");
    if (!succeeded) setDeleteCheck(null);
  }

  async function handleDeletion() {
    await runCommand(
      () =>
        deleteEpisode({
          projectId: project.id,
          episodeId: episode.id,
          expectedRevision: episode.revision,
        }).unwrap(),
      "单集已删除。",
    );
  }

  const writeDisabled = episode.status === "archived" || project.status === "archived";

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-4">
          <div>
            <CardTitle>{episode.name}</CardTitle>
            <CardDescription className="mt-1">
              第 {episode.position} 集 · {Math.round(episode.target_duration_ms / 1000)} 秒
            </CardDescription>
          </div>
          <Badge variant="secondary">
            {episode.status === "active" ? `${episodeSnapshot?.completion ?? 0}%` : "已归档"}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="grid gap-4">
        {commandError ? (
          <Alert variant="destructive">
            <AlertTitle>操作未完成</AlertTitle>
            <AlertDescription>{commandError}</AlertDescription>
          </Alert>
        ) : null}
        {message ? (
          <Alert>
            <AlertTitle>操作成功</AlertTitle>
            <AlertDescription>{message}</AlertDescription>
          </Alert>
        ) : null}
        {episodeSnapshot?.blocking_reasons.length ? (
          <p className="text-sm text-muted-foreground">
            {episodeSnapshot.blocking_reasons[0].summary}
          </p>
        ) : null}
        <form className="grid gap-3 sm:grid-cols-[1fr_8rem_auto] sm:items-end" key={`${episode.id}-${episode.revision}`} onSubmit={handleUpdate}>
          <div className="grid gap-2">
            <Label htmlFor={`episode-name-${episode.id}`}>单集名称 {episode.name}</Label>
            <Input defaultValue={episode.name} disabled={writeDisabled} id={`episode-name-${episode.id}`} maxLength={120} name="name" required />
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`episode-duration-${episode.id}`}>目标秒数</Label>
            <Input defaultValue={episode.target_duration_ms / 1000} disabled={writeDisabled} id={`episode-duration-${episode.id}`} max={7200} min={1} name="durationSeconds" required type="number" />
          </div>
          <Button aria-label={`保存 ${episode.name}`} disabled={writeDisabled || updateState.isLoading} type="submit" variant="outline">
            <Save aria-hidden="true" />
            保存
          </Button>
        </form>
        <div className="flex flex-wrap gap-2">
          {episode.status === "active" ? (
            <>
              <Button aria-label={`上移 ${episode.name}`} disabled={activeIndex <= 0 || reorderState.isLoading} onClick={() => move(-1)} size="icon" variant="outline">
                <ArrowUp aria-hidden="true" />
              </Button>
              <Button aria-label={`下移 ${episode.name}`} disabled={activeIndex < 0 || activeIndex >= activeEpisodes.length - 1 || reorderState.isLoading} onClick={() => move(1)} size="icon" variant="outline">
                <ArrowDown aria-hidden="true" />
              </Button>
            </>
          ) : null}
          <Button aria-label={`${episode.status === "active" ? "归档" : "恢复"} ${episode.name}`} disabled={archiveState.isLoading || project.status === "archived"} onClick={handleState} variant="outline">
            {episode.status === "active" ? <Archive aria-hidden="true" /> : <RotateCcw aria-hidden="true" />}
            {episode.status === "active" ? "归档" : "恢复"}
          </Button>
          <Button aria-label={`检查删除 ${episode.name}`} onClick={checkDeletion} variant="destructive">
            <Trash2 aria-hidden="true" />
            检查删除
          </Button>
        </div>
        {deleteCheck ? (
          <div className="rounded-lg bg-muted p-3 text-sm">
            {deleteCheck.allowed ? (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p>该单集是无引用空草稿，可以删除。</p>
                <Button aria-label={`确认删除 ${episode.name}`} disabled={deleteState.isLoading} onClick={handleDeletion} variant="destructive">
                  确认删除
                </Button>
              </div>
            ) : (
              <ul className="list-disc pl-5">
                {deleteCheck.blockers.map((blocker) => (
                  <li key={`${blocker.code}-${blocker.resource_id}`}>{blocker.summary}</li>
                ))}
              </ul>
            )}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
