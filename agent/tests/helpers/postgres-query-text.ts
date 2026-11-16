/** pg accepts both text and QueryConfig; fault gates must observe either form. */
export function postgresQueryText(value: unknown): string {
  if (typeof value === 'string') return value;
  if (
    value &&
    typeof value === 'object' &&
    'text' in value &&
    typeof value.text === 'string'
  )
    return value.text;
  return '';
}
