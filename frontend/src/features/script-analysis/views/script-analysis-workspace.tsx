"use client";

import { useMemo, useState } from "react";

import { approveScriptAnalysis } from "@/api/approveScriptAnalysis";
import { createProject } from "@/api/createProject";
import { createScriptRevision } from "@/api/createScriptRevision";
import { createSession } from "@/api/createSession";
import { createWorkspace } from "@/api/createWorkspace";
import { getAnalysisDraft } from "@/api/getAnalysisDraft";
import { getOperation } from "@/api/getOperation";
import { queueScriptAnalysis } from "@/api/queueScriptAnalysis";

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

function sleep(milliseconds: number) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

export function ScriptAnalysisWorkspace() {
  const [scriptName, setScriptName] = useState("首轮试点剧本");
  const [content, setContent] = useState(fixtureScript);
  const [analysis, setAnalysis] = useState<Analysis | null>(null);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [revisionID, setRevisionID] = useState<string | null>(null);
  const [projectID, setProjectID] = useState<string | null>(null);
  const [phase, setPhase] = useState<"idle" | "queued" | "draft" | "approved">("idle");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const assets = useMemo(
    () => analysis ? [
      ...analysis.characters,
      ...analysis.locations,
      ...analysis.props,
      ...analysis.costumes,
    ] : [],
    [analysis],
  );

  async function submitForAnalysis() {
    setBusy(true);
    setError(null);
    setAnalysis(null);
    setPhase("idle");
    try {
      const workspace = await createWorkspace({ name: "首轮试点工作区" });
      const session = await createSession({ identity_subject: "local-owner", workspace_id: workspace.data.id });
      window.localStorage.setItem("lanverse.workspace_id", workspace.data.id);
      window.localStorage.setItem("lanverse.session_token", session.data.token);
      const project = await createProject({ workspace_id: workspace.data.id }, { name: "剧本事实分析项目" });
      const revision = await createScriptRevision(
        { project_id: project.data.id },
        { name: scriptName.trim() || "未命名剧本", content },
      );
      setProjectID(project.data.id);
      setRevisionID(revision.data.id);
      const queued = await queueScriptAnalysis({ revision_id: revision.data.id }) as API.OperationResponse;
      setOperation(queued.data);
      setPhase("queued");

      let latest = queued.data;
      for (let attempt = 0; attempt < 80; attempt += 1) {
        await sleep(250);
        const response = await getOperation({ operation_id: latest.id });
        latest = response.data;
        setOperation(latest);
        if (latest.status === "succeeded" || latest.status === "failed") break;
      }
      if (latest.status !== "succeeded") {
        throw new Error(latest.error ?? "剧本解析任务未成功完成");
      }
      const draft = await getAnalysisDraft({ revision_id: revision.data.id });
      setAnalysis(draft.data);
      setPhase("draft");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "剧本解析失败，请查看任务状态。");
    } finally {
      setBusy(false);
    }
  }

  async function approve() {
    if (!revisionID) return;
    setBusy(true);
    setError(null);
    try {
      const response = await approveScriptAnalysis({ revision_id: revisionID });
      setAnalysis(response.data);
      setPhase("approved");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "批准失败，请修正当前事实后重试。");
    } finally {
      setBusy(false);
    }
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
          <div className="status" data-testid="phase-status">
            {phase === "idle" && "等待导入"}
            {phase === "queued" && `解析中 ${operation?.progress ?? 0}%`}
            {phase === "draft" && "待人工批准"}
            {phase === "approved" && "事实已批准"}
          </div>
        </header>

        <div className="grid">
          <section className="card" aria-labelledby="source-title">
            <h2 id="source-title">1. 导入整本剧本</h2>
            <div className="field">
              <label htmlFor="script-name">剧本名称</label>
              <input id="script-name" value={scriptName} onChange={(event) => setScriptName(event.target.value)} />
            </div>
            <div className="field">
              <label htmlFor="script-content">剧本文本</label>
              <textarea id="script-content" value={content} onChange={(event) => setContent(event.target.value)} />
              <span className="hint">支持当前首轮的纯文本输入；原文会计算 hash 并保全到 MinIO。</span>
            </div>
            <div className="actions">
              <button className="primary" type="button" onClick={submitForAnalysis} disabled={busy || !content.trim()} data-testid="analyze-button">
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
            {operation && <div className="bar" role="progressbar" aria-label="解析进度" aria-valuemin={0} aria-valuemax={100} aria-valuenow={operation.progress}><span style={{ width: `${operation.progress}%` }} /></div>}
            {projectID && <div className="hint" style={{ marginTop: 12 }}>Project：{projectID}</div>}
            {analysis && <div className="summary">
              <div className="metric"><strong>{analysis.episodes.length}</strong><span>剧集</span></div>
              <div className="metric"><strong>{analysis.episodes.reduce((total, episode) => total + episode.scenes.length, 0)}</strong><span>场景</span></div>
              <div className="metric"><strong>{analysis.characters.length}</strong><span>人物</span></div>
              <div className="metric"><strong>{assets.length}</strong><span>生产资产</span></div>
            </div>}
          </section>
        </div>

        {analysis && <section className="card" style={{ marginTop: 18 }} aria-labelledby="result-title">
          <h2 id="result-title">3. 剧集、场景与资产</h2>
          <div className="asset-list" role="list" aria-label="资产清单">
            {assets.map((asset) => <span className="asset" role="listitem" key={`${asset.kind}-${asset.name}`}>{asset.name}<em>{asset.kind} · {asset.episode_numbers.join(", ")}</em></span>)}
          </div>
          {analysis.episodes.map((episode) => <article className="episode" key={episode.number}>
            <div className="episode-head"><span className="episode-title">第 {episode.number} 集 · {episode.title}</span><span className="hint">{episode.scenes.length} 个场景</span></div>
            {episode.scenes.map((scene) => <div className="scene" key={scene.id}><strong>{scene.heading}</strong><p>{scene.narratives.slice(0, 4).map((narrative) => narrative.text).join(" · ")}</p></div>)}
          </article>)}
        </section>}
      </div>
    </main>
  );
}
