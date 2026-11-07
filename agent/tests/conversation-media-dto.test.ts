import 'reflect-metadata';
import { plainToInstance } from 'class-transformer';
import { validate } from 'class-validator';
import { expect, it } from 'vitest';
import { AuthorizedInput } from '../src/http/session-request.dto.js';

it('validates the frozen ninety-reference envelope with actual document identities', async () => {
  const raw = {
    schema_version: 1,
    draft: {},
    input_hash: 'a'.repeat(64),
    plain_text: '',
    prompt: '',
    media_pins: ['image', 'video', 'document'].flatMap((mediaKind) =>
      Array.from({ length: 30 }, () => ({
        sourceId: '00000000-0000-4000-8000-000000000001',
        sourceVersion: '2',
        mediaKind,
      })),
    ),
  };
  expect(await validate(plainToInstance(AuthorizedInput, raw))).toEqual([]);
  expect(
    await validate(
      plainToInstance(AuthorizedInput, {
        ...raw,
        media_pins: [...raw.media_pins, raw.media_pins[0]],
      }),
    ),
  ).not.toEqual([]);
  expect(
    await validate(
      plainToInstance(AuthorizedInput, {
        ...raw,
        media_pins: [
          {
            sourceId: 'https://example.com/video',
            sourceVersion: '2',
            mediaKind: 'video',
          },
        ],
      }),
    ),
  ).not.toEqual([]);
});
