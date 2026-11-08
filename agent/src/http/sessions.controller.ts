import {
  Body,
  Controller,
  Get,
  Headers,
  HttpCode,
  Inject,
  Param,
  Patch,
  Post,
  Query,
  Res,
} from '@nestjs/common';
import {
  ApiBearerAuth,
  ApiHeader,
  ApiOperation,
  ApiProduces,
  ApiResponse,
  ApiTags,
} from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import { streamEvents } from './events.js';
import { AgentLifecycle } from './lifecycle.js';
import { checkedResponse } from './validation.js';
import { AgentError } from '../sessions/errors.js';
import type { AgentApplication } from '../sessions/types.js';
import {
  ControlRequest,
  CreateRunRequest,
  CreateSessionRequest,
  HistoryQuery,
  IdentityQuery,
  MessageParams,
  MessageQuery,
  RunParams,
  SessionParams,
  SubmissionParams,
  UpdateSessionRequest,
} from './session-request.dto.js';
import {
  AcceptedRun,
  ControlReceipt,
  CurrentSubmission,
  MessagePage,
  PublicMessage,
  RunPage,
  RunSnapshot,
  SessionPage,
  SessionView,
} from './session-response.dto.js';

export const agentApplication = Symbol('AgentApplication');

@ApiTags('agent')
@ApiBearerAuth('serviceIdentity')
@ApiHeader({
  name: 'X-Agent-Audience',
  required: true,
  schema: { type: 'string', enum: ['agent'] },
})
@ApiResponse({ status: 400, description: 'Invalid input' })
@ApiResponse({ status: 401, description: 'Service identity required' })
@ApiResponse({ status: 503, description: 'Agent unavailable' })
@Controller('internal/agent')
export class SessionsController {
  constructor(
    @Inject(agentApplication) private readonly service: AgentApplication,
    private readonly lifecycle: AgentLifecycle,
  ) {}

  @Post('sessions')
  @ApiOperation({ operationId: 'createAgentSession' })
  @ApiResponse({ status: 201, type: SessionView })
  async createSession(
    @Body() input: CreateSessionRequest,
  ): Promise<SessionView> {
    return checkedResponse(
      SessionView,
      await this.service.createSession(input),
    );
  }

  @Get('sessions')
  @ApiOperation({ operationId: 'listAgentSessions' })
  @ApiResponse({ status: 200, type: SessionPage })
  async listSessions(@Query() query: HistoryQuery): Promise<SessionPage> {
    return checkedResponse(SessionPage, await this.service.listSessions(query));
  }

  @Get('sessions/:sid')
  @ApiOperation({ operationId: 'getAgentSession' })
  @ApiResponse({ status: 200, type: SessionView })
  async readSession(
    @Param() { sid }: SessionParams,
    @Query() identity: IdentityQuery,
  ): Promise<SessionView> {
    const session = await this.service.readSession(sid, identity);
    if (session === undefined) throw new AgentError('session_not_found');
    return checkedResponse(SessionView, session);
  }

  @Patch('sessions/:sid')
  @ApiOperation({ operationId: 'updateAgentSession' })
  @ApiResponse({ status: 200, type: SessionView })
  async updateSession(
    @Param() { sid }: SessionParams,
    @Body() input: UpdateSessionRequest,
  ): Promise<SessionView> {
    return checkedResponse(
      SessionView,
      await this.service.updateSession(sid, input),
    );
  }

  @Get('sessions/:sid/runs')
  @ApiOperation({ operationId: 'listAgentRuns' })
  @ApiResponse({ status: 200, type: RunPage })
  async listRuns(
    @Param() { sid }: SessionParams,
    @Query() query: HistoryQuery,
  ): Promise<RunPage> {
    return checkedResponse(RunPage, await this.service.listRuns(sid, query));
  }

  @Get('sessions/:sid/submissions/current')
  @ApiOperation({ operationId: 'getCurrentAgentSubmission' })
  @ApiResponse({ status: 200, type: CurrentSubmission })
  async currentSubmission(
    @Param() { sid }: SessionParams,
    @Query() identity: IdentityQuery,
  ): Promise<CurrentSubmission> {
    return checkedResponse(
      CurrentSubmission,
      await this.service.currentSubmission(sid, identity),
    );
  }

  @Get('sessions/:sid/submissions/:requestId')
  @ApiOperation({ operationId: 'getAgentSubmission' })
  @ApiResponse({ status: 200, type: AcceptedRun })
  async readSubmission(
    @Param() { sid, requestId }: SubmissionParams,
    @Query() identity: IdentityQuery,
  ): Promise<AcceptedRun> {
    const submission = await this.service.readSubmission(
      sid,
      requestId,
      identity,
    );
    if (!submission) throw new AgentError('submission_not_found');
    return checkedResponse(AcceptedRun, submission);
  }

  @Get('runs/:rid/messages')
  @ApiOperation({ operationId: 'listAgentMessages' })
  @ApiResponse({ status: 200, type: MessagePage })
  async listMessages(
    @Param() { rid }: RunParams,
    @Query() query: MessageQuery,
  ): Promise<MessagePage> {
    return checkedResponse(
      MessagePage,
      await this.service.listMessages(rid, query),
    );
  }

  @Get('runs/:rid/messages/:messageId')
  @ApiOperation({ operationId: 'getAgentMessage' })
  @ApiResponse({ status: 200, type: PublicMessage })
  async readMessage(
    @Param() { rid, messageId }: MessageParams,
    @Query() identity: IdentityQuery,
  ): Promise<PublicMessage> {
    const message = await this.service.readPublicMessage(
      rid,
      messageId,
      identity,
    );
    if (!message) throw new AgentError('message_not_found');
    return checkedResponse(PublicMessage, message);
  }

  @Post('sessions/:sid/runs')
  @HttpCode(202)
  @ApiOperation({ operationId: 'createAgentRun' })
  @ApiResponse({ status: 202, type: AcceptedRun })
  async createRun(
    @Param() { sid }: SessionParams,
    @Body() input: CreateRunRequest,
  ): Promise<AcceptedRun> {
    return checkedResponse(
      AcceptedRun,
      await this.service.createRun(sid, input),
    );
  }

  @Get('runs/:rid')
  @ApiOperation({ operationId: 'getAgentRun' })
  @ApiResponse({ status: 200, type: RunSnapshot })
  async readRun(
    @Param() { rid }: RunParams,
    @Query() identity: IdentityQuery,
  ): Promise<RunSnapshot> {
    const run = await this.service.readRun(rid, identity);
    if (run === undefined) throw new AgentError('run_not_found');
    return checkedResponse(RunSnapshot, run);
  }

  @Post('runs/:rid/commands')
  @HttpCode(200)
  @ApiOperation({ operationId: 'applyAgentDecision' })
  @ApiResponse({ status: 200, type: ControlReceipt })
  async command(
    @Param() { rid }: RunParams,
    @Body() input: ControlRequest,
  ): Promise<ControlReceipt> {
    return checkedResponse(
      ControlReceipt,
      await this.service.command(rid, input),
    );
  }

  @Get('runs/:rid/events')
  @ApiOperation({ operationId: 'subscribeAgentEvents' })
  @ApiHeader({ name: 'Last-Event-ID', required: false })
  @ApiProduces('text/event-stream')
  @ApiResponse({
    status: 200,
    description: 'Committed Agent event stream',
    schema: { type: 'string' },
  })
  @ApiResponse({ status: 409, description: 'Snapshot required' })
  @ApiResponse({ status: 410, description: 'Event cursor expired' })
  async events(
    @Param() { rid }: RunParams,
    @Query() identity: IdentityQuery,
    @Headers('last-event-id') lastEventId: string | string[] | undefined,
    @Res() reply: FastifyReply,
  ): Promise<void> {
    await streamEvents(
      reply,
      this.lifecycle,
      this.service,
      rid,
      identity,
      Array.isArray(lastEventId) ? lastEventId[0] : lastEventId,
    );
  }
}
