import { ApiProperty } from '@nestjs/swagger';
import { Expose, Type } from 'class-transformer';
import {
  IsArray,
  IsBoolean,
  IsISO8601,
  IsInt,
  IsObject,
  IsString,
  IsUUID,
  MaxLength,
  Min,
  ValidateIf,
  ValidateNested,
} from 'class-validator';
import { whenNonNull, whenPresent } from './validation.js';
import { AuthorizedInput, ScopeDto } from './session-request.dto.js';

export class PlanItem {
  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  id!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  title!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  status!: string;

  @ApiProperty({ required: false, type: 'string', isArray: true })
  @Expose()
  @ValidateIf(whenPresent)
  @IsArray()
  @IsString({ each: true })
  depends_on?: string[];

  @ApiProperty({ required: false, type: 'string', isArray: true })
  @Expose()
  @ValidateIf(whenPresent)
  @IsArray()
  @IsString({ each: true })
  artifacts?: string[];
}

export class ProposalView {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  proposal_id!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  candidate_hash!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  authorization_revision!: number;

  @ApiProperty({ type: Object, additionalProperties: true })
  @Expose()
  @IsObject()
  candidate!: Record<string, unknown>;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  canvas_revision?: number;

  @ApiProperty({ required: false, type: 'string' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsString()
  price_version?: string;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  quote_micros?: number;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  budget_micros?: number;
}

export class DelegationView {
  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  task_id!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  conversation_id!: number;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  role!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  status!: string;

  @ApiProperty({ required: false, type: 'string', isArray: true })
  @Expose()
  @ValidateIf(whenPresent)
  @IsArray()
  @IsString({ each: true })
  source_refs?: string[];

  @ApiProperty({ required: false, type: 'boolean' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsBoolean()
  adopted?: boolean;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  model_budget?: number;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  tool_budget?: number;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  model_used?: number;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  tool_used?: number;
}

export class AgentState {
  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  status!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  execution_revision!: number;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  authorization_revision?: number;

  @ApiProperty({ required: false, type: 'string', format: 'uuid' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsUUID()
  root_run_id?: string;

  @ApiProperty({ required: false, type: 'string' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsString()
  segment_id?: string;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  model_steps?: number;

  @ApiProperty({ required: false, type: 'integer', format: 'int64' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsInt()
  tool_steps?: number;

  @ApiProperty({ required: false, type: 'string' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsString()
  goal?: string;

  @ApiProperty({ required: false, type: () => PlanItem, isArray: true })
  @Expose()
  @ValidateIf(whenPresent)
  @IsArray()
  @IsObject({ each: true })
  @ValidateNested({ each: true })
  @Type(() => PlanItem)
  plan?: PlanItem[];

  @ApiProperty({ required: false, type: () => DelegationView, isArray: true })
  @Expose()
  @ValidateIf(whenPresent)
  @IsArray()
  @IsObject({ each: true })
  @ValidateNested({ each: true })
  @Type(() => DelegationView)
  delegations?: DelegationView[];

  @ApiProperty({
    required: false,
    type: 'array',
    items: { type: 'object', additionalProperties: true },
  })
  @Expose()
  @ValidateIf(whenPresent)
  @IsArray()
  @IsObject({ each: true })
  adopted?: Record<string, unknown>[];

  @ApiProperty({ required: false, nullable: true, type: 'string' })
  @Expose()
  @ValidateIf(whenNonNull)
  @IsString()
  pending_proposal?: string | null;

  @ApiProperty({ required: false, type: () => ProposalView })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  @ValidateNested()
  @Type(() => ProposalView)
  proposal?: ProposalView;

  @ApiProperty({ required: false, type: Object, additionalProperties: true })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  reconciliation?: Record<string, unknown>;

  @ApiProperty({ required: false, type: Object, additionalProperties: true })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  failure?: Record<string, unknown>;
}

export class CommittedEvent {
  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  event_type!: string;

  @ApiProperty({ type: Object, additionalProperties: true })
  @Expose()
  @IsObject()
  payload!: Record<string, unknown>;
}

export class PublicMessage {
  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  message_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  run_id!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  conversation_id!: number;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  sequence!: string;

  @ApiProperty({ type: 'integer', format: 'int64', minimum: 0 })
  @Expose()
  @IsInt()
  @Min(0)
  part_index!: number;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  kind!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  content!: string;
}

export class MessagePage {
  @ApiProperty({ type: () => PublicMessage, isArray: true })
  @Expose()
  @IsArray()
  @IsObject({ each: true })
  @ValidateNested({ each: true })
  @Type(() => PublicMessage)
  items!: PublicMessage[];

  @ApiProperty({ required: false, type: 'string' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsString()
  next_cursor?: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  through!: string;
}

export class RunSnapshot {
  @ApiProperty({ type: 'boolean' })
  @Expose()
  @IsBoolean()
  execution_available!: boolean;

  @ApiProperty({ required: false, type: 'boolean' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsBoolean()
  history_incomplete?: boolean;

  @ApiProperty({ type: () => MessagePage })
  @Expose()
  @IsObject()
  @ValidateNested()
  @Type(() => MessagePage)
  messages_page!: MessagePage;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  root_run_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  session_id!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  submission_id!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  goal!: string;

  @ApiProperty({ required: false, type: () => AuthorizedInput })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  @ValidateNested()
  @Type(() => AuthorizedInput)
  input?: AuthorizedInput;

  @ApiProperty({ required: false, type: 'string', format: 'uuid' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsUUID()
  retry_of_run_id?: string;

  @ApiProperty({ type: Object, additionalProperties: true })
  @Expose()
  @IsObject()
  manifest!: Record<string, unknown>;

  @ApiProperty({ type: () => AgentState })
  @Expose()
  @IsObject()
  @ValidateNested()
  @Type(() => AgentState)
  state!: AgentState;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  snapshot_cursor!: string;

  @ApiProperty({ type: () => CommittedEvent, isArray: true })
  @Expose()
  @IsArray()
  @IsObject({ each: true })
  @ValidateNested({ each: true })
  @Type(() => CommittedEvent)
  events!: CommittedEvent[];
}

export class SessionView extends ScopeDto {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  id!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  title!: string;

  @ApiProperty({ required: false, type: 'boolean' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsBoolean()
  pinned?: boolean;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  execution_revision!: number;

  @ApiProperty({ required: false, type: 'string', format: 'date-time' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsISO8601()
  created_at?: string;

  @ApiProperty({ required: false, type: () => RunSnapshot })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  @ValidateNested()
  @Type(() => RunSnapshot)
  run?: RunSnapshot;
}

export class AcceptedRun {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  root_run_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  session_id!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  submission_id!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  status!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  execution_revision!: number;
}

export class ControlReceipt {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  decision_id!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  authorization_revision!: number;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  status!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  execution_revision!: number;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  receipt_id!: string;
}

export class SessionPage {
  @ApiProperty({ type: () => SessionView, isArray: true })
  @Expose()
  @IsArray()
  @IsObject({ each: true })
  @ValidateNested({ each: true })
  @Type(() => SessionView)
  items!: SessionView[];

  @ApiProperty({ required: false, type: 'string' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsString()
  next_cursor?: string;
}

export class RunSummary {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  root_run_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  session_id!: string;

  @ApiProperty({ type: 'integer', format: 'int64' })
  @Expose()
  @IsInt()
  accepted_revision!: number;

  @ApiProperty({ type: 'string', format: 'date-time' })
  @Expose()
  @IsISO8601()
  created_at!: string;

  @ApiProperty({ type: 'string' })
  @Expose()
  @IsString()
  status!: string;

  @ApiProperty({ type: 'string', maxLength: 480 })
  @Expose()
  @IsString()
  @MaxLength(480)
  input_excerpt!: string;

  @ApiProperty({ required: false, type: 'string', format: 'uuid' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsUUID()
  retry_of_run_id?: string;
}

export class RunPage {
  @ApiProperty({ type: () => RunSummary, isArray: true })
  @Expose()
  @IsArray()
  @IsObject({ each: true })
  @ValidateNested({ each: true })
  @Type(() => RunSummary)
  items!: RunSummary[];

  @ApiProperty({ required: false, type: 'string' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsString()
  next_cursor?: string;
}

export class CurrentSubmission {
  @ApiProperty({ required: false, type: () => AcceptedRun })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  @ValidateNested()
  @Type(() => AcceptedRun)
  latest_accepted?: AcceptedRun;

  @ApiProperty({ required: false, type: () => RunSummary })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  @ValidateNested()
  @Type(() => RunSummary)
  active_run?: RunSummary;

  @ApiProperty({
    required: false,
    type: 'string',
    format: 'uuid',
    isArray: true,
  })
  @Expose()
  @ValidateIf(whenPresent)
  @IsArray()
  @IsUUID('all', { each: true })
  unresolved_run_ids?: string[];
}
