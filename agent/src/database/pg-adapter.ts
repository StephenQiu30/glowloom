import { AsyncLocalStorage } from 'node:async_hooks';
import { PrismaPg } from '@prisma/adapter-pg';
import type { Pool, PoolClient } from 'pg';

type Adapter = Awaited<ReturnType<PrismaPg['connect']>>;
type Transaction = Awaited<ReturnType<Adapter['startTransaction']>>;
interface Lease {
  broken: boolean;
}

function discardOnFailure<Args extends unknown[], Result>(
  lease: Lease,
  work: (...args: Args) => Promise<Result>,
): (...args: Args) => Promise<Result> {
  return async (...args) => {
    try {
      return await work(...args);
    } catch (error) {
      lease.broken = true;
      throw error;
    }
  };
}

function trackClient(client: PoolClient, lease: Lease): PoolClient {
  const onError = () => {
    lease.broken = true;
  };
  client.on('error', onError);
  return new Proxy(client, {
    get(target, property) {
      if (property === 'release') {
        return (error?: Error | boolean) => {
          target.removeListener('error', onError);
          target.release(error || lease.broken);
        };
      }
      const value: unknown = Reflect.get(target, property, target);
      return typeof value === 'function' ? value.bind(target) : value;
    },
  });
}

function trackPool(pool: Pool, leases: AsyncLocalStorage<Lease>): Pool {
  // Proxy retains Pool's prototype/instanceof identity and the caller owns the
  // original pool. The official adapter still acquires exactly one client.
  return new Proxy(pool, {
    get(target, property) {
      if (property === 'connect') {
        return (...args: unknown[]) => {
          // PrismaPg uses the promise overload. Preserve the callback overload
          // for other public adapter consumers without changing its contract.
          if (args.length) return Reflect.apply(target.connect, target, args);
          const lease = leases.getStore();
          return target
            .connect()
            .then((client) => (lease ? trackClient(client, lease) : client));
        };
      }
      const value: unknown = Reflect.get(target, property, target);
      return typeof value === 'function' ? value.bind(target) : value;
    },
  });
}

function trackTransaction(transaction: Transaction, lease: Lease): Transaction {
  transaction.queryRaw = discardOnFailure(
    lease,
    transaction.queryRaw.bind(transaction),
  );
  transaction.executeRaw = discardOnFailure(
    lease,
    transaction.executeRaw.bind(transaction),
  );
  transaction.commit = discardOnFailure(
    lease,
    transaction.commit.bind(transaction),
  );
  transaction.rollback = discardOnFailure(
    lease,
    transaction.rollback.bind(transaction),
  );
  if (transaction.createSavepoint)
    transaction.createSavepoint = discardOnFailure(
      lease,
      transaction.createSavepoint.bind(transaction),
    );
  if (transaction.rollbackToSavepoint)
    transaction.rollbackToSavepoint = discardOnFailure(
      lease,
      transaction.rollbackToSavepoint.bind(transaction),
    );
  if (transaction.releaseSavepoint)
    transaction.releaseSavepoint = discardOnFailure(
      lease,
      transaction.releaseSavepoint.bind(transaction),
    );
  return transaction;
}

/** Keep failed transaction connections out of the caller's reusable pg pool. */
export class AgentPrismaPg extends PrismaPg {
  constructor(pool: Pool) {
    const leases = new AsyncLocalStorage<Lease>();
    super(trackPool(pool, leases), { disposeExternalPool: false });
    this.leases = leases;
  }
  private readonly leases: AsyncLocalStorage<Lease>;

  override async connect(): Promise<Adapter> {
    const adapter = await super.connect();
    const startTransaction = adapter.startTransaction.bind(adapter);
    adapter.startTransaction = async (isolationLevel) => {
      const lease: Lease = { broken: false };
      return this.leases.run(lease, async () =>
        trackTransaction(await startTransaction(isolationLevel), lease),
      );
    };
    return adapter;
  }
}
