import { DocumentBuilder, SwaggerModule } from '@nestjs/swagger';
import type { INestApplication } from '@nestjs/common';
import { closedDtos } from './validation.js';

export function createOpenApi(app: INestApplication) {
  const config = new DocumentBuilder()
    .setTitle('Glowloom agent internal API')
    .setVersion('1')
    .setOpenAPIVersion('3.0.3')
    .addBearerAuth(undefined, 'serviceIdentity')
    .build();
  const document = SwaggerModule.createDocument(app, config);
  for (const [name, schema] of Object.entries(
    document.components?.schemas ?? {},
  )) {
    if ('$ref' in schema) continue;
    if (closedDtos.has(name)) schema.additionalProperties = false;
    const required = new Set(schema.required);
    for (const [field, property] of Object.entries(schema.properties ?? {})) {
      // The Go public Swagger derives its field contract from these tags.
      const tags: Record<string, string> = {};
      if (required.has(field)) tags.binding = 'required';
      if (!('$ref' in property) && property.nullable) {
        tags.extensions = 'x-nullable';
      }
      if (Object.keys(tags).length !== 0) {
        Object.assign(property, { 'x-oapi-codegen-extra-tags': tags });
      }
    }
  }
  retainWireStrings(document);
  return document;
}

// Keep the documented wire formats while generating string-valued Go DTOs.
function retainWireStrings(value: unknown) {
  if (value === null || typeof value !== 'object') return;
  if (
    'format' in value &&
    (value.format === 'uuid' || value.format === 'date-time')
  ) {
    Object.assign(value, { 'x-go-type': 'string' });
  }
  for (const child of Object.values(value)) retainWireStrings(child);
}
