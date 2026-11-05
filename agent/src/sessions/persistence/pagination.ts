import { createHmac, randomBytes, timingSafeEqual } from 'node:crypto';
import { AgentError } from '../errors.js';
export interface PageCursor {
  scope: string;
  upper: string;
  before: string;
  id?: string;
}
export class ScopedCursors {
  private readonly key = randomBytes(32);
  encode(cursor: PageCursor) {
    const body = Buffer.from(JSON.stringify(cursor)).toString('base64url');
    return (
      body +
      '.' +
      createHmac('sha256', this.key).update(body).digest('base64url')
    );
  }
  decode(value: string | undefined, scope: string): PageCursor | undefined {
    if (!value) return;
    try {
      const [body, signature, extra] = value.split('.');
      const actual = Buffer.from(signature ?? '', 'base64url');
      const expected = createHmac('sha256', this.key)
        .update(body ?? '')
        .digest();
      if (
        extra ||
        actual.length !== expected.length ||
        !timingSafeEqual(actual, expected)
      )
        throw new Error();
      const cursor = JSON.parse(
        Buffer.from(body!, 'base64url').toString('utf8'),
      ) as PageCursor;
      if (
        cursor.scope !== scope ||
        typeof cursor.upper !== 'string' ||
        typeof cursor.before !== 'string'
      )
        throw new Error();
      return cursor;
    } catch {
      throw new AgentError('cursor_expired');
    }
  }
}
export function eventPosition(value: string): [string, number] {
  if (!/^(0|[1-9]\d*):(0|[1-9]\d*)$/.test(value) || value.length > 64)
    throw new AgentError('invalid_cursor');
  const [seq, ordinal] = value.split(':');
  if (
    BigInt(seq!) > BigInt(Number.MAX_SAFE_INTEGER) ||
    !Number.isSafeInteger(Number(ordinal)) ||
    Number(ordinal) > 2147483647
  )
    throw new AgentError('invalid_cursor');
  return [seq!, Number(ordinal)];
}
