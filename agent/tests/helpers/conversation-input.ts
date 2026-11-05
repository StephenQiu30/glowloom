import { createHash } from 'node:crypto';
/** Trusted Go-boundary fixture; never a product fallback for missing input. */
export function authorizedText(text: string) {
  return {
    schema_version: 1 as const,
    draft: {
      schemaVersion: 1,
      document: {
        type: 'doc',
        content: [{ type: 'paragraph', content: [{ type: 'text', text }] }],
      },
      references: [],
    },
    input_hash: createHash('sha256').update(text).digest('hex'),
    plain_text: text,
    prompt: text,
    media_pins: [],
  };
}
