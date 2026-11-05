import {
  createConnection,
  createServer,
  type AddressInfo,
  type Socket,
} from "node:net";
import { Pool } from "pg";

// Drop precisely a PostgreSQL CommandComplete packet. The backend has executed
// this command, but the frontend can never learn its result from this connection.
export async function interruptCommand(dsn: string, command: string) {
  const database = new URL(dsn);
  const sockets = new Set<Socket>();
  let interrupted = false;
  const server = createServer((front) => {
    const back = createConnection({
      host: database.hostname,
      port: Number(database.port || 5432),
    });
    for (const socket of [front, back]) {
      sockets.add(socket);
      socket.on("error", () => {
        front.destroy();
        back.destroy();
      });
      socket.on("close", () => {
        sockets.delete(socket);
        front.destroy();
        back.destroy();
      });
    }
    front.pipe(back);
    let pending = Buffer.alloc(0);
    back.on("data", (chunk: Buffer) => {
      pending = Buffer.concat([pending, chunk]);
      while (pending.length >= 5) {
        const size = 1 + pending.readUInt32BE(1);
        if (pending.length < size) break;
        const packet = pending.subarray(0, size);
        pending = pending.subarray(size);
        if (
          !interrupted &&
          packet[0] === 67 &&
          packet.subarray(5, -1).toString() === command
        ) {
          interrupted = true;
          front.destroy();
          back.destroy();
          return;
        }
        front.write(packet);
      }
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const proxyUrl = new URL(database);
  proxyUrl.hostname = "127.0.0.1";
  proxyUrl.port = String((server.address() as AddressInfo).port);
  proxyUrl.searchParams.set("sslmode", "disable");
  const pool = new Pool({ connectionString: proxyUrl.href });
  return {
    pool,
    interrupted: () => interrupted,
    async close() {
      await pool.end();
      for (const socket of sockets) socket.destroy();
      await new Promise<void>((resolve, reject) =>
        server.close((err) => (err ? reject(err) : resolve())),
      );
    },
  };
}
