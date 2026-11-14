'use client';
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
import type { Run } from '@/workflows/run';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Spinner } from '@/components/ui/spinner';
import { Disclosure } from '@/components/disclosure';
const labels: Record<string, string> = {
  created: '待开始',
  ready: '就绪',
  running: '执行中',
  waiting_input: '等待审阅',
  waiting_external: '等待外部结果',
  paused: '已暂停',
  succeeded: '范围内完成',
  failed: '失败',
  canceled: '已取消',
  unknown: '回执未知',
};
interface RunCardProps {
  readonly run: Run;
  readonly busy: string;
  readonly onAction: (
    run: Run,
    action: string,
    extra?: Record<string, unknown>,
  ) => Promise<void>;
}
/** Keeps execution evidence and state-specific controls separate from proposal editing. */
export function RunCard({ run, busy, onAction }: RunCardProps) {
  return (
    <Card>
      <CardHeader>
        <div className="actions">
          <CardTitle>{run.context.goal}</CardTitle>
          <Badge variant="outline">{labels[run.status]}</Badge>
        </div>
        <p className="subtle-note">
          {run.context.project}　阶段 {run.stage + 1}：
          {run.context.stageIds[run.stage]}　尝试 {run.attempt}
          　版本 {run.version}
        </p>
        {run.externalHandle && (
          <p className="subtle-note">
            原句柄 <code>{run.externalHandle}</code>
          </p>
        )}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <Disclosure label="上下文与接受标准">
          <pre className="proposal-preview">
            {JSON.stringify(run.context, null, 2)}
          </pre>
        </Disclosure>
        <div className="actions" style={{ marginTop: 16 }}>
          <Button
            size="sm"
            disabled={!!busy || !['created', 'ready'].includes(run.status)}
            onClick={() => void onAction(run, 'start')}
          >
            {busy === run.id + 'start' && (
              <Spinner data-icon="inline-start" aria-label="处理中" />
            )}
            开始当前阶段
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={
              !!busy ||
              ![
                'running',
                'waiting_external',
                'unknown',
                'paused',
                'canceled',
              ].includes(run.status)
            }
            onClick={() => void onAction(run, 'reconcile')}
          >
            {busy === run.id + 'reconcile' && (
              <Spinner data-icon="inline-start" aria-label="处理中" />
            )}
            查询原运行
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={
              !!busy ||
              ![
                'running',
                'waiting_external',
                'waiting_input',
                'ready',
              ].includes(run.status)
            }
            onClick={() => void onAction(run, 'pause')}
          >
            {busy === run.id + 'pause' && (
              <Spinner data-icon="inline-start" aria-label="处理中" />
            )}
            暂停后续动作
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={!!busy || run.status !== 'paused'}
            onClick={() => void onAction(run, 'resume')}
          >
            {busy === run.id + 'resume' && (
              <Spinner data-icon="inline-start" aria-label="处理中" />
            )}
            恢复
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                disabled={
                  !!busy || ['succeeded', 'canceled'].includes(run.status)
                }
              >
                {busy === run.id + 'cancel' && (
                  <Spinner data-icon="inline-start" aria-label="处理中" />
                )}
                取消
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>取消这个任务？</AlertDialogTitle>
                <AlertDialogDescription>
                  停止后续动作并保留历史证据；已有副作用不会回滚，未知回执仍需查询原运行。
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>保留任务</AlertDialogCancel>
                <AlertDialogAction onClick={() => void onAction(run, 'cancel')}>
                  确认取消任务
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
        <div className="actions" style={{ marginTop: 10 }}>
          <Button
            size="sm"
            variant="outline"
            disabled={
              !!busy ||
              run.context.project !== 'workspace' ||
              !['waiting_input', 'ready', 'created'].includes(run.status)
            }
            onClick={() => void onAction(run, 'verify', { commandId: 'unit' })}
          >
            运行单元检查
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={
              !!busy ||
              !['waiting_input', 'ready', 'created'].includes(run.status)
            }
            onClick={() =>
              void onAction(run, 'verify', {
                commandId: 'typecheck',
              })
            }
          >
            {run.context.project === 'backend' ? '运行 Go vet' : '运行类型检查'}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={!!busy || run.status !== 'waiting_input'}
            onClick={() => void onAction(run, 'refresh')}
          >
            {busy === run.id + 'refresh' && (
              <Spinner data-icon="inline-start" aria-label="处理中" />
            )}
            复核并更新上下文
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={
              !!busy ||
              run.status !== 'waiting_input' ||
              (run.context.stageIds[run.stage] !== 'accept' &&
                !run.reacceptRequired)
            }
            onClick={() => void onAction(run, 'accept')}
          >
            {busy === run.id + 'accept' && (
              <Spinner data-icon="inline-start" aria-label="处理中" />
            )}
            人工接受
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={!!busy || run.status !== 'waiting_input'}
            onClick={() => void onAction(run, 'advance')}
          >
            {busy === run.id + 'advance' && (
              <Spinner data-icon="inline-start" aria-label="处理中" />
            )}
            推进阶段
          </Button>
        </div>
        {run.evidence.length > 0 && (
          <Disclosure label={`实际结果与证据（${run.evidence.length}）`}>
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
          </Disclosure>
        )}
      </CardContent>
    </Card>
  );
}
