import { BadRequestException, ValidationPipe } from '@nestjs/common';
import { plainToInstance } from 'class-transformer';
import { registerDecorator, validateSync } from 'class-validator';
import { AgentError } from '../sessions/errors.js';

export class InvalidRequest extends BadRequestException {}

export const closedDtos = new Set<string>();

/** Rejects extra fields only on DTOs whose object boundary is closed. */
export function ClosedDto() {
  return <T extends new () => object>(dto: T) => {
    closedDtos.add(dto.name);
    const fields = new Set(Object.keys(new dto()));
    registerDecorator({
      name: 'closedDto',
      target: dto,
      propertyName: '',
      validator: {
        validate(_value: unknown, args) {
          return (
            args !== undefined &&
            Object.keys(args.object).every((key) => fields.has(key))
          );
        },
      },
    });
  };
}

export function requestValidation() {
  return new ValidationPipe({
    transform: true,
    exceptionFactory: () => new InvalidRequest(),
  });
}

export function checkedResponse<T extends object>(
  dto: new () => T,
  value: unknown,
): T {
  const json: unknown = JSON.parse(JSON.stringify(value));
  if (json === null || typeof json !== 'object' || Array.isArray(json)) {
    throw new AgentError('invalid_service_response');
  }
  const response = plainToInstance(dto, json, {
    excludeExtraneousValues: true,
  });
  if (validateSync(response).length !== 0) {
    throw new AgentError('invalid_service_response');
  }
  return response;
}

export function queryInteger(value: unknown): unknown {
  return typeof value === 'string' && value.trim() !== ''
    ? Number(value)
    : value;
}

export function whenPresent(_target: unknown, value: unknown): boolean {
  return value !== undefined;
}

export function whenNonNull(_target: unknown, value: unknown): boolean {
  return value !== undefined && value !== null;
}
