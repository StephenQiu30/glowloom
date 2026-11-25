import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import * as grpc from "@grpc/grpc-js";
import * as protoLoader from "@grpc/proto-loader";
import { startGrpcServer } from "../dist/transport/grpc-server.js";

const token = "canvas-grpc-test-token-at-least-32-bytes-long";

test("Canvas gRPC runtime enforces Bearer auth and serves Health + Initialize", async () => {
  const srv = await startGrpcServer({ address: "127.0.0.1:0", token });
  try {
    const protoPath = path.resolve(
      path.dirname(fileURLToPath(import.meta.url)),
      "../proto/runtime.proto",
    );
    const def = protoLoader.loadSync(protoPath, {
      keepCase: true,
      longs: String,
      enums: String,
      defaults: true,
      oneofs: true,
    });
    const loaded = grpc.loadPackageDefinition(def);
    const client = new loaded.glowloom.canvas.CanvasRuntime(
      srv.address,
      grpc.credentials.createInsecure(),
    );

    const health = await new Promise((resolve, reject) => {
      client.CheckHealth({}, (err, resp) =>
        err ? reject(err) : resolve(resp),
      );
    });
    assert.equal(health.status, "ok");

    await assert.rejects(
      () =>
        new Promise((resolve, reject) => {
          client.Initialize({ payload_json: Buffer.from("{}") }, (err, resp) =>
            err ? reject(err) : resolve(resp),
          );
        }),
      (err) => err && err.code === grpc.status.UNAUTHENTICATED,
    );

    const meta = new grpc.Metadata();
    meta.add("authorization", `Bearer ${token}`);
    const badJson = await new Promise((resolve, reject) => {
      client.Initialize(
        { payload_json: Buffer.from("not-json") },
        meta,
        (err, resp) => (err ? reject(err) : resolve(resp)),
      );
    }).catch((err) => err);
    assert.equal(badJson.code, grpc.status.INVALID_ARGUMENT);

    client.close();
  } finally {
    await srv.close();
  }
});
