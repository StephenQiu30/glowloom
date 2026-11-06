import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
const extension = import.meta.url.endsWith('.ts') ? '.ts' : '.js';
const definitions = [
  'registry',
  'model-provider',
  'docs',
  'telemetry',
  'events',
];
export const DEFINITION_HASH = createHash('sha256')
  .update(
    definitions
      .map(
        (name) =>
          name +
          '\0' +
          readFileSync(new URL(name + extension, import.meta.url), 'utf8'),
      )
      .join('\0'),
  )
  .digest('hex');
