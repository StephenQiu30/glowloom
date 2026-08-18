"use client";

import { Archive, RotateCcw, Save, Trash2 } from "lucide-react";
import { type FormEvent, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  appApiErrorMessage,
  useDeleteProjectMutation,
  useProjectDeletePreflightMutation,
  useSetProjectArchivedMutation,
  useUpdateProjectMutation,
} from "@/lib/server-state";

export function ProjectLifecyclePanel({ project }: { project: API.ProjectResponse }) {
  const [message, setMessage] = useState<string | null>(null);
  const [commandError, setCommandError] = useState<string | null>(null);
  const [deleteCheck, setDeleteCheck] = useState<API.DeletePreflightResponse | null>(null);
  const [updateProject, updateState] = useUpdateProjectMutation();
  const [setArchived, archiveState] = useSetProjectArchivedMutation();
  const [deletePreflight] = useProjectDeletePreflightMutation();
  const [deleteProject, deleteState] = useDeleteProjectMutation();

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

  async function handleProjectUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runCommand(
      () =>
        updateProject({
          projectId: project.id,
          body: {
            name: String(form.get("projectName")),
            description: String(form.get("projectDescription")) || null,
            aspect_ratio: String(form.get("aspectRatio")) as API.ProjectUpdateRequest["aspect_ratio"],
            language: String(form.get("language")),
            visual_style: project.visual_style,
            target_duration_ms: Number(form.get("targetDurationSeconds")) * 1000,
            expected_revision: project.revision,
          },
        }).unwrap(),
      "项目信息已更新。",
    );
  }

  async function handleProjectState() {
    await runCommand(
      () =>
        setArchived({
          projectId: project.id,
          expectedRevision: project.revision,
          archived: project.status === "active",
        }).unwrap(),
      project.status === "active" ? "项目已归档。" : "项目已恢复。",
    );
  }

  async function checkDeletion() {
    setDeleteCheck(null);
    const succeeded = await runCommand(async () => {
      setDeleteCheck(await deletePreflight(project.id).unwrap());
    }, "项目删除条件已检查。");
    if (!succeeded) setDeleteCheck(null);
  }

  async function handleDeletion() {
    const succeeded = await runCommand(
      () =>
        deleteProject({
          projectId: project.id,
          expectedRevision: project.revision,
        }).unwrap(),
      "项目已删除。",
    );
    if (succeeded) window.location.replace("/projects");
  }

  return (
    <section className="mt-9 grid gap-5" aria-label="项目设置">
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
      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>项目信息</CardTitle>
            <CardDescription>更新会携带当前 revision，冲突时不会覆盖他人修改。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="grid gap-4" key={`project-${project.revision}`} onSubmit={handleProjectUpdate}>
              <div className="grid gap-2">
                <Label htmlFor="projectName">项目名称</Label>
                <Input defaultValue={project.name} disabled={project.status === "archived"} id="projectName" maxLength={120} name="projectName" required />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="projectDescription">项目简介</Label>
                <Input defaultValue={project.description ?? ""} disabled={project.status === "archived"} id="projectDescription" maxLength={2000} name="projectDescription" />
              </div>
              <div className="grid gap-4 sm:grid-cols-3">
                <div className="grid gap-2">
                  <Label htmlFor="aspectRatio">画幅</Label>
                  <Select defaultValue={project.aspect_ratio} disabled={project.status === "archived"} name="aspectRatio">
                    <SelectTrigger className="w-full" id="aspectRatio"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="9:16">9:16</SelectItem>
                      <SelectItem value="16:9">16:9</SelectItem>
                      <SelectItem value="1:1">1:1</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="language">语言</Label>
                  <Input defaultValue={project.language} disabled={project.status === "archived"} id="language" maxLength={35} name="language" required />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="targetDurationSeconds">目标秒数</Label>
                  <Input defaultValue={project.target_duration_ms / 1000} disabled={project.status === "archived"} id="targetDurationSeconds" max={7200} min={1} name="targetDurationSeconds" required type="number" />
                </div>
              </div>
              <Button disabled={project.status === "archived" || updateState.isLoading} type="submit">
                <Save aria-hidden="true" />
                保存项目信息
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>项目生命周期</CardTitle>
            <CardDescription>归档、恢复或删除项目；删除前必须先通过预检。</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-5">
            <div className="flex flex-wrap gap-2 pt-2">
              <Button disabled={archiveState.isLoading} onClick={handleProjectState} variant="outline">
                {project.status === "active" ? <Archive aria-hidden="true" /> : <RotateCcw aria-hidden="true" />}
                {project.status === "active" ? "归档项目" : "恢复项目"}
              </Button>
              <Button onClick={checkDeletion} variant="destructive">
                <Trash2 aria-hidden="true" />
                检查项目删除条件
              </Button>
            </div>
            {deleteCheck ? (
              <div className="rounded-lg bg-muted p-3 text-sm">
                {deleteCheck.allowed ? (
                  <div className="grid gap-3">
                    <p>当前项目是无引用空草稿，可以安全删除。</p>
                    <Button disabled={deleteState.isLoading} onClick={handleDeletion} variant="destructive">确认删除项目</Button>
                  </div>
                ) : (
                  <div>
                    <p className="font-medium">项目暂时不能删除：</p>
                    <ul className="mt-2 list-disc pl-5">
                      {deleteCheck.blockers.map((blocker) => (
                        <li key={`${blocker.code}-${blocker.resource_id}`}>{blocker.summary}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
