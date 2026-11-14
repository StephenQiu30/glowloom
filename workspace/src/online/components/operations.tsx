"use client";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  Field,
  FieldLabel,
  FieldDescription,
  FieldGroup,
} from "@/components/ui/field";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import type { Run } from "@/workflows/run";
import type { Proposal } from "@/server/store";
import { api } from "./api";
type Doc = {
  id: string;
  title: string;
  revision: string;
  source: string;
  editable: boolean;
};
type Status = {
  database: boolean;
  bridge: boolean;
  tts: boolean;
  actor: string;
  message: string | null;
};
const labels: Record<string, string> = {
  created: "待开始",
  ready: "就绪",
  running: "执行中",
  waiting_input: "等待审阅",
  waiting_external: "等待外部结果",
  paused: "已暂停",
  succeeded: "范围内完成",
  failed: "失败",
  canceled: "已取消",
  unknown: "回执未知",
};
export function Operations() {
  const [status, setStatus] = useState<Status | null>(null),
    [docs, setDocs] = useState<Doc[]>([]),
    [proposals, setProposals] = useState<Proposal[]>([]),
    [runs, setRuns] = useState<Run[]>([]),
    [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [documentId, setDocumentId] = useState("");
  const [original, setOriginal] = useState("");
  const [revision, setRevision] = useState("");
  const [text, setText] = useState("");
  const [notice, setNotice] = useState("");
  const [goal, setGoal] = useState(""),
    [acceptance, setAcceptance] = useState(""),
    [workflowId, setWorkflowId] = useState("feature"),
    [project, setProject] = useState("workspace"),
    [allowWrite, setAllowWrite] = useState(false),
    [skills, setSkills] = useState<
      { name: string; enabled: boolean; sourcePath: string }[]
    >([]);
  const requestKeys = useRef(new Map<string, string>());
  const key = (name: string) => {
    let value = requestKeys.current.get(name);
    if (!value) {
      value = crypto.randomUUID();
      requestKeys.current.set(name, value);
    }
    return value;
  };
  const reload = async () => {
    const [s, d] = await Promise.all([
      api<Status>("/status"),
      api<Doc[]>("/documents"),
    ]);
    setStatus(s);
    setDocs(d);
    if (s.database) {
      const [p, r] = await Promise.all([
        api<Proposal[]>("/proposals"),
        api<Run[]>("/runs"),
      ]);
      setProposals(p);
      setRuns(r);
    }
  };
  useEffect(() => {
    let canceled = false;
    const init = async () => {
      try {
        const [s, d] = await Promise.all([
          api<Status>("/status"),
          api<Doc[]>("/documents"),
        ]);
        if (canceled) return;
        setStatus(s);
        setDocs(d);
        const selected = new URL(location.href).searchParams.get("document");
        if (selected) setDocumentId(selected);
        if (s.database) {
          const [p, r] = await Promise.all([
            api<Proposal[]>("/proposals"),
            api<Run[]>("/runs"),
          ]);
          if (!canceled) {
            setProposals(p);
            setRuns(r);
          }
        }
      } catch (error) {
        if (!canceled)
          setError(error instanceof Error ? error.message : "加载失败");
      }
    };
    void init();
    return () => {
      canceled = true;
    };
  }, []);
  const perform = async (name: string, action: () => Promise<void>) => {
    setBusy(name);
    setError("");
    setNotice("");
    try {
      await action();
    } catch (error) {
      setError(error instanceof Error ? error.message : "操作失败");
    } finally {
      setBusy("");
    }
  };
  const runAction = async (
    run: Run,
    action: string,
    extra: Record<string, unknown> = {},
  ) =>
    perform(run.id + action, async () => {
      const updated = await api<Run>(`/runs/${run.id}/action`, {
        action,
        version: run.version,
        idempotencyKey: key(`${run.id}:${run.version}:${action}`),
        ...extra,
      });
      setRuns((rows) => rows.map((r) => (r.id === run.id ? updated : r)));
    });
  const selected = docs.find((d) => d.id === documentId);
  return (
    <>
      <div className="actions" style={{ marginBottom: 24 }}>
        <Badge variant="outline">本机协作</Badge>
        <span className="subtle-note">
          存储：{status?.database ? "已连接" : "未连接"}　执行桥：
          {status?.bridge ? "已连接" : "未连接"}　真实 TTS：未启用
        </span>
        <Button
          variant="ghost"
          size="sm"
          disabled={!!busy}
          onClick={() => void perform("reload", reload)}
        >
          刷新
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() =>
            void perform("logout", async () => {
              await api("/logout", {});
              location.assign(
                (process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? "") + "/login/",
              );
            })
          }
        >
          退出
        </Button>
      </div>
      {status?.message && (
        <Alert className="callout">
          <AlertTitle>部分在线能力暂不可用</AlertTitle>
          <AlertDescription>
            {status.message}；知识阅读可继续使用。
          </AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert className="callout">
          <AlertTitle>操作未完成</AlertTitle>
          <AlertDescription role="alert">{error}</AlertDescription>
        </Alert>
      )}
      {notice && (
        <p role="status" className="status-message">
          {notice}
        </p>
      )}
      <section className="operations-section">
        <h2>文档提案</h2>
        <p className="subtle-note">
          先读取当前原文，审阅前后差异，再单独应用。应用不会提交 Git
          或发布站点。
        </p>
        <FieldGroup className="form-stack">
          <Field>
            <FieldLabel htmlFor="proposal-document">选择文档</FieldLabel>
            <select
              id="proposal-document"
              className="ws-select"
              value={documentId}
              onChange={(e) => {
                setDocumentId(e.target.value);
                setOriginal("");
                setText("");
                setRevision("");
              }}
            >
              <option value="">请选择</option>
              {docs
                .filter((d) => d.editable)
                .map((d) => (
                  <option value={d.id} key={d.id}>
                    {d.title}
                  </option>
                ))}
            </select>
          </Field>
          <div className="actions">
            <Button
              variant="outline"
              disabled={!selected || !!busy || !status?.bridge}
              onClick={() =>
                void perform("read-document", async () => {
                  const live = await api<{
                    id: string;
                    source: string;
                    revision: string;
                  }>(`/documents?live=${encodeURIComponent(documentId)}`);
                  setOriginal(live.source);
                  setText(live.source);
                  setRevision(live.revision);
                  setNotice("已读取当前原文。阅读页仍显示构建快照。");
                })
              }
            >
              读取当前原文
            </Button>
            {revision && (
              <span className="subtle-note">基线 {revision.slice(0, 12)}</span>
            )}
          </div>
          {revision && (
            <>
              <Field>
                <FieldLabel htmlFor="proposal-text">
                  修改后的 Markdown
                </FieldLabel>
                <Textarea
                  id="proposal-text"
                  value={text}
                  onChange={(e) => setText(e.target.value)}
                />
              </Field>
              <details>
                <summary>审阅修改前后</summary>
                <div className="diff-columns">
                  <section>
                    <h3>修改前</h3>
                    <pre>{original}</pre>
                  </section>
                  <section>
                    <h3>修改后</h3>
                    <pre>{text}</pre>
                  </section>
                </div>
              </details>
              <Button
                disabled={!!busy || text === original || !status?.database}
                onClick={() =>
                  void perform("proposal", async () => {
                    const p = await api<Proposal>("/proposals", {
                      documentId,
                      baseRevision: revision,
                      baseText: original,
                      text,
                      idempotencyKey: key(
                        `proposal:${documentId}:${revision}:${text}`,
                      ),
                    });
                    setProposals((rows) => [
                      p,
                      ...rows.filter((r) => r.id !== p.id),
                    ]);
                    setNotice("提案已保存，尚未修改正文。请审阅后单独应用。");
                  })
                }
              >
                保存修改提案
              </Button>
            </>
          )}
        </FieldGroup>
        <div className="document-list" style={{ marginTop: 24 }}>
          {proposals.map((p) => (
            <section key={p.id} className="proposal-row">
              <div className="actions">
                <strong>
                  {docs.find((d) => d.id === p.documentId)?.title ??
                    p.documentId}
                </strong>
                <Badge variant="outline">
                  {p.receipt ? "已落地" : "待审阅"}
                </Badge>
                <span className="subtle-note">
                  基线 {p.baseRevision.slice(0, 10)}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!!busy || !!p.receipt || !status?.bridge}
                  onClick={() =>
                    void perform(p.id, async () => {
                      const updated = await api<Proposal>(
                        `/proposals/${p.id}/apply`,
                        {},
                      );
                      setProposals((rows) =>
                        rows.map((x) => (x.id === p.id ? updated : x)),
                      );
                      setNotice("正文已落地；Git 提交和站点发布尚未执行。");
                    })
                  }
                >
                  应用已审阅提案
                </Button>
              </div>
              <details>
                <summary>查看提案内容</summary>
                <div className="diff-columns">
                  <section>
                    <h3>修改前</h3>
                    <pre>{p.baseText ?? "未保存修改前正文"}</pre>
                  </section>
                  <section>
                    <h3>修改后</h3>
                    <pre>{p.text}</pre>
                  </section>
                </div>
              </details>
            </section>
          ))}
        </div>
      </section>
      <section className="operations-section">
        <h2>任务与工作流</h2>
        <FieldGroup className="form-stack">
          <div className="actions">
            <Field>
              <FieldLabel htmlFor="run-workflow">工作流</FieldLabel>
              <select
                id="run-workflow"
                className="ws-select"
                value={workflowId}
                onChange={(e) => setWorkflowId(e.target.value)}
              >
                <option value="feature">功能开发</option>
                <option value="bugfix">缺陷修复</option>
                <option value="knowledge">知识更新</option>
              </select>
            </Field>
            <Field>
              <FieldLabel htmlFor="run-project">工程</FieldLabel>
              <select
                id="run-project"
                className="ws-select"
                value={project}
                onChange={(e) => setProject(e.target.value)}
              >
                {["workspace", "frontend", "backend", "agent", "canvas"].map(
                  (p) => (
                    <option key={p}>{p}</option>
                  ),
                )}
              </select>
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="run-goal">任务目标</FieldLabel>
            <Textarea
              id="run-goal"
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="说明要解决的问题与允许范围"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="run-acceptance">接受标准</FieldLabel>
            <Input
              id="run-acceptance"
              value={acceptance}
              onChange={(e) => setAcceptance(e.target.value)}
            />
          </Field>
          <label className="subtle-note">
            <input
              type="checkbox"
              checked={allowWrite}
              onChange={(e) => setAllowWrite(e.target.checked)}
            />{" "}
            明确允许 Codex 修改所选工程内文件；仍不允许提交、推送、部署或收费。
          </label>
          <FieldDescription>
            默认只读。上下文包含当前选择文档、项目规则、Skill
            和流程修订；输入变化后须重新复核。
          </FieldDescription>
          <div className="actions">
            <Button
              disabled={
                !!busy ||
                !goal.trim() ||
                !acceptance.trim() ||
                !status?.database ||
                !status.bridge
              }
              onClick={() =>
                void perform("create-run", async () => {
                  const run = await api<Run>("/runs", {
                    goal,
                    acceptance,
                    workflowId,
                    project,
                    allowWrite,
                    documents: documentId ? [documentId] : [],
                    idempotencyKey: key(
                      `run:${goal}:${acceptance}:${project}:${workflowId}:${allowWrite}`,
                    ),
                  });
                  setRuns((rows) => [
                    run,
                    ...rows.filter((r) => r.id !== run.id),
                  ]);
                  setNotice("任务已创建，尚未派发。请核对上下文后开始。");
                })
              }
            >
              创建任务
            </Button>
            <Button
              variant="outline"
              disabled={!!busy || !status?.bridge}
              onClick={() =>
                void perform("skills", async () =>
                  setSkills(await api("/host/skills", { project })),
                )
              }
            >
              验证主机 Skill 发现
            </Button>
          </div>
          {skills.length > 0 && (
            <ul className="subtle-note">
              {skills.map((s) => (
                <li key={s.name}>
                  {s.name}：{s.enabled ? "已发现" : "未启用"}　{s.sourcePath}
                </li>
              ))}
            </ul>
          )}
        </FieldGroup>
        <div className="run-list">
          {runs.map((run) => (
            <section className="run-card" key={run.id}>
              <div className="actions">
                <h3>{run.context.goal}</h3>
                <Badge variant="outline">{labels[run.status]}</Badge>
              </div>
              <p className="subtle-note">
                {run.context.project}　阶段 {run.stage + 1}：
                {run.context.stageIds[run.stage]}　尝试 {run.attempt}　版本{" "}
                {run.version}
              </p>
              {run.externalHandle && (
                <p className="subtle-note">
                  原句柄 <code>{run.externalHandle}</code>
                </p>
              )}
              <details>
                <summary>上下文与接受标准</summary>
                <pre className="proposal-preview">
                  {JSON.stringify(run.context, null, 2)}
                </pre>
              </details>
              <div className="actions" style={{ marginTop: 16 }}>
                <Button
                  size="sm"
                  disabled={
                    !!busy || !["created", "ready"].includes(run.status)
                  }
                  onClick={() => void runAction(run, "start")}
                >
                  开始当前阶段
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={
                    !!busy ||
                    ![
                      "running",
                      "waiting_external",
                      "unknown",
                      "paused",
                      "canceled",
                    ].includes(run.status)
                  }
                  onClick={() => void runAction(run, "reconcile")}
                >
                  查询原运行
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={
                    !!busy ||
                    ![
                      "running",
                      "waiting_external",
                      "waiting_input",
                      "ready",
                    ].includes(run.status)
                  }
                  onClick={() => void runAction(run, "pause")}
                >
                  暂停后续动作
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={!!busy || run.status !== "paused"}
                  onClick={() => void runAction(run, "resume")}
                >
                  恢复
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={
                    !!busy || ["succeeded", "canceled"].includes(run.status)
                  }
                  onClick={() => void runAction(run, "cancel")}
                >
                  取消
                </Button>
              </div>
              <div className="actions" style={{ marginTop: 10 }}>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={
                    !!busy ||
                    run.context.project !== "workspace" ||
                    !["waiting_input", "ready", "created"].includes(run.status)
                  }
                  onClick={() =>
                    void runAction(run, "verify", { commandId: "unit" })
                  }
                >
                  运行单元检查
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={
                    !!busy ||
                    !["waiting_input", "ready", "created"].includes(run.status)
                  }
                  onClick={() =>
                    void runAction(run, "verify", { commandId: "typecheck" })
                  }
                >
                  {run.context.project === "backend"
                    ? "运行 Go vet"
                    : "运行类型检查"}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!!busy || run.status !== "waiting_input"}
                  onClick={() => void runAction(run, "refresh")}
                >
                  复核并更新上下文
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={
                    !!busy ||
                    run.status !== "waiting_input" ||
                    (run.context.stageIds[run.stage] !== "accept" &&
                      !run.reacceptRequired)
                  }
                  onClick={() => void runAction(run, "accept")}
                >
                  人工接受
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!!busy || run.status !== "waiting_input"}
                  onClick={() => void runAction(run, "advance")}
                >
                  推进阶段
                </Button>
              </div>
              {run.evidence.length > 0 && (
                <details style={{ marginTop: 16 }}>
                  <summary>实际结果与证据（{run.evidence.length}）</summary>
                  {run.evidence.map((e, i) => (
                    <section key={i} className="evidence-item">
                      <Badge variant="outline">
                        {e.kind} / {e.status}
                      </Badge>
                      <pre>{e.summary}</pre>
                      <code>{e.artifactRef}</code>
                      {e.inputDigest !== run.context.digest && (
                        <p className="subtle-note">对应旧输入，需重新验证。</p>
                      )}
                    </section>
                  ))}
                </details>
              )}
            </section>
          ))}
        </div>
      </section>
    </>
  );
}
