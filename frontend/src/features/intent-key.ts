export function newIntentKey(scope: string): string {
  return `${scope}:${crypto.randomUUID()}`;
}
