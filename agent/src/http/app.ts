import 'reflect-metadata';
import { timingSafeEqual } from 'node:crypto';
import { Controller, Get, Inject, Module, Res } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { FastifyAdapter } from '@nestjs/platform-fastify';
import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { ApiExcludeController, SwaggerModule } from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import type { Logger } from 'pino';
import type { AgentApplication } from '../sessions/types.js';
import { AgentExceptionFilter } from './errors.js';
import { AgentLifecycle } from './lifecycle.js';
import { agentApplication, SessionsController } from './sessions.controller.js';
import { createOpenApi } from './openapi.js';
import { requestValidation } from './validation.js';

@ApiExcludeController()
@Controller()
class HealthController {
  constructor(
    @Inject(agentApplication) private readonly service: AgentApplication,
    private readonly lifecycle: AgentLifecycle,
  ) {}

  @Get('healthz')
  health() {
    return { status: 'ok', service: 'agent' };
  }

  @Get('readyz')
  async ready(@Res({ passthrough: true }) reply: FastifyReply) {
    const ready = !this.lifecycle.draining && (await this.service.ready());
    reply.status(ready ? 200 : 503);
    return { ready };
  }
}

@Module({ controllers: [HealthController, SessionsController] })
class AgentHttpModule {}

export async function buildApp(options: {
  service: AgentApplication;
  serviceToken: string;
  logger?: Logger;
  swagger?: boolean;
}): Promise<NestFastifyApplication> {
  const adapter = new FastifyAdapter({
    bodyLimit: 8 * 1024 * 1024,
    requestTimeout: 30000,
    ...(options.logger
      ? { loggerInstance: options.logger }
      : { logger: false }),
  });
  const lifecycle = new AgentLifecycle(options.service);
  const app = await NestFactory.create<NestFastifyApplication>(
    {
      module: AgentHttpModule,
      providers: [
        { provide: agentApplication, useValue: options.service },
        { provide: AgentLifecycle, useValue: lifecycle },
      ],
    },
    adapter,
    { logger: false, abortOnError: false },
  );
  const token = Buffer.from(`Bearer ${options.serviceToken}`);
  adapter.getInstance().addHook('onRequest', async (request, reply) => {
    const path = request.url.split('?', 1)[0] ?? '';
    const internal = path.startsWith('/internal/');
    const docs =
      options.swagger &&
      (path === '/docs' ||
        path.startsWith('/docs/') ||
        path === '/openapi.json');
    if (!internal && !docs) return;
    const supplied = Buffer.from(request.headers.authorization ?? '');
    if (
      request.headers['x-agent-audience'] !== 'agent' ||
      supplied.length !== token.length ||
      !timingSafeEqual(supplied, token)
    ) {
      return reply.code(401).send({ code: 'service_identity_required' });
    }
    if (lifecycle.draining) {
      return reply
        .code(503)
        .send({ code: 'shutting_down', request_id: request.id });
    }
  });
  app.useGlobalFilters(new AgentExceptionFilter());
  app.useGlobalPipes(requestValidation());
  if (options.swagger) {
    SwaggerModule.setup('docs', app, () => createOpenApi(app), {
      jsonDocumentUrl: '/openapi.json',
      raw: ['json'],
    });
  }
  await app.init();
  return app;
}
