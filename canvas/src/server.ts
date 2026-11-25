import { loadConfig } from "./config.js";
import { buildApp } from "./transport/app.js";
import { ConcurrencyGate } from "./transport/concurrency.js";
import {
  startGrpcServer,
  type CanvasGrpcServer,
} from "./transport/grpc-server.js";

const config = loadConfig(process.env);
const gate = new ConcurrencyGate(config.maxConcurrent);
const app = await buildApp({
  token: config.token,
  maxConcurrent: config.maxConcurrent,
  gate,
});

let grpcServer: CanvasGrpcServer | undefined;
let closing: Promise<void> | undefined;

function close(): Promise<void> {
  closing ??= (async () => {
    const deadline = setTimeout(() => {
      process.exit(1);
    }, 15000);
    deadline.unref();
    try {
      await (grpcServer ? grpcServer.close() : Promise.resolve()).finally(() =>
        app.close(),
      );
    } finally {
      clearTimeout(deadline);
    }
  })();
  return closing;
}

function onSignal(): void {
  void close().catch(() => {
    process.exitCode = 1;
  });
}

process.on("SIGTERM", onSignal);
process.on("SIGINT", onSignal);

try {
  await app.listen(config.port, config.host);
  grpcServer = await startGrpcServer({
    address: config.grpcAddress,
    token: config.token,
    maxConcurrent: config.maxConcurrent,
    gate,
  });
} catch (error) {
  await close();
  throw error;
}
