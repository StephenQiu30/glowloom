import { timingSafeEqual } from 'node:crypto';

export type IdentityFailure = 'unauthorized' | 'invalid_audience' | null;

export function verifyServiceIdentity(
  authorization: string | undefined,
  audience: string | undefined,
  expectedAuthorization: Buffer,
): IdentityFailure {
  if (!authorization) {
    return 'unauthorized';
  }
  const actual = Buffer.from(authorization);
  if (
    actual.length !== expectedAuthorization.length ||
    !timingSafeEqual(actual, expectedAuthorization)
  ) {
    return 'unauthorized';
  }
  if (audience !== 'agent') {
    return 'invalid_audience';
  }
  return null;
}
