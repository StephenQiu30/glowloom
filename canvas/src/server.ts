import { buildApp } from "./http/app.js";
import { startGrpcServer } from "./rpc/server.js";

const token = process.env.GLOWLOOM_COLLAB_RUNTIME_TOKEN;
const app = await buildApp({ token });
app.enableShutdownHooks(["SIGTERM", "SIGINT"]);

const grpcAddr = process.env.GLOWLOOM_CANVAS_GRPC_ADDR ?? "0.0.0.0:4402";
const grpcServer = await startGrpcServer({ address: grpcAddr, token });

const shutdown = async () => {
  await grpcServer.close();
};
process.once("SIGTERM", () => {
  void shutdown();
});
process.once("SIGINT", () => {
  void shutdown();
});

await app.listen(
  Number(process.env.PORT ?? 4400),
  process.env.HOST ?? "127.0.0.1",
);
