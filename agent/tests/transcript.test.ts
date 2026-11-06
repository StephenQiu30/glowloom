import { expect, it } from 'vitest';
import { businessTranscript } from '../src/business/transcript.js';
it('keeps the latest named context section and removes obsolete adopted constraints', () => {
  const context = {
    messages: [
      {
        role: 'system' as const,
        content: 'producer',
        sections: { context: 'obsolete-plan' },
        timestamp: 1,
      },
      {
        role: 'system' as const,
        content: '',
        sections: { context: 'current-plan' },
        timestamp: 2,
      },
    ],
  };
  const result = businessTranscript(context);
  expect(JSON.stringify(result)).toContain('current-plan');
  expect(JSON.stringify(result)).not.toContain('obsolete-plan');
});
