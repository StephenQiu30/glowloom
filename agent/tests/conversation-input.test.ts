import { expect, it, vi } from 'vitest';
import { resolveConversationContent } from '../src/sessions/input.js';
import type { AuthorizedInput } from '../src/sessions/types.js';
const asset = '00000000-0000-4000-8000-000000000001';
const input: AuthorizedInput = {
  schema_version: 1,
  draft: {},
  input_hash: 'a'.repeat(64),
  plain_text: '',
  prompt: '',
  media_pins: [{ sourceId: asset, sourceVersion: '2', mediaKind: 'image' }],
};
it('puts authorized image bytes into the official user input without inventing typed text', async () => {
  const content = await resolveConversationContent(input, async () => ({
    asset_id: asset,
    revision: 2,
    content: [{ type: 'image', mimeType: 'image/png', data: 'aW1hZ2U=' }],
  }));
  expect(content).toContainEqual({
    type: 'image',
    mimeType: 'image/png',
    data: 'aW1hZ2U=',
  });
  expect(input.plain_text).toBe('');
  expect(JSON.stringify(content)).not.toContain('https:');
});
it('fails closed if the current asset version differs from the frozen pin', async () => {
  await expect(
    resolveConversationContent(input, async () => ({
      asset_id: asset,
      revision: 3,
      content: [],
    })),
  ).rejects.toThrow(/version/);
});
it('keeps a quote separate from the new instruction', async () => {
  const read = vi.fn();
  const content = await resolveConversationContent(
    {
      ...input,
      plain_text: '改为夜景',
      prompt: '改为夜景',
      media_pins: [],
      quote: {
        run_id: asset,
        message_id: 'message',
        text: '原回复',
        content_hash: 'b'.repeat(64),
      },
    },
    read,
  );
  expect(content).toContainEqual({ type: 'text', text: '改为夜景' });
  expect(
    content.some(
      (part) => part.type === 'text' && part.text.includes('原回复'),
    ),
  ).toBe(true);
  expect(read).not.toHaveBeenCalled();
});
it('rejects media that the reader cannot inspect instead of sending only a filename', async () => {
  await expect(
    resolveConversationContent(input, async () => ({
      asset_id: asset,
      revision: 2,
      content: [{ type: 'image', mimeType: 'image/webp', data: 'aW1hZ2U=' }],
    })),
  ).rejects.toThrow(/image/);
});

it('passes timestamped video frames and the sampling limitation to the model', async () => {
  const content = await resolveConversationContent(
    {
      ...input,
      media_pins: [{ sourceId: asset, sourceVersion: '2', mediaKind: 'video' }],
    },
    async () => ({
      asset_id: asset,
      revision: 2,
      media_kind: 'video',
      content: [
        {
          type: 'text',
          text: 'Video sample at 0 ms. Only sampled frames; audio has not been inspected.',
        },
        { type: 'image', mimeType: 'image/jpeg', data: 'aW1hZ2U=' },
        { type: 'text', text: 'Video sample at 1000 ms.' },
        { type: 'image', mimeType: 'image/jpeg', data: 'aW1hZ2U=' },
        { type: 'text', text: 'Video sample at 2000 ms.' },
        { type: 'image', mimeType: 'image/jpeg', data: 'aW1hZ2U=' },
      ],
    }),
  );
  expect(content.filter((part) => part.type === 'image')).toHaveLength(3);
  expect(JSON.stringify(content)).toContain('audio has not been inspected');
  expect(JSON.stringify(content)).toContain('1000 ms');
});

it('reads complete document text and rejects oversized context without truncating it', async () => {
  const document: AuthorizedInput = {
    ...input,
    media_pins: [
      { sourceId: asset, sourceVersion: '2', mediaKind: 'document' },
    ],
  };
  const read = async () => ({
    asset_id: asset,
    revision: 2,
    media_kind: 'document',
    content: [{ type: 'text', text: '真实小说正文\n第二章完整结尾' }],
  });
  expect(await resolveConversationContent(document, read)).toContainEqual({
    type: 'text',
    text: '真实小说正文\n第二章完整结尾',
  });
  await expect(resolveConversationContent(document, read, 2)).rejects.toThrow(
    /context/,
  );
});

it('allows thirty source images and rejects a thirty-first source before reading', async () => {
  const images: AuthorizedInput = {
    ...input,
    media_pins: Array.from({ length: 30 }, (_, index) => ({
      sourceId: `asset-${index}`,
      sourceVersion: '2',
      mediaKind: 'image' as const,
    })),
  };
  const read = vi.fn(async (pin: AuthorizedInput['media_pins'][number]) => ({
    asset_id: pin.sourceId,
    revision: 2,
    content: [{ type: 'image', mimeType: 'image/png', data: 'aW1hZ2U=' }],
  }));
  expect(
    (await resolveConversationContent(images, read)).filter(
      (part) => part.type === 'image',
    ),
  ).toHaveLength(30);
  read.mockClear();
  await expect(
    resolveConversationContent(
      { ...images, media_pins: [...images.media_pins, images.media_pins[0]!] },
      read,
    ),
  ).rejects.toThrow(/limit/);
  expect(read).not.toHaveBeenCalled();
});
