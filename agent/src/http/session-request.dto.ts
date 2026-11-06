import { ApiProperty } from '@nestjs/swagger';
import {
  Expose,
  Type,
  Transform,
  type TransformFnParams,
} from 'class-transformer';
import {
  ArrayMaxSize,
  Equals,
  IsArray,
  IsBoolean,
  IsIn,
  IsInt,
  IsObject,
  IsString,
  IsUUID,
  Matches,
  Max,
  MaxLength,
  Min,
  MinLength,
  ValidateIf,
  ValidateNested,
} from 'class-validator';
import { ClosedDto, queryInteger, whenPresent } from './validation.js';

export class ScopeDto {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  actor_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  org_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  scope_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  project_id!: string;

  @ApiProperty({ required: false, type: 'string', format: 'uuid' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsUUID()
  canvas_id?: string;
}

export class IdentityQuery {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  actor_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  project_id!: string;
}

export class SessionParams {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  sid!: string;
}

export class RunParams {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  rid!: string;
}

export class HistoryQuery extends IdentityQuery {
  @ApiProperty({
    required: false,
    type: 'integer',
    format: 'int64',
    minimum: 1,
    maximum: 50,
    default: 20,
  })
  @ValidateIf(whenPresent)
  @Transform(({ value }: TransformFnParams) => queryInteger(value))
  @IsInt()
  @Min(1)
  @Max(50)
  limit?: number;

  @ApiProperty({ required: false, type: 'string', maxLength: 2048 })
  @ValidateIf(whenPresent)
  @IsString()
  @MaxLength(2048)
  cursor?: string;
}

export class MessageQuery extends IdentityQuery {
  @ApiProperty({
    required: false,
    type: 'integer',
    format: 'int64',
    minimum: 1,
    maximum: 100,
    default: 50,
  })
  @ValidateIf(whenPresent)
  @Transform(({ value }: TransformFnParams) => queryInteger(value))
  @IsInt()
  @Min(1)
  @Max(100)
  limit?: number;

  @ApiProperty({ required: false, type: 'string', maxLength: 2048 })
  @ValidateIf(whenPresent)
  @IsString()
  @MaxLength(2048)
  before?: string;

  @ApiProperty({ type: 'string', maxLength: 64, pattern: '^[0-9]+:[0-9]+$' })
  @IsString()
  @MaxLength(64)
  @Matches(/^[0-9]+:[0-9]+$/)
  through!: string;
}

export class SubmissionParams extends SessionParams {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  requestId!: string;
}

export class MessageParams extends RunParams {
  @ApiProperty({ type: 'string', minLength: 1, maxLength: 256 })
  @IsString()
  @MinLength(1)
  @MaxLength(256)
  messageId!: string;
}

export class MediaPin {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  sourceId!: string;

  @ApiProperty({ type: 'string', minLength: 1, maxLength: 256 })
  @Expose()
  @IsString()
  @MinLength(1)
  @MaxLength(256)
  sourceVersion!: string;

  @ApiProperty({ enum: ['image', 'video', 'audio'], type: 'string' })
  @Expose()
  @IsIn(['image', 'video', 'audio'])
  mediaKind!: 'image' | 'video' | 'audio';

  @ApiProperty({ required: false, type: 'string' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsString()
  bindingId?: string;
}

export class QuotedMessage {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @Expose()
  @IsUUID()
  run_id!: string;

  @ApiProperty({ type: 'string', minLength: 1, maxLength: 256 })
  @Expose()
  @IsString()
  @MinLength(1)
  @MaxLength(256)
  message_id!: string;

  @ApiProperty({ type: 'string', maxLength: 65536 })
  @Expose()
  @IsString()
  @MaxLength(65536)
  text!: string;

  @ApiProperty({ type: 'string', pattern: '^[a-f0-9]{64}$' })
  @Expose()
  @IsString()
  @Matches(/^[a-f0-9]{64}$/)
  content_hash!: string;
}

@ClosedDto()
export class AuthorizedInput {
  @ApiProperty({ type: 'integer', format: 'int64', enum: [1] })
  @Expose()
  @IsInt()
  @Equals(1)
  schema_version!: 1;

  @ApiProperty({ required: false, type: 'boolean' })
  @Expose()
  @ValidateIf(whenPresent)
  @IsBoolean()
  references_unavailable?: boolean;

  @ApiProperty({ type: Object, additionalProperties: true })
  @Expose()
  @IsObject()
  draft!: Record<string, unknown>;

  @ApiProperty({ type: 'string', pattern: '^[a-f0-9]{64}$' })
  @Expose()
  @IsString()
  @Matches(/^[a-f0-9]{64}$/)
  input_hash!: string;

  @ApiProperty({ type: 'string', maxLength: 1048576 })
  @Expose()
  @IsString()
  @MaxLength(1048576)
  plain_text!: string;

  @ApiProperty({ type: 'string', maxLength: 1048576 })
  @Expose()
  @IsString()
  @MaxLength(1048576)
  prompt!: string;

  @ApiProperty({ type: () => MediaPin, isArray: true, maxItems: 4 })
  @Expose()
  @IsArray()
  @ArrayMaxSize(4)
  @IsObject({ each: true })
  @ValidateNested({ each: true })
  @Type(() => MediaPin)
  media_pins!: MediaPin[];

  @ApiProperty({ required: false, type: () => QuotedMessage })
  @Expose()
  @ValidateIf(whenPresent)
  @IsObject()
  @ValidateNested()
  @Type(() => QuotedMessage)
  quote?: QuotedMessage;
}

@ClosedDto()
export class CreateSessionRequest extends ScopeDto {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  request_id!: string;

  @ApiProperty({ type: 'string', pattern: '^[a-f0-9]{64}$' })
  @IsString()
  @Matches(/^[a-f0-9]{64}$/)
  content_hash!: string;

  @ApiProperty({ type: 'string', minLength: 1, maxLength: 240 })
  @IsString()
  @MinLength(1)
  @MaxLength(240)
  title!: string;
}

@ClosedDto()
export class UpdateSessionRequest extends ScopeDto {
  @ApiProperty({ enum: ['pin', 'unpin', 'rename', 'delete'], type: 'string' })
  @IsIn(['pin', 'unpin', 'rename', 'delete'])
  action!: 'pin' | 'unpin' | 'rename' | 'delete';

  @ApiProperty({
    required: false,
    type: 'string',
    minLength: 1,
    maxLength: 240,
  })
  @ValidateIf(whenPresent)
  @IsString()
  @MinLength(1)
  @MaxLength(240)
  title?: string;
}

@ClosedDto()
export class CreateRunRequest extends ScopeDto {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  request_id!: string;

  @ApiProperty({ type: 'string', pattern: '^[a-f0-9]{64}$' })
  @IsString()
  @Matches(/^[a-f0-9]{64}$/)
  content_hash!: string;

  @ApiProperty({
    type: 'integer',
    format: 'int64',
    minimum: 0,
    maximum: Number.MAX_SAFE_INTEGER,
  })
  @IsInt()
  @Min(0)
  @Max(Number.MAX_SAFE_INTEGER)
  expected_execution_revision!: number;

  @ApiProperty({ type: 'string', minLength: 1, maxLength: 1048576 })
  @IsString()
  @MinLength(1)
  @MaxLength(1048576)
  goal!: string;

  @ApiProperty({ type: () => AuthorizedInput })
  @IsObject()
  @ValidateNested()
  @Type(() => AuthorizedInput)
  input!: AuthorizedInput;

  @ApiProperty({ required: false, type: 'string', format: 'uuid' })
  @ValidateIf(whenPresent)
  @IsUUID()
  retry_of_run_id?: string;

  @ApiProperty({ type: Object, additionalProperties: true })
  @IsObject()
  manifest!: Record<string, unknown>;
}

@ClosedDto()
export class ControlRequest extends ScopeDto {
  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  decision_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  root_run_id!: string;

  @ApiProperty({ type: 'string', format: 'uuid' })
  @IsUUID()
  session_id!: string;

  @ApiProperty({
    type: 'integer',
    format: 'int64',
    minimum: 1,
    maximum: Number.MAX_SAFE_INTEGER,
  })
  @IsInt()
  @Min(1)
  @Max(Number.MAX_SAFE_INTEGER)
  authorization_revision!: number;

  @ApiProperty({
    enum: ['approve', 'reject', 'pause', 'resume', 'stop'],
    type: 'string',
  })
  @IsIn(['approve', 'reject', 'pause', 'resume', 'stop'])
  action!: 'approve' | 'reject' | 'pause' | 'resume' | 'stop';

  @ApiProperty({ type: 'string', pattern: '^[a-f0-9]{64}$' })
  @IsString()
  @Matches(/^[a-f0-9]{64}$/)
  content_hash!: string;
}
