import { timingSafeEqual } from 'node:crypto';

export type AuthFailureReason = 'unauthorized' | 'invalid_audience';

export function verifyServiceIdentity(
  authorizationHeader: string | undefined,
  audienceHeader: string | undefined,
  expectedAuthorization: Buffer,
): AuthFailureReason | null {
  const actual = Buffer.from(authorizationHeader ?? '');
  if (
    actual.length !== expectedAuthorization.length ||
    !timingSafeEqual(actual, expectedAuthorization)
  ) {
    return 'unauthorized';
  }
  if (audienceHeader !== 'agent') {
    return 'invalid_audience';
  }
  return null;
}
