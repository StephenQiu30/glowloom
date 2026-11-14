'use client';
import { useSearchParams } from 'next/navigation';
import { DraftGuard } from './draft-guard';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import {
  Field,
  FieldLabel,
  FieldDescription,
  FieldGroup,
} from '@/components/ui/field';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import type { Run } from '@/workflows/run';
import type { Proposal } from '@/server/store';
import { api } from './api';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card';
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select';
import { Checkbox } from '@/components/ui/checkbox';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Spinner } from '@/components/ui/spinner';
import { Skeleton } from '@/components/ui/skeleton';
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty';
import { Disclosure } from '@/components/disclosure';
import { request } from './api';
import { RunCard } from './run-card';
import {
  documentsContract,
  statusContract,
  proposalsContract,
  runsContract,
  runContract,
  proposalContract,
  liveDocumentContract,
  skillsContract,
  type Document as Doc,
  type Status,
  type HostSkill,
} from './contracts';
export function Operations() {
  const [status, setStatus] = useState<Status | null>(null);
  const [docs, setDocs] = useState<Doc[]>([]);
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [documentId, setDocumentId] = useState('');
  const [original, setOriginal] = useState('');
  const [revision, setRevision] = useState('');
  const [text, setText] = useState('');
  const [savedText, setSavedText] = useState('');
  const dirty = text !== savedText;
  const params = useSearchParams();
  const tab = params.get('tab') === 'runs' ? 'runs' : 'proposals';
  const [notice, setNotice] = useState('');
  const [goal, setGoal] = useState('');
  const [acceptance, setAcceptance] = useState('');
  const [workflowId, setWorkflowId] = useState('feature');
  const [project, setProject] = useState('workspace');
  const [allowWrite, setAllowWrite] = useState(false);
  const [skills, setSkills] = useState<HostSkill[]>([]);
  const requestKeys = useRef(new Map<string, string>());
  function key(name: string) {
    let value = requestKeys.current.get(name);
    if (!value) {
      value = crypto.randomUUID();
      requestKeys.current.set(name, value);
    }
    return value;
  }
  async function reload() {
    const [s, d] = await Promise.all([
      request('/status', statusContract),
      request('/documents', documentsContract),
    ]);
    setStatus(s);
    setDocs(d);
    if (s.database) {
      const [p, r] = await Promise.all([
        request('/proposals', proposalsContract),
        request('/runs', runsContract),
      ]);
      setProposals(p);
      setRuns(r);
    }
  }
  useEffect(() => {
    let canceled = false;
    async function init() {
      try {
        const [s, d] = await Promise.all([
          request('/status', statusContract),
          request('/documents', documentsContract),
        ]);
        if (canceled) {
          return;
        }
        setStatus(s);
        setDocs(d);
        const selected = new URL(location.href).searchParams.get('document');
        if (selected) {
          setDocumentId(selected);
        }
        if (s.database) {
          const [p, r] = await Promise.all([
            request('/proposals', proposalsContract),
            request('/runs', runsContract),
          ]);
          if (!canceled) {
            setProposals(p);
            setRuns(r);
          }
        }
      } catch (error) {
        if (!canceled) {
          setError(error instanceof Error ? error.message : '加载失败');
        }
      }
    }
    void init();
    return () => {
      canceled = true;
    };
  }, []);
  async function perform(name: string, action: () => Promise<void>) {
    setBusy(name);
    setError('');
    setNotice('');
    try {
      await action();
    } catch (error) {
      setError(error instanceof Error ? error.message : '操作失败');
    } finally {
      setBusy('');
    }
  }
  async function runAction(
    run: Run,
    action: string,
    extra: Record<string, unknown> = {},
  ) {
    return perform(run.id + action, async () => {
      const updated = await request(`/runs/${run.id}/action`, runContract, {
        action,
        version: run.version,
        idempotencyKey: key(`${run.id}:${run.version}:${action}`),
        ...extra,
      });
      setRuns((rows) => rows.map((r) => (r.id === run.id ? updated : r)));
    });
  }
  if (!status && !error) {
    return (
      <div
        className="flex flex-col gap-4"
        role="status"
        aria-label="加载协作台"
      >
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }
  const selected = docs.find((d) => d.id === documentId);
  return (
    <>
      <div className="actions" style={{ marginBottom: 24 }}>
        <Badge variant="outline">本机协作</Badge>
        <span className="subtle-note">
          存储：{status?.database ? '已连接' : '未连接'}　执行桥：
          {status?.bridge ? '已连接' : '未连接'}　真实 TTS：未启用
        </span>
        <Button
          variant="ghost"
          size="sm"
          disabled={!!busy}
          onClick={() => void perform('reload', reload)}
        >
          {busy === 'reload' && (
            <Spinner data-icon="inline-start" aria-label="处理中" />
          )}
          刷新
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() =>
            void perform('logout', async () => {
              await api('/logout', {});
              location.assign(
                (process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? '') + '/login/',
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
        <Alert>
          <AlertDescription role="status">{notice}</AlertDescription>
        </Alert>
      )}
      <DraftGuard dirty={dirty} />
      <Tabs
        value={tab}
        onValueChange={(value) => {
          const url = new URL(location.href);
          url.searchParams.set('tab', value);
          history.pushState(null, '', url);
        }}
        className="mt-6"
      >
        <TabsList>
          <TabsTrigger value="proposals">文档提案</TabsTrigger>
          <TabsTrigger value="runs">任务与工作流</TabsTrigger>
        </TabsList>
        <TabsContent value="proposals">
          <Card>
            <CardHeader>
              <CardTitle>文档提案</CardTitle>
              <CardDescription>
                先读取当前原文，审阅前后差异，再单独应用。应用不会提交 Git
                或发布站点。
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-6">
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="proposal-document">选择文档</FieldLabel>
                  <NativeSelect
                    id="proposal-document"
                    disabled={dirty || Boolean(busy)}
                    className="w-full"
                    value={documentId}
                    onChange={(e) => {
                      setDocumentId(e.target.value);
                      setOriginal('');
                      setText('');
                      setSavedText('');
                      setRevision('');
                    }}
                  >
                    <NativeSelectOption value="">请选择</NativeSelectOption>
                    {docs
                      .filter((d) => d.editable)
                      .map((d) => (
                        <NativeSelectOption value={d.id} key={d.id}>
                          {d.title}
                        </NativeSelectOption>
                      ))}
                  </NativeSelect>
                </Field>
                {dirty && (
                  <FieldDescription>
                    有未保存修改。请保存提案，或{' '}
                    <AlertDialog>
                      <AlertDialogTrigger asChild>
                        <Button variant="link">放弃本次编辑</Button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>放弃未保存修改？</AlertDialogTitle>
                          <AlertDialogDescription>
                            仅恢复本页编辑框，不会修改仓库正文。
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>继续编辑</AlertDialogCancel>
                          <AlertDialogAction onClick={() => setText(savedText)}>
                            放弃本次编辑
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>{' '}
                    后切换文档。
                  </FieldDescription>
                )}
                <div className="actions">
                  <Button
                    variant="outline"
                    disabled={dirty || !selected || !!busy || !status?.bridge}
                    onClick={() =>
                      void perform('read-document', async () => {
                        const live = await request(
                          `/documents?live=${encodeURIComponent(documentId)}`,
                          liveDocumentContract,
                        );
                        if (live.id !== documentId) {
                          throw new Error(
                            '文档响应与当前选择不一致，请重新读取。',
                          );
                        }
                        setOriginal(live.source);
                        setText(live.source);
                        setSavedText(live.source);
                        setRevision(live.revision);
                        setNotice('已读取当前原文。阅读页仍显示构建快照。');
                      })
                    }
                  >
                    {busy === 'read-document' && (
                      <Spinner data-icon="inline-start" aria-label="处理中" />
                    )}
                    读取当前原文
                  </Button>
                  {revision && (
                    <span className="subtle-note">
                      基线 {revision.slice(0, 12)}
                    </span>
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
                        disabled={busy === 'read-document'}
                        name="proposal-text"
                        value={text}
                        onChange={(e) => setText(e.target.value)}
                        autoComplete="off"
                      />
                    </Field>
                    <Disclosure label="审阅修改前后">
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
                    </Disclosure>
                    <Button
                      disabled={
                        !!busy || text === original || !status?.database
                      }
                      onClick={() =>
                        void perform('proposal', async () => {
                          const p = await request(
                            '/proposals',
                            proposalContract,
                            {
                              documentId,
                              baseRevision: revision,
                              baseText: original,
                              text,
                              idempotencyKey: key(
                                `proposal:${documentId}:${revision}:${text}`,
                              ),
                            },
                          );
                          setProposals((rows) => [
                            p,
                            ...rows.filter((r) => r.id !== p.id),
                          ]);
                          setSavedText(text);
                          setNotice(
                            '提案已保存，尚未修改正文。请审阅后单独应用。',
                          );
                        })
                      }
                    >
                      {busy === 'proposal' && (
                        <Spinner data-icon="inline-start" aria-label="处理中" />
                      )}
                      保存修改提案
                    </Button>
                  </>
                )}
              </FieldGroup>
              <div className="flex flex-col gap-4">
                {!proposals.length && (
                  <Empty>
                    <EmptyHeader>
                      <EmptyTitle>暂无文档提案</EmptyTitle>
                    </EmptyHeader>
                  </Empty>
                )}
                {proposals.map((p) => (
                  <Card key={p.id}>
                    <CardHeader>
                      <div className="actions">
                        <strong>
                          {docs.find((d) => d.id === p.documentId)?.title ??
                            p.documentId}
                        </strong>
                        <Badge variant="outline">
                          {p.receipt ? '已落地' : '待审阅'}
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
                              const updated = await request(
                                `/proposals/${p.id}/apply`,
                                proposalContract,
                                {},
                              );
                              setProposals((rows) =>
                                rows.map((x) => (x.id === p.id ? updated : x)),
                              );
                              setNotice(
                                '正文已落地；Git 提交和站点发布尚未执行。',
                              );
                            })
                          }
                        >
                          {busy === p.id && (
                            <Spinner
                              data-icon="inline-start"
                              aria-label="处理中"
                            />
                          )}
                          应用已审阅提案
                        </Button>
                      </div>
                    </CardHeader>
                    <CardContent>
                      <Disclosure label="查看提案内容">
                        <div className="diff-columns">
                          <section>
                            <h3>修改前</h3>
                            <pre>{p.baseText ?? '未保存修改前正文'}</pre>
                          </section>
                          <section>
                            <h3>修改后</h3>
                            <pre>{p.text}</pre>
                          </section>
                        </div>
                      </Disclosure>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="runs">
          <Card>
            <CardHeader>
              <CardTitle>任务与工作流</CardTitle>
              <CardDescription>
                先核对上下文和授权，再派发当前阶段。
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-6">
              <FieldGroup>
                <div className="actions">
                  <Field>
                    <FieldLabel htmlFor="run-workflow">工作流</FieldLabel>
                    <NativeSelect
                      id="run-workflow"
                      className="w-full"
                      value={workflowId}
                      onChange={(e) => setWorkflowId(e.target.value)}
                    >
                      <NativeSelectOption value="feature">
                        功能开发
                      </NativeSelectOption>
                      <NativeSelectOption value="bugfix">
                        缺陷修复
                      </NativeSelectOption>
                      <NativeSelectOption value="knowledge">
                        知识更新
                      </NativeSelectOption>
                    </NativeSelect>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="run-project">工程</FieldLabel>
                    <NativeSelect
                      id="run-project"
                      className="w-full"
                      value={project}
                      onChange={(e) => setProject(e.target.value)}
                    >
                      {[
                        'workspace',
                        'frontend',
                        'backend',
                        'agent',
                        'canvas',
                      ].map((p) => (
                        <NativeSelectOption key={p}>{p}</NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </Field>
                </div>
                <Field>
                  <FieldLabel htmlFor="run-goal">任务目标</FieldLabel>
                  <Textarea
                    id="run-goal"
                    name="run-goal"
                    value={goal}
                    onChange={(e) => setGoal(e.target.value)}
                    placeholder="说明要解决的问题与允许范围"
                    autoComplete="off"
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="run-acceptance">接受标准</FieldLabel>
                  <Input
                    id="run-acceptance"
                    name="run-acceptance"
                    value={acceptance}
                    onChange={(e) => setAcceptance(e.target.value)}
                    autoComplete="off"
                  />
                </Field>
                <Field orientation="horizontal">
                  <Checkbox
                    id="allow-write"
                    checked={allowWrite}
                    onCheckedChange={(checked) =>
                      setAllowWrite(checked === true)
                    }
                  />
                  <FieldLabel htmlFor="allow-write">
                    允许修改所选工程内文件；提交、推送、部署和收费仍需单独授权。
                  </FieldLabel>
                </Field>
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
                      void perform('create-run', async () => {
                        const run = await request('/runs', runContract, {
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
                        setNotice('任务已创建，尚未派发。请核对上下文后开始。');
                      })
                    }
                  >
                    {busy === 'create-run' && (
                      <Spinner data-icon="inline-start" aria-label="处理中" />
                    )}
                    创建任务
                  </Button>
                  <Button
                    variant="outline"
                    disabled={!!busy || !status?.bridge}
                    onClick={() =>
                      void perform('skills', async () =>
                        setSkills(
                          await request('/host/skills', skillsContract, {
                            project,
                          }),
                        ),
                      )
                    }
                  >
                    {busy === 'skills' && (
                      <Spinner data-icon="inline-start" aria-label="处理中" />
                    )}
                    验证主机 Skill 发现
                  </Button>
                </div>
                {skills.length > 0 && (
                  <ul className="subtle-note">
                    {skills.map((s) => (
                      <li key={s.name}>
                        {s.name}：{s.enabled ? '已发现' : '未启用'}　
                        {s.sourcePath}
                      </li>
                    ))}
                  </ul>
                )}
              </FieldGroup>
              <div className="flex flex-col gap-4">
                {!runs.length && (
                  <Empty>
                    <EmptyHeader>
                      <EmptyTitle>暂无任务</EmptyTitle>
                    </EmptyHeader>
                  </Empty>
                )}
                {runs.map((run) => (
                  <RunCard
                    key={run.id}
                    run={run}
                    busy={busy}
                    onAction={runAction}
                  />
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  );
}
