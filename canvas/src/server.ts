import { buildApp } from "./http/app.js";

const app = await buildApp({
  token: process.env.GLOWLOOM_COLLAB_RUNTIME_TOKEN,
});
app.enableShutdownHooks(["SIGTERM", "SIGINT"]);
await app.listen(
  Number(process.env.PORT ?? 4400),
  process.env.HOST ?? "127.0.0.1",
);
