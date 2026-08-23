"use client";

import { useEffect, useMemo, useState } from "react";

import { authLogin, authLogout, authRefresh, authRegister } from "@/api/auth";
import { operationGet } from "@/api/operation";
import { projectCreate } from "@/api/project";
import {
  scriptAnalysisApprove,
  scriptAnalysisDraft,
  scriptAnalysisQueue,
  scriptRevisionCreate,
} from "@/api/script";
import { ApiClientError, setAccessToken } from "@/lib/request";

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
type AuthenticatedWorkspace = { id: string; name: string };

function sleep(milliseconds: number) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
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
  const [scriptName, setScriptName] = useState("首轮试点剧本");
  const [content, setContent] = useState(fixtureScript);
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [analysis, setAnalysis] = useState<Analysis | null>(null);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [revisionID, setRevisionID] = useState<string | null>(null);
  const [projectID, setProjectID] = useState<string | null>(null);
  const [phase, setPhase] = useState<"idle" | "queued" | "draft" | "approved">("idle");
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
      .then((response) => {
        const identity = response.data;
        if (!active || !identity?.access_token || !identity.workspace?.id) return;
        setAccessToken(identity.access_token);
        setWorkspace({ id: identity.workspace.id, name: identity.workspace.name ?? "当前工作区" });
      })
      .catch(() => {
        if (active) setAccessToken();
      })
      .finally(() => {
        if (active) setRestoringSession(false);
      });
    return () => {
      active = false;
    };
  }, []);

  function resetSession(message: string | null) {
    setAccessToken();
    setWorkspace(null);
    setAuthMode("login");
    setPassword("");
    setAnalysis(null);
    setOperation(null);
    setRevisionID(null);
    setProjectID(null);
    setPhase("idle");
    setError(message);
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
      setPassword("");
      setWorkspace({ id: identity.workspace.id, name: identity.workspace.name ?? "当前工作区" });
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
    try {
      if (!workspace) {
        throw new Error("登录会话缺失，请重新登录。");
      }
      const project = await projectCreate({ workspaceID: workspace.id }, { name: "剧本事实分析项目" });
      if (!project.data?.id) {
        throw new Error("创建项目后未返回项目 ID。");
      }
      const source = sourceFile ?? new File(
        [content],
        `${scriptName.trim() || "未命名剧本"}.txt`,
        { type: "text/plain;charset=utf-8" },
      );
      const revision = await scriptRevisionCreate({ projectID: project.data.id }, {}, source);
      if (!revision.data?.id) {
        throw new Error("创建剧本版本后未返回版本 ID。");
      }
      setProjectID(project.data.id);
      setRevisionID(revision.data.id);
      const queued = await scriptAnalysisQueue({ revisionID: revision.data.id });
      if (!queued.data?.id) {
        throw new Error("剧本解析任务未返回 Operation ID。");
      }
      setOperation(queued.data);
      setPhase("queued");

      let latest = queued.data;
      for (let attempt = 0; attempt < 80; attempt += 1) {
        await sleep(250);
        const response = await operationGet({ operationID: latest.id });
        if (!response.data?.id) {
          throw new Error("Operation 查询返回无效结果。");
        }
        latest = response.data;
        setOperation(latest);
        if (latest.status === "succeeded" || latest.status === "failed") break;
      }
      if (latest.status !== "succeeded") {
        throw new Error(latest.error ?? "剧本解析任务未成功完成");
      }
      const draft = await scriptAnalysisDraft({ revisionID: revision.data.id });
      if (!draft.data) {
        throw new Error("解析完成但未返回可审阅草稿。");
      }
      setAnalysis(draft.data);
      setPhase("draft");
    } catch (cause) {
      reportAuthenticatedFailure(cause, "剧本解析失败，请查看任务状态。");
    } finally {
      setBusy(false);
    }
  }

  async function approve() {
    if (!revisionID) return;
    setBusy(true);
    setError(null);
    try {
      const response = await scriptAnalysisApprove({ revisionID });
      if (!response.data) {
        throw new Error("批准完成但未返回正式分析事实。");
      }
      setAnalysis(response.data);
      setPhase("approved");
    } catch (cause) {
      reportAuthenticatedFailure(cause, "批准失败，请修正当前事实后重试。");
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
              {phase === "draft" && "待人工批准"}
              {phase === "approved" && "事实已批准"}
            </div>
            <div className="actions" style={{ marginTop: 10, justifyContent: "flex-end" }}>
              <span className="hint">{workspace.name}</span>
              <button className="secondary" type="button" onClick={logout} disabled={busy}>退出登录</button>
            </div>
          </div>
        </header>

        <div className="grid">
          <section className="card" aria-labelledby="source-title">
            <h2 id="source-title">1. 导入整本剧本</h2>
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
              {phase === "draft" && <button className="secondary" type="button" onClick={approve} disabled={busy} data-testid="approve-button">批准事实并物化</button>}
            </div>
            {error && <div className="error" role="alert">{error}</div>}
            {phase === "approved" && <div className="success" role="status">已批准。剧集、叙事单元、资产和生产需求已在同一事务中物化。</div>}
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
              <div className="metric"><strong>{assets.length}</strong><span>生产资产</span></div>
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

        {analysis && <section className="card" style={{ marginTop: 18 }} aria-labelledby="result-title">
          <h2 id="result-title">3. 剧集、场景与资产</h2>
          <div className="asset-list" role="list" aria-label="资产清单">
            {assets.map((asset) => <span className="asset" role="listitem" key={`${asset.kind}-${asset.name}`}>{asset.name}<em>{asset.kind} · {(asset.episode_numbers ?? []).join(", ")}</em></span>)}
          </div>
          {(analysis.episodes ?? []).map((episode) => <article className="episode" key={episode.number}>
            <div className="episode-head"><span className="episode-title">第 {episode.number} 集 · {episode.title}</span><span className="hint">{episode.scenes?.length ?? 0} 个场景</span></div>
            {(episode.scenes ?? []).map((scene) => <div className="scene" key={scene.id}><strong>{scene.heading}</strong><p>{(scene.narratives ?? []).slice(0, 4).map((narrative) => narrative.text).join(" · ")}</p></div>)}
          </article>)}
        </section>}
      </div>
    </main>
  );
}
