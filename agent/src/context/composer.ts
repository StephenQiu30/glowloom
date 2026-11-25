import type { JsonObject } from '@earendil-works/pi-durable';
import type { businessTranscript } from '../business/transcript.js';

type TranscriptInput = ReturnType<typeof businessTranscript>;

export interface ContextComposeOptions {
  readonly instructions: string;
  readonly snapshot: Readonly<JsonObject>;
  readonly changedSincePrevious: boolean;
  readonly isFirstModelStep: boolean;
  readonly memoryDeltaNotice?: string;
}

/**
 * Composes model messages using a cache-friendly "static prefix + dynamic tail" layout:
 * - System instructions stay strictly static at index 0 so provider KV/prefix caches hit across turns.
 * - Mutable project snapshots and memory re-anchor notices are appended at the tail only when
 *   actually needed (first turn or changed state), never spliced at index 1.
 */
export function composeModelMessages(
  transcriptInput: TranscriptInput,
  options: ContextComposeOptions,
): TranscriptInput {
  const messages = transcriptInput.messages.map((message) => ({ ...message }));
  const first = messages[0];
  if (first?.role === 'system' && typeof first.content === 'string') {
    first.content = `${first.content}\n\n${options.instructions}`;
  } else {
    messages.unshift({
      role: 'system',
      content: options.instructions,
    });
  }

  const tailParts: string[] = [];
  if (options.isFirstModelStep || options.changedSincePrevious) {
    tailParts.push(
      `Current project snapshot:\n${JSON.stringify(options.snapshot)}`,
    );
  }
  if (options.memoryDeltaNotice) {
    tailParts.push(options.memoryDeltaNotice);
  }
  if (tailParts.length > 0) {
    messages.push({
      role: 'user',
      content: tailParts.join('\n\n'),
    });
  }

  return {
    ...transcriptInput,
    messages,
  };
}
