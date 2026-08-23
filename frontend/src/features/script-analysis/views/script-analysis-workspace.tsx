"use client";

import { useEffect, useMemo, useState } from "react";

import { authLogin, authLogout, authRefresh, authRegister } from "@/api/auth";
import { operationGet } from "@/api/operation";
import { projectAnalysisGet, projectCreate, projectList } from "@/api/project";
import {
  scriptAnalysisApprove,
  scriptAnalysisDraft,
  scriptAnalysisDraftRevise,
  scriptAnalysisQueue,
  scriptRevisionCreate,
} from "@/api/script";
import { ApiClientError, setAccessToken } from "@/lib/request";
import { EpisodeBreakdownEditor } from "@/features/script-analysis/views/episode-breakdown-editor";

const fixtureScript = `第1集 归途
场景：海边码头
人物：林夏、顾远
道具：旧怀表
服装：雨衣
林夏：我们必须马上离开。
顾远：怀表还在这里。

第2集 回声
场景：旧仓库
人物：林夏
道具：手电筒
服装：黑色外套
林夏：你换了雨衣。

第3集 终局
场景：山顶
人物：顾远
道具：信封
服装：风衣
顾远：一切结束了。`;

type Analysis = API.Analysis;
type Operation = API.Operation;
type Project = API.Project;
type AuthenticatedWorkspace = { id: string; name: string };
type WorkflowLocator = { projectID: string; revisionID: string; operationID: string };
type WorkflowPhase = "idle" | "queued" | "draft" | "approved";

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function clearWorkflowLocator() {
  const url = new URL(window.location.href);
  url.searchParams.delete("project");
  url.searchParams.delete("revision");
  url.searchParams.delete("operation");
  window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
}

function readWorkflowLocator(): WorkflowLocator | null {
  const params = new URLSearchParams(window.location.search);
  const locator = {
    projectID: params.get("project") ?? "",
    revisionID: params.get("revision") ?? "",
    operationID: params.get("operation") ?? "",
  };
  if (Object.values(locator).every((value) => uuidPattern.test(value))) return locator;
  if (Object.values(locator).some(Boolean)) clearWorkflowLocator();
  return null;
}

function writeWorkflowLocator(locator: WorkflowLocator) {
  const url = new URL(window.location.href);
  url.searchParams.set("project", locator.projectID);
  url.searchParams.set("revision", locator.revisionID);
  url.searchParams.set("operation", locator.operationID);
  window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
}

function sleep(milliseconds: number) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function waitForOperation(initial: Operation, onProgress: (operation: Operation) => void) {
  let latest = initial;
  for (let attempt = 0; attempt < 80 && latest.status !== "succeeded" && latest.status !== "failed"; attempt += 1) {
    await sleep(250);
    if (!latest.id) throw new Error("Operation 缺少可恢复 ID。");
    const response = await operationGet({ operationID: latest.id });
    if (!response.data?.id) throw new Error("Operation 查询返回无效结果。");
    latest = response.data;
    onProgress(latest);
  }
  return latest;
}

async function restoreWorkflow(locator: WorkflowLocator, onProgress: (operation: Operation) => void): Promise<{ analysis: Analysis; operation: Operation; phase: Exclude<WorkflowPhase, "idle" | "queued"> }> {
  const response = await operationGet({ operationID: locator.operationID });
  if (!response.data?.id || response.data.project_id !== locator.projectID || response.data.source_revision_id !== locator.revisionID) {
    throw new ApiClientError("无法恢复当前剧本工作流", "not_found", 404);
  }
  onProgress(response.data);
  const latest = await waitForOperation(response.data, onProgress);
  if (latest.status !== "succeeded") {
    throw new Error(latest.error ?? "剧本解析任务未成功完成");
  }
  try {
    const approved = await projectAnalysisGet({ projectID: locator.projectID });
    if (!approved.data) throw new Error("项目分析查询返回无效结果。");
    return { analysis: approved.data, operation: latest, phase: "approved" };
  } catch (cause) {
    if (!(cause instanceof ApiClientError) || cause.status !== 404) throw cause;
  }
  const draft = await scriptAnalysisDraft({ revisionID: locator.revisionID });
  if (!draft.data) throw new Error("分析草稿查询返回无效结果。");
  return { analysis: draft.data, operation: latest, phase: "draft" };
}

function projectWorkflowLocator(project: Project): WorkflowLocator | null {
  const workflow = project.latest_workflow;
  const locator = {
    projectID: workflow?.project_id ?? "",
    revisionID: workflow?.source_revision_id ?? "",
    operationID: workflow?.operation_id ?? "",
  };
  if (project.id !== locator.projectID) return null;
  return Object.values(locator).every((value) => uuidPattern.test(value)) ? locator : null;
}

async function listWorkspaceProjects(workspaceID: string, page: number) {
  const response = await projectList({ workspaceID, page, page_size: 20 });
  return {
    items: response.data?.items ?? [],
    page: response.data?.page ?? page,
    total: response.data?.total ?? 0,
  };
}

function userFacingError(cause: unknown, fallback: string) {
  if (!(cause instanceof Error)) return fallback;
  if (cause instanceof ApiClientError && cause.nextAction) {
    return `${cause.message}。下一步：${cause.nextAction}`;
  }
  return cause.message;
}

let restorePromise: ReturnType<typeof authRefresh> | null = null;

function restoreAuthSession() {
  if (restorePromise) return restorePromise;
  const current = authRefresh();
  restorePromise = current;
  const clear = () => {
    if (restorePromise === current) restorePromise = null;
  };
  void current.then(clear, clear);
  return current;
}

export function ScriptAnalysisWorkspace() {
  const [authMode, setAuthMode] = useState<"register" | "login">("register");
  const [restoringSession, setRestoringSession] = useState(true);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [workspaceName, setWorkspaceName] = useState("");
  const [workspace, setWorkspace] = useState<AuthenticatedWorkspace | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectPage, setProjectPage] = useState(1);
  const [projectTotal, setProjectTotal] = useState(0);
  const [projectListLoading, setProjectListLoading] = useState(false);
  const [projectListError, setProjectListError] = useState<string | null>(null);
  const [projectName, setProjectName] = useState("剧本事实分析项目");
  const [scriptName, setScriptName] = useState("首轮试点剧本");
  const [content, setContent] = useState(fixtureScript);
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [analysis, setAnalysis] = useState<Analysis | null>(null);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [revisionID, setRevisionID] = useState<string | null>(null);
  const [projectID, setProjectID] = useState<string | null>(null);
  const [phase, setPhase] = useState<WorkflowPhase>("idle");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const assets = useMemo(
    () => analysis ? [
      ...(analysis.characters ?? []),
      ...(analysis.locations ?? []),
      ...(analysis.props ?? []),
      ...(analysis.costumes ?? []),
    ] : [],
    [analysis],
  );

  useEffect(() => {
    let active = true;
    restoreAuthSession()
      .then(async (response) => {
        const identity = response.data;
        if (!active || !identity?.access_token || !identity.workspace?.id) return;
        setAccessToken(identity.access_token);
        setWorkspace({ id: identity.workspace.id, name: identity.workspace.name ?? "当前工作区" });
        setProjectListLoading(true);
        setProjectListError(null);
        let listedProjects: Project[] = [];
        try {
          const projectPage = await listWorkspaceProjects(identity.workspace.id, 1);
          listedProjects = projectPage.items;
          if (active) {
            setProjects(listedProjects);
            setProjectPage(projectPage.page);
            setProjectTotal(projectPage.total);
          }
        } catch (cause) {
          if (!active) return;
          if (cause instanceof ApiClientError && cause.status === 401) throw cause;
          setProjectListError(userFacingError(cause, "项目列表加载失败，请重试。"));
        } finally {
          if (active) setProjectListLoading(false);
        }
        const locator = readWorkflowLocator();
        if (!locator) return;
        setProjectID(locator.projectID);
        const locatedProject = listedProjects.find((project) => project.id === locator.projectID);
        if (locatedProject?.name) setProjectName(locatedProject.name);
        setRevisionID(locator.revisionID);
        try {
          const restored = await restoreWorkflow(locator, (nextOperation) => {
            if (active) setOperation(nextOperation);
          });
          if (!active) return;
          setOperation(restored.operation);
          setAnalysis(restored.analysis);
          setPhase(restored.phase);
        } catch (cause) {
          if (!active) return;
          clearWorkflowLocator();
          setProjectID(null);
          setRevisionID(null);
          setOperation(null);
          setAnalysis(null);
          setPhase("idle");
          setError(userFacingError(cause, "无法恢复上次剧本工作流，请从项目入口重新打开。"));
        }
      })
      .catch(() => {
        if (active) {
          setAccessToken();
          setWorkspace(null);
          setProjects([]);
          setProjectPage(1);
          setProjectTotal(0);
        }
      })
      .finally(() => {
        if (active) setRestoringSession(false);
      });
    return () => {
      active = false;
    };
  }, []);

  function resetSession(message: string | null) {
    clearWorkflowLocator();
    setAccessToken();
    setWorkspace(null);
    setProjects([]);
    setProjectPage(1);
    setProjectTotal(0);
    setProjectListError(null);
    setAuthMode("login");
    setPassword("");
    setAnalysis(null);
    setOperation(null);
    setRevisionID(null);
    setProjectID(null);
    setPhase("idle");
    setError(message);
  }

  async function reloadProjects(workspaceID: string, page = projectPage) {
    setProjectListLoading(true);
    setProjectListError(null);
    try {
      const projectPage = await listWorkspaceProjects(workspaceID, page);
      setProjects(projectPage.items);
      setProjectPage(projectPage.page);
      setProjectTotal(projectPage.total);
    } catch (cause) {
      if (cause instanceof ApiClientError && cause.status === 401) {
        resetSession("登录会话已失效，请重新登录。");
        return;
      }
      setProjectListError(userFacingError(cause, "项目列表加载失败，请重试。"));
    } finally {
      setProjectListLoading(false);
    }
  }

  async function openProject(project: Project) {
    if (!project.id) return;
    const locator = projectWorkflowLocator(project);
    setError(null);
    setProjectID(project.id);
    if (project.name) setProjectName(project.name);
    setRevisionID(locator?.revisionID ?? null);
    setAnalysis(null);
    setOperation(null);
    setPhase("idle");
    if (!locator) {
      clearWorkflowLocator();
      return;
    }

    setBusy(true);
    writeWorkflowLocator(locator);
    try {
      const restored = await restoreWorkflow(locator, setOperation);
      setOperation(restored.operation);
      setAnalysis(restored.analysis);
      setPhase(restored.phase);
    } catch (cause) {
      clearWorkflowLocator();
      reportAuthenticatedFailure(cause, "无法恢复所选项目的剧本工作流。可在该项目中导入新版本。");
    } finally {
      setBusy(false);
    }
  }

  function startNewProject() {
    clearWorkflowLocator();
    setProjectID(null);
    setRevisionID(null);
    setOperation(null);
    setAnalysis(null);
    setPhase("idle");
    setError(null);
  }

  function reportAuthenticatedFailure(cause: unknown, fallback: string) {
    if (cause instanceof ApiClientError && cause.status === 401) {
      resetSession("登录会话已失效，请重新登录。");
      return;
    }
    setError(userFacingError(cause, fallback));
  }

  async function authenticate() {
    setBusy(true);
    setError(null);
    try {
      const response = authMode === "register"
        ? await authRegister({
          email: email.trim(),
          password,
          display_name: displayName.trim(),
          workspace_name: workspaceName.trim(),
        })
        : await authLogin({ email: email.trim(), password });
      const identity = response.data;
      if (!identity?.access_token || !identity.workspace?.id) {
        throw new Error("认证响应缺少访问令牌或工作区，请联系管理员。");
      }
      setAccessToken(identity.access_token);
      clearWorkflowLocator();
      setPassword("");
      setWorkspace({ id: identity.workspace.id, name: identity.workspace.name ?? "当前工作区" });
      await reloadProjects(identity.workspace.id, 1);
    } catch (cause) {
      setError(userFacingError(cause, "认证失败，请检查邮箱和密码。"));
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    setBusy(true);
    setError(null);
    try {
      await authLogout();
      resetSession(null);
    } catch (cause) {
      setError(userFacingError(cause, "退出失败，请重试。"));
    } finally {
      setBusy(false);
    }
  }

  async function submitForAnalysis() {
    setBusy(true);
    setError(null);
    setAnalysis(null);
    setPhase("idle");
    clearWorkflowLocator();
    try {
      if (!workspace) {
        throw new Error("登录会话缺失，请重新登录。");
      }
      let targetProjectID = projectID;
      if (!targetProjectID) {
        const project = await projectCreate(
          { workspaceID: workspace.id },
          { name: projectName.trim() || "剧本事实分析项目" },
        );
        if (!project.data?.id) {
          throw new Error("创建项目后未返回项目 ID。");
        }
        targetProjectID = project.data.id;
      }
      const source = sourceFile ?? new File(
        [content],
        `${scriptName.trim() || "未命名剧本"}.txt`,
        { type: "text/plain;charset=utf-8" },
      );
      const revision = await scriptRevisionCreate({ projectID: targetProjectID }, {}, source);
      if (!revision.data?.id) {
        throw new Error("创建剧本版本后未返回版本 ID。");
      }
      setProjectID(targetProjectID);
      setRevisionID(revision.data.id);
      const queued = await scriptAnalysisQueue({ revisionID: revision.data.id });
      if (!queued.data?.id) {
        throw new Error("剧本解析任务未返回 Operation ID。");
      }
      setOperation(queued.data);
      setPhase("queued");
      writeWorkflowLocator({ projectID: targetProjectID, revisionID: revision.data.id, operationID: queued.data.id });

      const latest = await waitForOperation(queued.data, setOperation);
      if (latest.status !== "succeeded") {
        throw new Error(latest.error ?? "剧本解析任务未成功完成");
      }
      const draft = await scriptAnalysisDraft({ revisionID: revision.data.id });
      if (!draft.data) {
        throw new Error("解析完成但未返回可审阅草稿。");
      }
      setAnalysis(draft.data);
      setPhase("draft");
      await reloadProjects(workspace.id, 1);
    } catch (cause) {
      reportAuthenticatedFailure(cause, "剧本解析失败，请查看任务状态。");
    } finally {
      setBusy(false);
    }
  }

  async function approve() {
    if (!revisionID || analysis?.breakdown?.status !== "ready") return;
    setBusy(true);
    setError(null);
    try {
      const response = await scriptAnalysisApprove({ revisionID });
      if (!response.data) {
        throw new Error("批准完成但未返回正式分析事实。");
      }
      setAnalysis(response.data);
      setPhase("approved");
      if (workspace) await reloadProjects(workspace.id, 1);
    } catch (cause) {
      reportAuthenticatedFailure(cause, "批准失败，请修正当前事实后重试。");
    } finally {
      setBusy(false);
    }
  }

  async function reviseBreakdown(operations: API.EpisodeBreakdownOperation[]) {
    if (!revisionID || !analysis?.source_hash) return;
    setBusy(true);
    setError(null);
    try {
      const response = await scriptAnalysisDraftRevise(
        { revisionID },
        { expected_source_hash: analysis.source_hash, operations },
      );
      if (!response.data) {
        throw new Error("拆解修订完成但未返回新版本。");
      }
      setAnalysis(response.data);
      setPhase("draft");
    } catch (cause) {
      reportAuthenticatedFailure(cause, "剧集拆解修订失败，请刷新当前基线后重试。");
    } finally {
      setBusy(false);
    }
  }

  if (restoringSession) {
    return <main className="page"><div className="shell"><div className="status" role="status">正在恢复登录会话…</div></div></main>;
  }

  if (!workspace) {
    return (
      <main className="page">
        <div className="shell">
          <section className="card" aria-labelledby="auth-title">
            <div className="eyebrow">Lanverse · Identity Gate</div>
            <h1 id="auth-title">登录后开始剧本事实分析</h1>
            <p>认证工作区决定后续项目、剧本、任务和媒体的租户边界。</p>
            <div className="actions" role="group" aria-label="认证方式">
              <button className={authMode === "register" ? "primary" : "secondary"} type="button" onClick={() => setAuthMode("register")}>注册</button>
              <button className={authMode === "login" ? "primary" : "secondary"} type="button" onClick={() => setAuthMode("login")}>登录</button>
            </div>
            <div className="field">
              <label htmlFor="auth-email">邮箱</label>
              <input id="auth-email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} />
            </div>
            <div className="field">
              <label htmlFor="auth-password">密码</label>
              <input id="auth-password" type="password" autoComplete={authMode === "register" ? "new-password" : "current-password"} value={password} onChange={(event) => setPassword(event.target.value)} />
            </div>
            {authMode === "register" && <>
              <div className="field">
                <label htmlFor="display-name">显示名称</label>
                <input id="display-name" value={displayName} onChange={(event) => setDisplayName(event.target.value)} />
              </div>
              <div className="field">
                <label htmlFor="workspace-name">工作区名称</label>
                <input id="workspace-name" value={workspaceName} onChange={(event) => setWorkspaceName(event.target.value)} />
              </div>
            </>}
            <div className="actions">
              <button className="primary" type="button" onClick={authenticate} disabled={busy || !email.trim() || !password || (authMode === "register" && (!displayName.trim() || !workspaceName.trim()))}>
                {busy ? "正在认证…" : authMode === "register" ? "注册并进入" : "登录并进入"}
              </button>
            </div>
            {error && <div className="error" role="alert">{error}</div>}
          </section>
        </div>
      </main>
    );
  }

  return (
    <main className="page">
      <div className="shell">
        <header className="hero">
          <div className="hero-copy">
            <div className="eyebrow">Lanverse · Script Fact Line</div>
            <h1>先把整本剧本，变成可核对的事实。</h1>
            <p>从来源保全开始，逐步拆出剧集、场景、人物、道具和服装。每一个结果都保留原文 Anchor，批准前不会写入正式生产事实。</p>
          </div>
          <div>
            <div className="status" data-testid="phase-status">
              {phase === "idle" && "等待导入"}
              {phase === "queued" && `解析中 ${operation?.progress ?? 0}%`}
              {phase === "draft" && "剧集边界待校对"}
              {phase === "approved" && "叙事已批准 · 知识待决议"}
            </div>
            <div className="actions" style={{ marginTop: 10, justifyContent: "flex-end" }}>
              <span className="hint">{workspace.name}</span>
              <button className="secondary" type="button" onClick={logout} disabled={busy}>退出登录</button>
            </div>
          </div>
        </header>

        <section className="card project-browser" aria-labelledby="project-browser-title">
          <div className="section-heading">
            <div>
              <h2 id="project-browser-title">继续已有项目</h2>
              <p>项目和最近一次剧本工作流来自当前 Workspace 的服务端事实，可在刷新或更换设备后继续。</p>
            </div>
            <button className="secondary" type="button" onClick={startNewProject} disabled={busy || projectID === null}>创建新项目</button>
          </div>
          {projectListLoading && <div className="hint" role="status">正在加载项目…</div>}
          {projectListError && <div className="error" role="alert">
            {projectListError}
            <button className="secondary inline-action" type="button" onClick={() => void reloadProjects(workspace.id)}>重试</button>
          </div>}
          {!projectListLoading && !projectListError && projects.length === 0 && <p>当前 Workspace 暂无项目。首次提交解析任务时会创建一个新项目。</p>}
          {projects.length > 0 && <ul className="project-list">
            {projects.map((project) => {
              if (!project.id) return null;
              const locator = projectWorkflowLocator(project);
              const selected = project.id === projectID;
              return <li key={project.id} className={selected ? "selected" : undefined}>
                <div>
                  <strong>{project.name ?? "未命名项目"}</strong>
                  <div className="hint">
                    {locator
                      ? `${project.latest_workflow?.source_status ?? "unknown"} · ${project.latest_workflow?.operation_status ?? "unknown"} · ${project.latest_workflow?.progress ?? 0}%`
                      : "尚无可恢复的剧本工作流"}
                  </div>
                </div>
                <button
                  className={selected ? "primary" : "secondary"}
                  type="button"
                  aria-label={`${locator ? "继续解析" : "在项目中导入"} ${project.name ?? "未命名项目"}`}
                  onClick={() => void openProject(project)}
                  disabled={busy}
                >
                  {selected ? "当前项目" : locator ? "继续" : "选择"}
                </button>
              </li>;
            })}
          </ul>}
          {projectTotal > 0 && <nav className="project-pagination" aria-label="项目列表分页">
            <span className="hint">共 {projectTotal} 个项目 · 第 {projectPage} 页</span>
            <div className="actions">
              <button className="secondary" type="button" disabled={busy || projectListLoading || projectPage <= 1} onClick={() => void reloadProjects(workspace.id, projectPage - 1)}>上一页</button>
              <button className="secondary" type="button" disabled={busy || projectListLoading || projectPage * 20 >= projectTotal} onClick={() => void reloadProjects(workspace.id, projectPage + 1)}>下一页</button>
            </div>
          </nav>}
        </section>

        <div className="grid">
          <section className="card" aria-labelledby="source-title">
            <h2 id="source-title">1. 导入整本剧本</h2>
            <div className="field">
              <label htmlFor="project-name">项目名称</label>
              <input
                id="project-name"
                value={projectName}
                disabled={projectID !== null}
                onChange={(event) => setProjectName(event.target.value)}
              />
              <span className="hint">{projectID ? "新版本将导入当前选中项目；如需新项目，请先点击“创建新项目”。" : "尚未选择已有项目，提交时会创建此项目。"}</span>
            </div>
            <div className="field">
              <label htmlFor="script-file">剧本文件</label>
              <input
                id="script-file"
                type="file"
                accept=".docx,.md,.markdown,.txt,text/markdown,text/plain,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
                onChange={(event) => {
                  const nextFile = event.target.files?.item(0) ?? null;
                  setSourceFile(nextFile);
                  if (nextFile) setContent("");
                }}
              />
              <span className="hint">支持 DOCX、Markdown 与 UTF-8 TXT；原件按原始字节计算 hash 并保全到 MinIO。</span>
            </div>
            <div className="field">
              <label htmlFor="script-name">粘贴文本名称</label>
              <input id="script-name" value={scriptName} onChange={(event) => setScriptName(event.target.value)} disabled={sourceFile !== null} />
              <label htmlFor="script-content">或粘贴 UTF-8 纯文本</label>
              <textarea
                id="script-content"
                value={content}
                disabled={sourceFile !== null}
                onChange={(event) => {
                  setSourceFile(null);
                  setContent(event.target.value);
                }}
              />
            </div>
            <div className="actions">
              <button className="primary" type="button" onClick={submitForAnalysis} disabled={busy || (!sourceFile && !content.trim())} data-testid="analyze-button">
                {busy && phase !== "draft" ? "正在解析…" : "提交解析任务"}
              </button>
              {phase === "draft" && <button className="secondary" type="button" onClick={approve} disabled={busy || analysis?.breakdown?.status !== "ready"} data-testid="approve-button">批准当前拆解与叙事</button>}
            </div>
            {error && <div className="error" role="alert">{error}</div>}
            {phase === "approved" && <div className="success" role="status">叙事已批准，ProductionElementMention 已冻结；实体与生产需求仍待知识决议。</div>}
          </section>

          <section className="card" aria-labelledby="operation-title">
            <h2 id="operation-title">2. 任务与事实状态</h2>
            {!operation && <p>提交后会在这里显示 Operation、解析进度和失败后的下一动作。</p>}
            {operation && <div className="operation" data-testid="operation-status">
              <div><strong>{operation.type}</strong><div className="hint">{operation.id}</div></div>
              <span>{operation.status}</span>
            </div>}
            {operation && <div className="bar" role="progressbar" aria-label="解析进度" aria-valuemin={0} aria-valuemax={100} aria-valuenow={operation.progress ?? 0}><span style={{ width: `${operation.progress ?? 0}%` }} /></div>}
            {projectID && <div className="hint" style={{ marginTop: 12 }}>Project：{projectID}</div>}
            {analysis && <div className="summary">
              <div className="metric"><strong>{analysis.episodes?.length ?? 0}</strong><span>剧集</span></div>
              <div className="metric"><strong>{(analysis.episodes ?? []).reduce((total, episode) => total + (episode.scenes?.length ?? 0), 0)}</strong><span>场景</span></div>
              <div className="metric"><strong>{analysis.characters?.length ?? 0}</strong><span>人物</span></div>
              <div className="metric"><strong>{assets.length}</strong><span>来源 Mention</span></div>
            </div>}
            {analysis?.parse_report && <div className="operation" data-testid="parse-report">
              <div>
                <strong>{analysis.parse_report.format?.toUpperCase()} · {analysis.parse_report.paragraph_count ?? 0} 段 · {analysis.parse_report.character_count ?? 0} 字符</strong>
                <div className="hint">Parser：{analysis.parse_report.parser_version}</div>
              </div>
              <span>{(analysis.parse_report.failed_scopes?.length ?? 0) === 0 ? "解析完整，无失败范围" : `${analysis.parse_report.failed_scopes?.length} 个失败范围`}</span>
            </div>}
          </section>
        </div>

        {analysis?.breakdown && <EpisodeBreakdownEditor analysis={analysis} disabled={busy} editable={phase === "draft"} onRevise={reviseBreakdown} />}

        {analysis && <section className="card" style={{ marginTop: 18 }} aria-labelledby="result-title">
          <h2 id="result-title">{analysis.breakdown ? "4" : "3"}. 场景与来源 Mention</h2>
          <div className="asset-list" role="list" aria-label="来源 Mention 清单">
            {assets.map((asset) => <span className="asset" role="listitem" key={`${asset.kind}-${asset.name}`}>{asset.name}<em>{asset.kind} · {(asset.episode_numbers ?? []).join(", ")}</em></span>)}
          </div>
          {(analysis.episodes ?? []).filter((episode) => episode.decision !== "ignored").map((episode) => <article className="episode" key={episode.temporary_key ?? episode.number}>
            <div className="episode-head"><span className="episode-title">第 {episode.number} 集 · {episode.title}</span><span className="hint">{episode.scenes?.length ?? 0} 个场景</span></div>
            {(episode.scenes ?? []).map((scene) => <div className="scene" key={scene.id}><strong>{scene.heading}</strong><p>{(scene.narratives ?? []).slice(0, 4).map((narrative) => narrative.text).join(" · ")}</p></div>)}
          </article>)}
        </section>}
      </div>
    </main>
  );
}
