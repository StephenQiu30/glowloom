import { z } from 'zod';
import type { NarrationManifest } from './manifest';

const manifestContract = z.object({
  documentId: z.string(),
  revision: z.string(),
  scope: z.string().min(1),
  configDigest: z.string().min(1),
  precision: z.enum(['sentence', 'paragraph', 'segment', 'none']),
  segments: z
    .array(
      z.object({
        id: z.string(),
        src: z
          .string()
          .regex(/^[\w/-]+\.(wav|mp3|m4a|ogg|opus)$/)
          .refine((value) => !value.includes('..') && !value.startsWith('/')),
        sha256: z.string().regex(/^[a-f0-9]{64}$/),
        durationMs: z.number().positive(),
        cues: z.array(
          z.object({
            startMs: z.number().nonnegative(),
            endMs: z.number().positive(),
            blockId: z.string(),
            textStart: z.number().int().nonnegative(),
            textEnd: z.number().int().positive(),
          }),
        ),
      }),
    )
    .min(1),
}) satisfies z.ZodType<NarrationManifest>;
const resumeContract = z.object({
  segment: z.number().int().nonnegative(),
  seconds: z.number().nonnegative(),
});

/** Rejects malformed browser resources before media state or paths are accessed. */
export function decodeManifest(value: unknown): NarrationManifest {
  const result = manifestContract.safeParse(value);
  if (!result.success) {
    throw new Error('朗读资源格式无效，请重新导入并发布。');
  }
  return result.data;
}

/** Invalid or obsolete local progress is optional and never blocks reading. */
export function decodeResume(value: unknown) {
  const result = resumeContract.safeParse(value);
  return result.success ? result.data : null;
}
