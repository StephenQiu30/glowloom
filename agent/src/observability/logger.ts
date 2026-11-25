import pino, { type Logger } from 'pino';

export function createAgentLogger(
  level = process.env.GLOWLOOM_LOG_LEVEL ?? 'info',
): Logger {
  return pino({
    name: 'agent',
    level,
    base: { service: 'agent' },
    redact: {
      paths: [
        'req.headers.authorization',
        'authorization',
        'serviceToken',
        'businessToken',
        'dbDsn',
        'messages',
        'goal',
        'params',
        'manifest',
      ],
      remove: true,
    },
  });
}
