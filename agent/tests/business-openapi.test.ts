import { it } from 'vitest';
import { execFileSync } from 'node:child_process';
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

const agent = process.cwd();
const backend = path.resolve(agent, '../backend');
const cache = path.resolve(agent, 'node_modules/.cache/openapi');
const expected = path.resolve(agent, 'src/business/generated');
const check =
  process.env.BUSINESS_API_MODE !== 'generate' &&
  process.env.BUSINESS_API_MODE !== 'schema';
const schemaOnly = process.env.BUSINESS_API_MODE === 'schema';
it('generates and verifies the internal Go business client', () => {
  const workspace = mkdtempSync(path.join(tmpdir(), 'business-api-'));
  const schemaFile = path.join(workspace, 'swagger.json');
  const output = check ? path.join(workspace, 'client') : expected;
  try {
    execFileSync(
      'go',
      [
        'run',
        'github.com/swaggo/swag/cmd/swag@v1.16.6',
        'init',
        '-g',
        'business_swagger.go',
        '-d',
        'internal/agent/adapter/http,internal/agent/domain,internal/agent/application',
        '--parseInternal',
        '--tags',
        'business',
        '--outputTypes',
        'json',
        '-o',
        workspace,
      ],
      { cwd: backend, stdio: 'inherit' },
    );
    const source = readFileSync(schemaFile, 'utf8');
    const schema = JSON.parse(source) as {
      paths: Record<string, Record<string, { operationId: string }>>;
    };
    const actualRoutes = new Set<string>();
    for (const name of [
      'business.go',
      'conversation_sources.go',
      'skills.go',
    ]) {
      const source = readFileSync(
        path.join(backend, 'internal/agent/adapter/http', name),
        'utf8',
      );
      const relevant =
        name === 'skills.go'
          ? source.slice(0, source.indexOf('// SkillManagementHandler'))
          : source;
      for (const match of relevant.matchAll(
        /g\.(GET|POST|PUT|PATCH|DELETE)\("([^\"]+)"/g,
      )) {
        const route = match[2]?.replace(/:([a-z_]+)/g, '{$1}');
        actualRoutes.add(
          `${match[1]?.toLowerCase()} /internal/agent-business${route}`,
        );
      }
    }
    const operationIds = new Set<string>();
    const documented = new Set<string>();
    for (const [route, methods] of Object.entries(schema.paths))
      for (const [method, operation] of Object.entries(methods)) {
        if (!operation.operationId || operationIds.has(operation.operationId))
          throw new Error('Missing or duplicate operationId');
        operationIds.add(operation.operationId);
        documented.add(`${method} ${route}`);
      }
    if (
      [...actualRoutes].some((route) => !documented.has(route)) ||
      [...documented].some((route) => !actualRoutes.has(route))
    )
      throw new Error(
        `Internal routes differ from schema: ${JSON.stringify({ actualRoutes: [...actualRoutes], documented: [...documented] })}`,
      );
    if (!check) {
      mkdirSync(cache, { recursive: true });
      writeFileSync(path.join(cache, 'go-business.swagger.json'), source);
    }
    if (!schemaOnly) {
      execFileSync('pnpm', ['exec', 'openapi2ts'], {
        cwd: agent,
        stdio: 'inherit',
        env: {
          ...process.env,
          BUSINESS_API_OUTPUT: output,
          BUSINESS_API_SCHEMA: schemaFile,
        },
      });
      const index = path.join(output, 'index.ts');
      writeFileSync(
        index,
        readFileSync(index, 'utf8').replace(
          /from '\.\/business'/g,
          "from './business.js'",
        ),
      );
      execFileSync(
        'pnpm',
        [
          'exec',
          'prettier',
          '--config',
          path.join(agent, '.prettierrc.json'),
          '--write',
          output,
        ],
        {
          cwd: agent,
          stdio: 'inherit',
        },
      );
      if (check) {
        const generated = readdirSync(output).sort();
        const existing = readdirSync(expected).sort();
        if (
          JSON.stringify(generated) !== JSON.stringify(existing) ||
          generated.some(
            (file) =>
              readFileSync(path.join(output, file), 'utf8') !==
              readFileSync(path.join(expected, file), 'utf8'),
          )
        )
          throw new Error(
            'Generated business API differs; run pnpm api:generate',
          );
      }
    }
  } finally {
    rmSync(workspace, { recursive: true, force: true });
  }
}, 120000);
