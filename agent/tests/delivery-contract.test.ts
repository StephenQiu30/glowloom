import { expect, it } from 'vitest';
import { deliveryOutcome } from '../src/sessions/delivery.js';

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
