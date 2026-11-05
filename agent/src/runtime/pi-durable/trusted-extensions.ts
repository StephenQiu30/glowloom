import {
  chmod,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  realpath,
  rm,
} from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import {
  SettingsManager,
  DefaultPackageManager,
  discoverAndLoadExtensions,
  type ToolDefinition,
  type ExtensionToolContext,
  type ExtensionRuntime,
} from '@earendil-works/pi-coding-agent';
import type { JsonObject } from '@earendil-works/pi-durable';
import type { ImageContent, TextContent } from '@earendil-works/pi-ai';

export interface TrustedTool {
  readonly name: string;
  readonly description: string;
  readonly parameters: Record<string, unknown>;
  readonly replay: 'unsafe';
  readonly executionMode: 'sequential' | 'parallel';
  invoke(
    callId: string,
    args: JsonObject,
    signal: AbortSignal | undefined,
  ): Promise<{ content: (TextContent | ImageContent)[]; details?: unknown }>;
}
export interface TrustedEntry {
  path: string;
  sha256: string;
}
/** This is an image allowlist, never tenant input. The SDK is not a JavaScript sandbox. */
export async function loadTrustedExtensions(options: {
  imageRoot: string;
  entries: readonly TrustedEntry[];
}): Promise<{ tools: readonly TrustedTool[]; close(): Promise<void> }> {
  let runtime: ExtensionRuntime | undefined;
  const scratch = await mkdtemp(join(tmpdir(), 'glowloom-sdk-'));
  const cwd = join(scratch, 'cwd');
  const agentDir = join(scratch, 'agent');
  const empty = [join(cwd, '.pi', 'extensions'), join(agentDir, 'extensions')];
  const close = async () => {
    runtime?.invalidate('Trusted extension host closed');
    for (const p of [scratch, cwd, join(cwd, '.pi'), ...empty, agentDir])
      await chmod(p, 0o700).catch(() => {});
    await rm(scratch, { recursive: true, force: true });
  };
  try {
    for (const p of empty) await mkdir(p, { recursive: true });
    for (const p of [...empty, join(cwd, '.pi'), cwd, agentDir, scratch])
      await chmod(p, 0o555);
    const checkEmpty = async () => {
      for (const p of empty) {
        if ((await readdir(p)).length)
          throw new Error('Unexpected SDK discovery entry');
        if ((await lstat(p)).mode & 0o222)
          throw new Error('SDK discovery root must be read-only');
      }
    };
    await checkEmpty();
    const allowed = new Set<string>();
    if (options.entries.length) {
      const root = await realpath(options.imageRoot);
      for (const entry of options.entries) {
        if (
          !entry.path ||
          isAbsolute(entry.path) ||
          entry.path.split(/[\\/]/).includes('..') ||
          /[:~]/.test(entry.path)
        )
          throw new Error(
            'Only reviewed image-relative local entries are allowed',
          );
        const path = resolve(root, entry.path);
        const actual = await realpath(path);
        if (
          actual !== path ||
          relative(root, actual).startsWith('..' + sep) ||
          !relative(root, actual)
        )
          throw new Error('Extension escapes the reviewed image root');
        if (allowed.has(actual)) throw new Error('Duplicate trusted extension');
        const stat = await lstat(actual);
        if (!stat.isFile() || stat.mode & 0o222)
          throw new Error('Trusted extension must be a read-only file');
        for (let parent = dirname(actual); ; parent = dirname(parent)) {
          if ((await lstat(parent)).mode & 0o222)
            throw new Error('Trusted extension directory must be read-only');
          if (parent === root) break;
        }
        if (
          !/^[a-f0-9]{64}$/.test(entry.sha256) ||
          createHash('sha256')
            .update(await readFile(actual))
            .digest('hex') !== entry.sha256
        )
          throw new Error('Trusted extension hash mismatch');
        allowed.add(actual);
      }
    }
    const settingsManager = SettingsManager.inMemory(
      { packages: [], extensions: [], skills: [], prompts: [], themes: [] },
      { projectTrusted: false },
    );
    const manager = new DefaultPackageManager({
      cwd,
      agentDir,
      settingsManager,
      builtinExtensions: [],
    });
    // Local files have been validated before this API: it must never see npm/git/URL inputs.
    const resolved = await manager.resolveExtensionSources([...allowed]);
    if (
      resolved.installFailures?.length ||
      resolved.skills.length ||
      resolved.prompts.length ||
      resolved.themes.length ||
      resolved.extensions.length !== allowed.size
    )
      throw new Error('Unexpected SDK package resources');
    for (const resource of resolved.extensions)
      if (!resource.enabled || !allowed.has(await realpath(resource.path)))
        throw new Error('Unapproved SDK package resource');
    const loaded = await discoverAndLoadExtensions(
      resolved.extensions.map((x) => x.path),
      cwd,
      agentDir,
    );
    runtime = loaded.runtime;
    await checkEmpty();
    if (
      runtime.pendingProviderRegistrations.length ||
      runtime.pendingNativeProviderRegistrations.length ||
      runtime.pendingVirtualModelRegistrations.length ||
      runtime.mcpServers.list().length
    )
      throw new Error(
        'Unsupported SDK provider or MCP registration in tool-only extension',
      );
    if (loaded.errors.length || loaded.warnings?.length)
      throw new Error(
        'Trusted SDK extension load failed: ' +
          loaded.errors.map((x) => x.error).join(';'),
      );
    if (loaded.extensions.length !== allowed.size)
      throw new Error('Unexpected SDK extension discovery');
    const tools: TrustedTool[] = [];
    const names = new Set<string>();
    for (const extension of loaded.extensions) {
      if (!allowed.has(await realpath(extension.resolvedPath)))
        throw new Error('Unapproved discovered extension');
      if (extension.handlers.size)
        throw new Error(
          'Unsupported SDK lifecycle hook in tool-only extension',
        );
      if (
        extension.commands.size ||
        extension.flags.size ||
        extension.shortcuts.size ||
        extension.messageRenderers.size ||
        extension.markdownTransformer ||
        extension.toolRenderers?.length ||
        extension.entryRenderers?.size
      )
        throw new Error('Unsupported SDK UI or runtime extension capability');
      for (const { definition } of extension.tools.values()) {
        if (
          names.has(definition.name) ||
          !/^[a-zA-Z][a-zA-Z0-9_.-]{0,95}$/.test(definition.name)
        )
          throw new Error('Duplicate or invalid trusted tool');
        if (
          definition.prepareArguments ||
          definition.prepareLoadout ||
          definition.reachesTools ||
          definition.outputSchema ||
          definition.constrainedSampling ||
          definition.namespace ||
          (definition.exposure && definition.exposure !== 'direct')
        )
          throw new Error('Unsupported SDK tool lifecycle capability');
        names.add(definition.name);
        // SDK actions require its other AgentSession and are deliberately unavailable here.
        const unavailable = new Proxy({} as ExtensionToolContext, {
          get: () => {
            throw new Error(
              'SDK session context is unsupported in a tool-only trusted extension',
            );
          },
        });
        tools.push({
          name: definition.name,
          description: definition.description,
          parameters: definition.parameters as unknown as Record<
            string,
            unknown
          >,
          replay: 'unsafe',
          executionMode: definition.executionMode ?? 'sequential',
          invoke: async (callId, args, signal) => {
            signal?.throwIfAborted();
            return (definition as ToolDefinition).execute(
              callId,
              args,
              signal,
              undefined,
              unavailable,
            );
          },
        });
      }
    }
    return { tools, close };
  } catch (error) {
    await close();
    throw error;
  }
}
