'use client';
import { useState } from 'react';
import { FileDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
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
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Spinner } from '@/components/ui/spinner';
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty';
import { Input } from '@/components/ui/input';
import {
  Field,
  FieldLabel,
  FieldDescription,
  FieldGroup,
} from '@/components/ui/field';
interface Source {
  id: string;
  title: string;
  revision: string;
  sourcePath: string;
  path: string;
}
export function Handoff({
  documents,
  repoCommit,
  preview,
}: {
  documents: Source[];
  repoCommit: string;
  preview: boolean;
}) {
  const [goal, setGoal] = useState('');
  const [acceptance, setAcceptance] = useState('');
  const [project, setProject] = useState('workspace');
  const [selected, setSelected] = useState(
    documents.find((d) => d.id === 'requirement-28')?.id ??
      documents[0]?.id ??
      '',
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function download() {
    setBusy(true);
    setError('');
    try {
      const ids = [
        ...new Set([
          'root-agents',
          selected,
          project === 'workspace'
            ? 'workspace-agents'
            : `root-${project}-agents`,
          'root-project',
          'workflow-feature',
        ]),
      ];
      const source = await Promise.all(
        ids
          .flatMap((id) => {
            const doc = documents.find((d) => d.id === id);
            return doc ? [doc] : [];
          })
          .map(async (doc) => {
            const r = await fetch(
              `${process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? ''}/raw/${doc.id}${doc.path.endsWith('.csv') ? '.csv' : '.md'}`,
            );
            if (!r.ok) {
              throw new Error('原文加载失败');
            }
            const bytes = await r.arrayBuffer();
            const sha = Array.from(
              new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)),
              (b) => b.toString(16).padStart(2, '0'),
            ).join('');
            if (sha !== doc.revision) {
              throw new Error('原文版本已变化，请重新加载页面');
            }
            return { ...doc, source: new TextDecoder().decode(bytes) };
          }),
      );
      let remaining = 12000;
      const excerpts = source.map(({ source, ...doc }) => {
        const excerpt = source.slice(0, remaining);
        remaining -= excerpt.length;
        return { ...doc, excerpt };
      });
      const pack = {
        kind: 'workspace-handoff',
        goal,
        acceptance,
        project,
        repoCommit,
        preview,
        issuedAt: new Date().toISOString(),
        executionAuthorization: false,
        requiresRevalidation: [
          '当前工作树与代码摘要',
          '任务权限与有效期',
          '适用Skill、流程和完整原文',
        ],
        documents: excerpts,
      };
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(pack, null, 2)], { type: 'application/json' }),
      );
      const link = document.createElement('a');
      link.href = url;
      link.download = 'workspace-handoff.json';
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      setError(e instanceof Error ? e.message : '交接包生成失败');
    } finally {
      setBusy(false);
    }
  }
  if (!documents.length) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>当前范围没有可交接的知识正文</EmptyTitle>
        </EmptyHeader>
      </Empty>
    );
  }
  return (
    <Card className="mt-6">
      <CardHeader>
        <CardTitle>导出人工交接</CardTitle>
        <CardDescription>以经过核对的原文生成上下文包。</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void download();
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="handoff-goal">交接目标</FieldLabel>
              <Input
                id="handoff-goal"
                required
                name="handoff-goal"
                value={goal}
                onChange={(e) => setGoal(e.target.value)}
                autoComplete="off"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="handoff-acceptance">交接接受标准</FieldLabel>
              <Input
                id="handoff-acceptance"
                required
                name="handoff-acceptance"
                value={acceptance}
                onChange={(e) => setAcceptance(e.target.value)}
                autoComplete="off"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="handoff-project">目标工程</FieldLabel>
              <NativeSelect
                className="w-full"
                id="handoff-project"
                value={project}
                onChange={(e) => setProject(e.target.value)}
              >
                {['workspace', 'frontend', 'backend', 'agent', 'canvas'].map(
                  (p) => (
                    <NativeSelectOption key={p}>{p}</NativeSelectOption>
                  ),
                )}
              </NativeSelect>
            </Field>
            <Field>
              <FieldLabel htmlFor="handoff-source">关联文档</FieldLabel>
              <NativeSelect
                className="w-full"
                id="handoff-source"
                value={selected}
                onChange={(e) => setSelected(e.target.value)}
              >
                {documents.map((d) => (
                  <NativeSelectOption key={d.id} value={d.id}>
                    {d.title}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </Field>
            <FieldDescription>
              包含同版原文、修订和最多12000字符摘录。交接包不授予执行权限；接手需核对当前工作树、完整规则和授权。
            </FieldDescription>
            <Button type="submit" disabled={busy}>
              {busy ? (
                <Spinner data-icon="inline-start" aria-label="正在核对原文" />
              ) : (
                <FileDown data-icon="inline-start" aria-hidden="true" />
              )}
              导出交接包
            </Button>
            {error && (
              <Alert>
                <AlertDescription role="alert">{error}</AlertDescription>
              </Alert>
            )}
          </FieldGroup>
        </form>
      </CardContent>
    </Card>
  );
}
