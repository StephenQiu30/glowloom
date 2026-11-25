import pino, { type Logger } from 'pino';

export function createAgentLogger(
  level: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal' = 'info',
): Logger {
  return pino({
    name: 'glowloom-agent',
    level,
    redact: {
      paths: [
        'authorization',
        '*.authorization',
        '*.token',
        '*.serviceToken',
        '*.businessToken',
        '*.dbDsn',
        '*.secretKey',
        '*.apiKey',
        '*.prompt',
        '*.messages',
        '*.content',
      ],
      censor: '[REDACTED]',
    },
  });
}
