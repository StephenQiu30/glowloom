export class BusinessError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
  }
}

export interface RequestOptions {
  transport?: {
    origin: string;
    token: string;
    signal?: AbortSignal;
    timeoutMs?: number;
  };
}

export async function request<T>(
  path: string,
  options: RequestOptions & {
    method: string;
    headers?: Record<string, string>;
    data?: unknown;
    params?: Record<string, string | number | undefined>;
  },
): Promise<T> {
  const transport = options.transport;
  if (!transport) throw new Error('Business transport is required');
  const url = new URL(path, transport.origin);
  for (const [key, value] of Object.entries(options.params ?? {})) {
    if (value !== undefined) url.searchParams.set(key, String(value));
  }
  const timeout = AbortSignal.timeout(transport.timeoutMs ?? 10000);
  const response = await fetch(url, {
    method: options.method,
    headers: {
      ...options.headers,
      authorization: `Bearer ${transport.token}`,
      'x-agent-audience': 'agent-business',
    },
    ...(options.data === undefined
      ? {}
      : { body: JSON.stringify(options.data) }),
    signal: transport.signal
      ? AbortSignal.any([transport.signal, timeout])
      : timeout,
    redirect: 'error',
  });
  const reader = response.body?.getReader();
  if (!reader) throw new BusinessError(502, 'business_response_invalid');
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    for (;;) {
      const next = await reader.read();
      if (next.done) break;
      size += next.value.byteLength;
      if (size > 8 * 1024 * 1024)
        throw new BusinessError(502, 'business_response_too_large');
      chunks.push(next.value);
    }
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
  let value: unknown;
  try {
    value = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  } catch {
    throw new BusinessError(502, 'business_response_invalid');
  }
  if (!response.ok) {
    const code =
      value && typeof value === 'object' && 'code' in value
        ? String(value.code)
        : 'business_request_failed';
    throw new BusinessError(response.status, code);
  }
  return value as T;
}
