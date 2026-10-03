// @vitest-environment node

import { resolve } from "node:path";
import { ESLint } from "eslint";
import { beforeAll, describe, expect, it } from "vitest";

const frontendRoot = resolve(import.meta.dirname, "../..");
const eslint = new ESLint({
  cwd: frontendRoot,
  overrideConfigFile: resolve(frontendRoot, "eslint.config.mjs"),
});
const boundaryRules = new Set([
  "no-restricted-imports",
  "no-restricted-globals",
  "no-restricted-syntax",
]);

beforeAll(async () => {
  const config = await eslint.calculateConfigForFile(
    resolve(frontendRoot, "src/components/probe.ts"),
  );
  expect(config).toBeDefined();
}, 15_000);

async function boundaryMessages(
  source: string,
  filePath = "src/components/probe.ts",
) {
  const [result] = await eslint.lintText(source, {
    filePath: resolve(frontendRoot, filePath),
  });
  expect(
    result.fatalErrorCount,
    result.messages.map((item) => item.message).join("\n"),
  ).toBe(0);
  return result.messages.filter(
    (message) => message.ruleId && boundaryRules.has(message.ruleId),
  );
}

describe("统一请求边界", () => {
  it.each([
    {
      name: "业务直接导入 Axios",
      source: 'import axios from "axios"; export const client = axios;',
      rule: "no-restricted-imports",
    },
    {
      name: "业务直接导入 request",
      source:
        'import { request } from "@/lib/request"; export const load = request;',
      rule: "no-restricted-imports",
    },
    {
      name: "重命名 request 导入",
      source:
        'import { request as rawRequest } from "@/lib/request"; export const load = rawRequest;',
      rule: "no-restricted-imports",
    },
    {
      name: "相对路径导入 request",
      source:
        'import { request } from "../lib/request"; export const load = request;',
      rule: "no-restricted-imports",
    },
    {
      name: "旧生成目录",
      source:
        'import { listProjects } from "@/gen/api/projects"; export const load = listProjects;',
      rule: "no-restricted-imports",
    },
    {
      name: "直接 fetch",
      source:
        'export const load = () => fetch("https://media.invalid/object");',
      rule: "no-restricted-globals",
    },
    {
      name: "XMLHttpRequest",
      source: "export const create = () => new XMLHttpRequest();",
      rule: "no-restricted-globals",
    },
    {
      name: "EventSource",
      source:
        'export const connect = () => new EventSource("https://media.invalid/events");',
      rule: "no-restricted-globals",
    },
    {
      name: "WebSocket",
      source:
        'export const connect = () => new WebSocket("wss://media.invalid/socket");',
      rule: "no-restricted-globals",
    },
    {
      name: "window.fetch",
      source:
        'export const load = () => window.fetch("https://media.invalid/object");',
      rule: "no-restricted-syntax",
    },
    {
      name: "globalThis.fetch",
      source:
        'export const load = () => globalThis.fetch("https://media.invalid/object");',
      rule: "no-restricted-syntax",
    },
    {
      name: "字符串属性调用 fetch",
      source:
        'export const load = () => globalThis["fetch"]("https://media.invalid/object");',
      rule: "no-restricted-syntax",
    },
    {
      name: "window.XMLHttpRequest",
      source: "export const create = () => new window.XMLHttpRequest();",
      rule: "no-restricted-syntax",
    },
    {
      name: "window.EventSource",
      source:
        'export const connect = () => new window.EventSource("https://media.invalid/events");',
      rule: "no-restricted-syntax",
    },
    {
      name: "globalThis.WebSocket",
      source:
        'export const connect = () => new globalThis.WebSocket("wss://media.invalid/socket");',
      rule: "no-restricted-syntax",
    },
    {
      name: "字面量业务接口路径",
      source: 'export const endpoint = "/api/projects";',
      rule: "no-restricted-syntax",
    },
    {
      name: "接口根路径",
      source: 'export const endpoint = "/api";',
      rule: "no-restricted-syntax",
    },
    {
      name: "动态业务接口路径",
      source: "export const endpoint = (id: string) => `/api/projects/${id}`;",
      rule: "no-restricted-syntax",
    },
  ])("拒绝 $name", async ({ source, rule }) => {
    const messages = await boundaryMessages(source);
    expect(messages).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ ruleId: rule, severity: 2 }),
      ]),
    );
  });

  it.each([
    {
      name: "生成 API 导入",
      source:
        'import { listProjects } from "@/api/projects"; export const load = listProjects;',
      filePath: "src/components/probe.ts",
    },
    {
      name: "错误类型和统一媒体传输入口",
      source:
        'import { ApiError, readResourceStream, type RequestOptions } from "@/lib/request"; export const error = ApiError; export const load = readResourceStream; export type Options = RequestOptions;',
      filePath: "src/components/probe.ts",
    },
    {
      name: "原生媒体使用授权 URL",
      source:
        "export const Preview = ({ authorizedUrl }: { authorizedUrl: string }) => <><video src={authorizedUrl} controls /><audio src={authorizedUrl} controls /></>;",
      filePath: "src/components/probe.tsx",
    },
    {
      name: "授权图片 URL",
      source:
        "export const image = (authorizedUrl: string) => { const image = new Image(); image.src = authorizedUrl; return image; };",
      filePath: "src/components/probe.ts",
    },
    {
      name: "普通页面路由",
      source: 'export const destination = "/projects";',
      filePath: "src/components/probe.ts",
    },
    {
      name: "与 API 前缀相似的页面路由",
      source: 'export const destination = "/api-guide";',
      filePath: "src/components/probe.ts",
    },
  ])("允许 $name", async ({ source, filePath }) => {
    expect(await boundaryMessages(source, filePath)).toEqual([]);
  });

  const lowLevelSource =
    'import axios from "axios"; import { request } from "@/lib/request"; export const client = axios; export const transport = request; export const load = () => fetch("/api/projects");';

  it.each([
    "src/api/probe.ts",
    "src/lib/request.ts",
    "src/components/probe.test.ts",
    "src/components/probe.test.tsx",
  ])("允许精确例外 %s", async (filePath) => {
    expect(await boundaryMessages(lowLevelSource, filePath)).toEqual([]);
  });

  it.each([
    "src/api-client/probe.ts",
    "src/lib/request-helper.ts",
    "src/components/probe.test-helper.ts",
  ])("拒绝相似但不属于例外的路径 %s", async (filePath) => {
    const rules = new Set(
      (await boundaryMessages(lowLevelSource, filePath)).map(
        (message) => message.ruleId,
      ),
    );
    expect(rules).toEqual(boundaryRules);
  });
});
