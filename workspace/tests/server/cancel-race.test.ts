import { execFileSync } from 'node:child_process';
import { expect, it } from 'vitest';
import {
  mkdtemp,
  mkdir,
  writeFile,
  readFile,
  rm,
  chmod,
} from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { request } from 'node:http';
import type { liveContext } from '../../src/server/bridge/context';
import { startBridge } from '../../src/server/bridge/service';
import { buildContext } from '../../src/workflows/definition';
it('remembers cancellation before a native turn handle is available', async () => {
  const root = await mkdtemp(join(tmpdir(), 'ws-cancel-'));
  const socket = '/tmp/ws-' + randomUUID() + '.sock';
  const token = 'fixture-token-with-at-least-thirty-two-chars';
  await mkdir(join(root, 'workspace/workflows'), { recursive: true });
  await writeFile(
    join(root, 'workspace/projects.json'),
    JSON.stringify([{ id: 'workspace', path: 'workspace' }]),
  );
  await writeFile(
    join(root, 'workspace/catalog.json'),
    JSON.stringify([
      {
        id: 'workflow-feature',
        path: 'workflows/feature.md',
        visibility: 'internal',
      },
    ]),
  );
  await writeFile(
    join(root, 'workspace/workflows/feature.md'),
    '# Feature\n```workspace-workflow\n' +
      JSON.stringify({
        id: 'feature',
        stages: [
          {
            id: 'accept',
            label: 'Accept',
            needs: [],
            outputs: ['result'],
            optional: false,
          },
        ],
      }) +
      '\n```',
  );
  const events = join(root, 'events');
  const fake = join(root, 'codex');
  const release = join(root, 'release-thread');
  await writeFile(
    fake,
    `#!/usr/bin/env node\nconst{createInterface}=require('node:readline');const{appendFileSync,existsSync}=require('node:fs');createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);if(m.id===undefined)return;appendFileSync(${JSON.stringify(events)},m.method+'\\n');let result={};if(m.method==='thread/start')result={thread:{id:'thread'}};if(m.method==='turn/start')result={turn:{id:'turn'}};if(m.method==='thread/read')result={thread:{turns:[{id:'turn',status:'inProgress',items:[]}]}};const respond=()=>process.stdout.write(JSON.stringify({id:m.id,result})+'\\n');if(m.method==='thread/start'){const wait=setInterval(()=>{if(existsSync(${JSON.stringify(release)})){clearInterval(wait);respond();}},5);}else respond();});`,
  );
  await chmod(fake, 0o700);
  execFileSync('git', ['init', '-q'], { cwd: root });
  const bridge = await startBridge({
    repo: root,
    storage: join(root, 'runtime'),
    socket,
    token,
    codexBinary: fake,
  });
  function call<T = unknown>(path: string, body: unknown) {
    return new Promise<T>((yes, no) => {
      const req = request(
        {
          socketPath: socket,
          path,
          method: 'POST',
          headers: { authorization: 'Bearer ' + token },
        },
        (res) => {
          let text = '';
          res.on('data', (c) => (text += c));
          res.on('end', () => yes(JSON.parse(text)));
        },
      );
      req.on('error', no);
      req.end(JSON.stringify(body));
    });
  }
  try {
    const live = await call<Awaited<ReturnType<typeof liveContext>>>(
      '/context',
      {
        project: 'workspace',
        workflowId: 'feature',
      },
    );
    const context = buildContext({
      goal: 'fixture',
      project: 'workspace',
      repoCommit: live.repoCommit,
      codeDigest: live.codeDigest,
      documents: [],
      skillRevisions: live.skillRevisions,
      workflowRevision: live.workflowRevision,
      allowedActions: ['read'],
      acceptance: ['fixture'],
      expiresAt: Date.now() + 60000,
    });
    const runId = randomUUID();
    const started = call('/run/start', {
      runId,
      attempt: 1,
      context,
      workflowId: 'feature',
      stage: 0,
    });
    await expect
      .poll(
        async () => {
          try {
            return (await readFile(events, 'utf8')).includes('thread/start');
          } catch (error) {
            if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
              return false;
            }
            throw error;
          }
        },
        { timeout: 5000 },
      )
      .toBe(true);
    await call('/run/interrupt', { runId, attempt: 1 });
    await writeFile(release, 'continue');
    await started;
    const log = await readFile(events, 'utf8');
    expect(log).toContain('thread/start');
    expect(log).not.toContain('turn/start');
  } finally {
    await bridge.close();
    await rm(root, { recursive: true, force: true });
  }
});
