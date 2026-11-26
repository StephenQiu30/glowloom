import { describe, expect, it } from 'vitest';
import { loadConfig } from '../src/config.js';
import { PromptRegistry } from '../src/observability/langfuse.js';
import { createAgentLogger } from '../src/observability/logger.js';
import {
  filterToolsByAllowlist,
  loadDeclarativeSkills,
} from '../src/skills/loader.js';

describe('config, declarative skills, and prompt fallback', () => {
  it.each([undefined, '', '  '])(
    'loads Agent config with an unset or blank Langfuse URL (%j)',
    (langfuseBaseURL) => {
      const config = loadConfig({
        GLOWLOOM_AGENT_DB_DSN:
          'postgres://glowloom_agent:secret@127.0.0.1:5432/glowloom',
        GLOWLOOM_AGENT_SERVICE_TOKEN: 's'.repeat(32),
        GLOWLOOM_AGENT_BUSINESS_ORIGIN: 'http://backend:18080/',
        GLOWLOOM_AGENT_BUSINESS_TOKEN: 'b'.repeat(32),
        LANGFUSE_BASE_URL: langfuseBaseURL,
      });

      expect(config.httpHost).toBe('0.0.0.0');
      expect(config.httpPort).toBe(4401);
      expect(config.grpcAddr).toBe('0.0.0.0:4402');
      expect(config.businessOrigin).toBe('http://backend:18080');
      expect(config.langfuse).toBeUndefined();
    },
  );

  it('loads declarative SKILL.md bundles and filters MCP tools strictly by allowlist', async () => {
    const skills = await loadDeclarativeSkills();
    expect(skills.length).toBeGreaterThanOrEqual(2);
    const scriptSkill = skills.find((s) => s.name === 'script-breakdown');
    expect(scriptSkill).toBeDefined();
    expect(scriptSkill?.allowedTools).toContain('read_project_script');

    const filtered = filterToolsByAllowlist(
      {
        glowloom_read_project_script: { name: 'read_project_script' },
        glowloom_submit_storyboard_generation: {
          name: 'submit_storyboard_generation',
        },
      },
      scriptSkill?.allowedTools ?? [],
    );
    expect(Object.keys(filtered)).toEqual(['glowloom_read_project_script']);
  });

  it('falls back to local prompts/ when Langfuse is not configured', async () => {
    const logger = createAgentLogger('error');
    const registry = new PromptRegistry(undefined, logger);
    const prompt = await registry.resolve('producer');
    expect(prompt).toContain('Producer');
    await registry.shutdown();
  });
});
