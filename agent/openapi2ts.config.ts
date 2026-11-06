import path from 'node:path';
import type { GenerateServiceProps } from '@umijs/openapi';

const config: GenerateServiceProps = {
  schemaPath: path.resolve(
    process.env.BUSINESS_API_SCHEMA ??
      'node_modules/.cache/openapi/go-business.swagger.json',
  ),
  serversPath: path.resolve(
    process.env.BUSINESS_API_OUTPUT ?? 'src/business/generated',
  ),
  projectName: '',
  namespace: 'AgentBusinessApi',
  requestLibPath:
    "import { request } from '../request.js'; import type { RequestOptions } from '../request.js';",
  requestOptionsType: 'RequestOptions',
  hook: {
    customFunctionName(operation) {
      if (!operation.operationId) throw new Error('Missing operationId');
      return operation.operationId;
    },
    customFileNames: () => ['business'],
  },
};
export default config;
