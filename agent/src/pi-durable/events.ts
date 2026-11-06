import {
  ROOT_CONVERSATION_ID,
  type StorageWrite,
  type JsonObject,
} from '@earendil-works/pi-durable';
export function publicState(value: JsonObject): JsonObject {
  const allowed = [
    'root_run_id',
    'segment_id',
    'status',
    'execution_revision',
    'authorization_revision',
    'model_steps',
    'tool_steps',
    'plan',
    'adopted',
    'pending_proposal',
    'proposal',
    'failure',
    'delegations',
    'reconciliation',
  ];
  return Object.fromEntries(
    Object.entries(value).filter(([key]) => allowed.includes(key)),
  );
}
export function eventProjection(runId: string, knownDocuments: number[] = []) {
  const appDocs = new Set(knownDocuments);
  return (writes: readonly StorageWrite[]) => {
    const events: { type: string; payload: JsonObject }[] = [];
    for (const write of writes) {
      if (
        write.type === 'document.create' &&
        write.record.kind === 'glowloom.agent'
      )
        appDocs.add(write.record.id);
      if (
        ((write.type === 'document.create' &&
          write.record.kind === 'glowloom.agent') ||
          (write.type === 'document.change' && appDocs.has(write.id))) &&
        write.content.kind === 'base'
      ) {
        events.push({
          type: 'agent.state',
          payload: publicState(write.content.value),
        });
      }
      if (
        write.type === 'entry' &&
        write.value.kind === 'glowloom.reconciled_receipt' &&
        write.value.conversationId === ROOT_CONVERSATION_ID &&
        write.value.data &&
        typeof write.value.data === 'object' &&
        !Array.isArray(write.value.data)
      ) {
        const data = write.value.data as JsonObject;
        const result = data.provider_result;
        if (
          result &&
          typeof result === 'object' &&
          !Array.isArray(result) &&
          typeof result.output === 'string'
        )
          events.push({
            type: 'text',
            payload: {
              root_run_id: runId,
              conversation_id: write.value.conversationId,
              text: result.output,
              receipt_id: data.receipt_id ?? null,
              source: 'reconciled_original_receipt',
            },
          });
      }
      if (write.type === 'entry') {
        for (const message of write.value.model ?? []) {
          if (
            message.role === 'toolResult' &&
            !message.isError &&
            message.details &&
            typeof message.details === 'object' &&
            !Array.isArray(message.details)
          ) {
            const result = (message.details as JsonObject).result;
            if (
              result &&
              typeof result === 'object' &&
              !Array.isArray(result)
            ) {
              const id = result.operation_id;
              if (
                typeof id === 'string' &&
                /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
                  id,
                )
              )
                events.push({
                  type: 'operation',
                  payload: { root_run_id: runId, operation_id: id },
                });
            }
          }
          if (
            message.role === 'assistant' &&
            write.value.conversationId === ROOT_CONVERSATION_ID
          )
            for (const item of message.content) {
              if (item.type === 'text')
                events.push({
                  type: 'text',
                  payload: {
                    root_run_id: runId,
                    conversation_id: write.value.conversationId,
                    text: item.text,
                  },
                });
              else if (item.type === 'toolCall')
                events.push({
                  type: 'tool_call',
                  payload: {
                    root_run_id: runId,
                    conversation_id: write.value.conversationId,
                    call_id: item.id,
                    name: item.name,
                  },
                });
            }
        }
      }
    }
    return events.length
      ? events
      : [{ type: 'runtime.committed', payload: { root_run_id: runId } }];
  };
}
