import { createHash } from 'node:crypto';

/** Shared by build and online contracts without importing the document renderer. */
export function digest(value: string | Uint8Array) {
  return createHash('sha256').update(value).digest('hex');
}

export function documentHref(id: string, anchor = '') {
  return `/read/${encodeURIComponent(id)}/${anchor ? `#${encodeURIComponent(anchor)}` : ''}`;
}
