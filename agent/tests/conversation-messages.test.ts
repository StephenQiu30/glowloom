import { expect, it } from 'vitest';
import { ROOT_CONVERSATION_ID } from '@earendil-works/pi-durable';
import { stableId } from '../src/sessions/manifest.js';
import { publicMessageParts } from '../src/pi-durable/messages.js';
const run = '00000000-0000-4000-8000-000000000001';
it('preserves a long unicode answer in stable bounded pieces', () => {
  const text = '星海🌌'.repeat(9000);
  const event = {
    type: 'text',
    payload: {
      root_run_id: run,
      conversation_id: Number(ROOT_CONVERSATION_ID),
      text,
    },
  };
  const parts = publicMessageParts(event, 5, 2);
  expect(parts.length).toBeGreaterThan(1);
  expect(parts.map((part) => part.content).join('')).toBe(text);
  expect(parts.every((part) => Buffer.byteLength(part.content) <= 16384)).toBe(
    true,
  );
  expect(new Set(parts.map((part) => part.message_id)).size).toBe(1);
  expect(publicMessageParts(event, 5, 2)).toEqual(parts);
});
it("never turns a specialist's internal text or reasoning into the main answer", () => {
  expect(
    publicMessageParts(
      {
        type: 'text',
        payload: { root_run_id: run, conversation_id: 999, text: 'private' },
      },
      1,
      1,
    ),
  ).toEqual([]);
  expect(
    publicMessageParts(
      { type: 'thinking', payload: { root_run_id: run, text: 'secret' } },
      1,
      1,
    ),
  ).toEqual([]);
});
it('persists only a formal operation identity from a tool receipt', () => {
  const op = '00000000-0000-4000-8000-000000000005';
  expect(
    publicMessageParts(
      { type: 'operation', payload: { root_run_id: run, operation_id: op } },
      4,
      1,
    ),
  ).toEqual([
    {
      message_id: stableId(run, `operation:${op}`),
      run_id: run,
      conversation_id: Number(ROOT_CONVERSATION_ID),
      sequence: '4:1',
      part_index: 0,
      kind: 'operation',
      content: op,
    },
  ]);
  expect(
    publicMessageParts(
      {
        type: 'operation',
        payload: { root_run_id: run, operation_id: 'javascript:alert(1)' },
      },
      4,
      1,
    ),
  ).toEqual([]);
});
