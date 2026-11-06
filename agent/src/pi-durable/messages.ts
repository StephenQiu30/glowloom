import type { RuntimeEvent } from './storage/postgres.js';
import { ROOT_CONVERSATION_ID } from '@earendil-works/pi-durable';
import { stableId } from '../sessions/manifest.js';
export interface PublicMessagePart {
  message_id: string;
  run_id: string;
  conversation_id: number;
  sequence: string;
  part_index: number;
  kind: string;
  content: string;
}
export function publicMessageParts(
  event: RuntimeEvent,
  seq: number,
  ordinal: number,
): PublicMessagePart[] {
  const {
    root_run_id: run,
    conversation_id: conversation,
    text,
  } = event.payload;
  if (
    event.type === 'operation' &&
    typeof run === 'string' &&
    typeof event.payload.operation_id === 'string' &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      event.payload.operation_id,
    )
  ) {
    const operation = event.payload.operation_id;
    return [
      {
        message_id: stableId(run, `operation:${operation}`),
        run_id: run,
        conversation_id: Number(ROOT_CONVERSATION_ID),
        sequence: `${seq}:${ordinal}`,
        part_index: 0,
        kind: 'operation',
        content: operation,
      },
    ];
  }

  if (
    event.type !== 'text' ||
    typeof run !== 'string' ||
    Number(conversation) !== Number(ROOT_CONVERSATION_ID) ||
    typeof text !== 'string' ||
    !text
  )
    return [];
  const parts: string[] = [];
  let part = '',
    bytes = 0;
  for (const character of text) {
    const size = Buffer.byteLength(character);
    if (bytes + size > 16384) {
      parts.push(part);
      part = '';
      bytes = 0;
    }
    part += character;
    bytes += size;
  }
  if (part) parts.push(part);
  const message = stableId(run, `answer:${seq}:${ordinal}`);
  return parts.map((content, part_index) => ({
    message_id: message,
    run_id: run,
    conversation_id: Number(conversation),
    sequence: `${seq}:${ordinal}`,
    part_index,
    kind: 'assistant',
    content,
  }));
}
