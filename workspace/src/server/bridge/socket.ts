import { lstat, rm } from "node:fs/promises";
import { createConnection } from "node:net";
export async function prepareSocket(path: string) {
  let before;
  try {
    before = await lstat(path);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return;
    throw error;
  }
  if (!before.isSocket() || before.uid !== process.getuid?.())
    throw new Error("unsafe_socket_path");
  const refused = await new Promise<boolean>((yes, no) => {
    const socket = createConnection(path);
    socket.setTimeout(1000);
    socket.once("connect", () => {
      socket.destroy();
      yes(false);
    });
    socket.once("timeout", () => {
      socket.destroy();
      no(new Error("active_socket_unknown"));
    });
    socket.once("error", (error: NodeJS.ErrnoException) => {
      socket.destroy();
      if (["ECONNREFUSED", "ENOENT"].includes(error.code ?? "")) yes(true);
      else no(error);
    });
  });
  if (!refused) throw new Error("active_socket");
  const current = await lstat(path).catch((error: NodeJS.ErrnoException) => {
    if (error.code === "ENOENT") return null;
    throw error;
  });
  if (current) {
    if (current.ino !== before.ino || current.dev !== before.dev)
      throw new Error("active_socket_changed");
    await rm(path);
  }
}
