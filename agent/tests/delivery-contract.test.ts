import { expect, it } from 'vitest';
import { deliveryOutcome } from '../src/sessions/delivery.js';
import { canonicalHash, freezeManifest } from '../src/sessions/manifest.js';

const production = { kind: 'production', required_deliveries: 1 } as const;
const productionOptions = {
  requireDeliveryFacts: true,
  goalContract: production,
};
const goal = (status: string) => ({
  contract_hash: canonicalHash(production),
  kind: 'production',
  status,
  required_deliveries: 1,
  completed_deliveries: status === 'completed' ? 1 : 0,
  missing: status === 'completed' ? [] : ['required_delivery_missing'],
});

it('pins the accepted business goal and never treats a production empty set as chat', () => {
  const manifest = freezeManifest(
    { model_key: 'model', goal_contract: production },
    [],
  );
  expect(manifest.goal_contract).toEqual(production);
  expect(
    deliveryOutcome(
      { shot_deliveries: [], goal_completion: goal('incomplete') },
      null,
      productionOptions,
    ).status,
  ).toBe('needs_reconciliation');
  expect(
    deliveryOutcome({ shot_deliveries: [] }, null, productionOptions).status,
  ).not.toBe('completed');
});

it('waits for original production operations and requests one deterministic continuation when ready', () => {
  const operationID = '11111111-1111-4111-8111-111111111111';
  const snapshot = {
    shot_deliveries: [],
    goal_completion: goal('incomplete'),
    production_operations: [{ operation_id: operationID, status: 'submitted' }],
  };
  const waiting = deliveryOutcome(snapshot, null, productionOptions, 1000);
  expect(waiting.status).toBe('waiting_delivery');
  expect(waiting.wait?.operation_ids).toEqual([operationID]);
  const ready = deliveryOutcome(
    {
      ...snapshot,
      production_operations: [
        { operation_id: operationID, status: 'completed' },
      ],
    },
    waiting.wait,
    productionOptions,
    2000,
  );
  expect(ready.continueOperations).toEqual([operationID]);
  expect(ready.wait?.deadline_at).toBe(waiting.wait?.deadline_at);
  const unknown = deliveryOutcome(
    {
      ...snapshot,
      production_operations: [{ operation_id: operationID, status: 'unknown' }],
    },
    waiting.wait,
    productionOptions,
    2000,
  );
  expect(unknown.status).toBe('needs_reconciliation');
  expect(unknown.continueOperations).toBeUndefined();
});

it('requires matching authoritative goal facts even when individual files succeeded', () => {
  const complete = {
    shot_deliveries: [
      {
        delivery_id: '11111111-1111-4111-8111-111111111111',
        status: 'completed',
      },
    ],
    goal_completion: goal('completed'),
  };
  expect(deliveryOutcome(complete, null, productionOptions).status).toBe(
    'completed',
  );
  expect(
    deliveryOutcome(
      {
        ...complete,
        goal_completion: {
          ...goal('completed'),
          contract_hash: '0'.repeat(64),
        },
      },
      null,
      productionOptions,
    ).status,
  ).not.toBe('completed');
});

it('distinguishes an authoritative empty chat delivery set from missing production facts', () => {
  expect(
    deliveryOutcome({ shot_deliveries: [] }, null, {
      requireDeliveryFacts: true,
    }).status,
  ).toBe('completed');
  const missing = deliveryOutcome({ revision: 1 }, null, {
    requireDeliveryFacts: true,
  });
  expect(missing.status).toBe('waiting_delivery');
  expect(missing.wait?.reason).toBe('delivery_facts_unavailable');
});

it('never completes an approved but unadmitted production obligation', () => {
  const result = deliveryOutcome(
    {
      shot_deliveries: [
        {
          delivery_id: '11111111-1111-4111-8111-111111111111',
          status: 'incomplete',
        },
      ],
    },
    null,
    { requireDeliveryFacts: true },
  );
  expect(result.status).toBe('needs_reconciliation');
  expect(result.wait?.delivery_ids).toEqual([
    '11111111-1111-4111-8111-111111111111',
  ]);
});

it('parks malformed or missing original facts at the same absolute deadline', () => {
  const previous = {
    first_wait_at: new Date(0).toISOString(),
    deadline_at: new Date(100).toISOString(),
    delivery_ids: ['11111111-1111-4111-8111-111111111111'],
  };
  const result = deliveryOutcome(
    { shot_deliveries: [] },
    previous,
    { requireDeliveryFacts: true },
    101,
  );
  expect(result.status).toBe('needs_reconciliation');
  expect(result.wait?.deadline_at).toBe(previous.deadline_at);
  expect(result.wait?.delivery_ids).toEqual(previous.delivery_ids);
});

it('refuses malformed, duplicated or missing original operation facts before continuing', () => {
  const id = '11111111-1111-4111-8111-111111111111';
  const prior = { operation_ids: [id] };
  const base = { shot_deliveries: [], goal_completion: goal('incomplete') };
  for (const operations of [
    [{ operation_id: '-'.repeat(36), status: 'completed' }],
    [{ operation_id: id, status: 'invented_pending' }],
    [
      { operation_id: id, status: 'completed' },
      { operation_id: id, status: 'completed' },
    ],
    [
      {
        operation_id: '22222222-2222-4222-8222-222222222222',
        status: 'completed',
      },
    ],
    [],
  ]) {
    const result = deliveryOutcome(
      { ...base, production_operations: operations },
      prior,
      productionOptions,
      1000,
    );
    expect(result.continueOperations).toBeUndefined();
    expect(result.status).not.toBe('completed');
    expect(result.wait?.operation_ids).toContain(id);
    expect(result.wait?.reason).toBe('production_facts_unavailable');
  }
});

it('honors the owning Go goal verdict despite an optional unsuccessful delivery', () => {
  const result = deliveryOutcome(
    {
      goal_completion: goal('completed'),
      shot_deliveries: [
        {
          delivery_id: '11111111-1111-4111-8111-111111111111',
          status: 'completed',
        },
        {
          delivery_id: '22222222-2222-4222-8222-222222222222',
          status: 'incomplete',
        },
      ],
    },
    null,
    productionOptions,
  );
  expect(result.status).toBe('completed');
});

it('continues the next ready operation after a previous required shot was delivered', () => {
  const contract = { kind: 'production', required_deliveries: 2 } as const;
  const result = deliveryOutcome(
    {
      goal_completion: {
        ...goal('incomplete'),
        required_deliveries: 2,
        completed_deliveries: 1,
        contract_hash: canonicalHash(contract),
      },
      shot_deliveries: [
        {
          delivery_id: '11111111-1111-4111-8111-111111111111',
          status: 'completed',
        },
      ],
      production_operations: [
        {
          operation_id: '22222222-2222-4222-8222-222222222222',
          status: 'completed',
        },
      ],
    },
    null,
    { requireDeliveryFacts: true, goalContract: contract },
  );
  expect(result.continueOperations).toEqual([
    '22222222-2222-4222-8222-222222222222',
  ]);
});

it('continues a second output of the same completed operation using formal delivery progress', () => {
  const contract = { kind: 'production', required_deliveries: 2 } as const;
  const operation = '11111111-1111-4111-8111-111111111111',
    delivery = '22222222-2222-4222-8222-222222222222';
  const result = deliveryOutcome(
    {
      goal_completion: {
        ...goal('incomplete'),
        required_deliveries: 2,
        completed_deliveries: 1,
        contract_hash: canonicalHash(contract),
      },
      shot_deliveries: [
        {
          delivery_id: delivery,
          status: 'completed',
          operation_id: operation,
          output_id: '33333333-3333-4333-8333-333333333333',
          video_export_id: '44444444-4444-4444-8444-444444444444',
          project_export_id: '55555555-5555-4555-8555-555555555555',
        },
      ],
      production_operations: [{ operation_id: operation, status: 'completed' }],
    },
    null,
    { requireDeliveryFacts: true, goalContract: contract },
  );
  expect(result.continueOperations).toEqual([operation]);
  expect(result.continueDeliveries).toEqual([delivery]);
});

it('waits without a model wake when all required deliveries only need original review', () => {
  const id = '11111111-1111-4111-8111-111111111111';
  const result = deliveryOutcome(
    {
      goal_completion: {
        ...goal('awaiting_review'),
        missing: [`delivery:${id}:awaiting_review`],
      },
      shot_deliveries: [
        {
          delivery_id: id,
          operation_id: id,
          status: 'awaiting_review',
          video_export_id: id,
          project_export_id: id,
        },
      ],
      production_operations: [{ operation_id: id, status: 'completed' }],
    },
    null,
    productionOptions,
  );
  expect(result.status).toBe('waiting_delivery');
  expect(result.continueOperations).toBeUndefined();
});

it('keeps original operation identities and deadline while the business snapshot is temporarily unavailable', () => {
  const id = '11111111-1111-4111-8111-111111111111';
  const previous = {
    operation_ids: [id],
    delivery_ids: [],
    first_wait_at: new Date(1000).toISOString(),
    deadline_at: new Date(10000).toISOString(),
  };
  const result = deliveryOutcome(undefined, previous, productionOptions, 2000);
  expect(result.status).toBe('waiting_delivery');
  expect(result.wait?.operation_ids).toEqual([id]);
  expect(result.wait?.deadline_at).toBe(previous.deadline_at);
  expect(result.continueOperations).toBeUndefined();
});
