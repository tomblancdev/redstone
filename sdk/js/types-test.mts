/**
 * Type-level test — never runs, only compiles (tsc --strict --noEmit).
 * Exercises every declared surface so index.d.mts cannot drift from usage.
 */
import { createClient, type Binding, type Client } from "./index.mjs";

const client: Client = createClient({ register: "localhost:8216", stack: "prod", app: "reporter" });

async function usage(): Promise<void> {
  const uploads: Binding = await client.bind("uploads");
  const pinned: Binding = await client.bind("archive", { name: "ipfs-local" });
  const selected: Binding = await client.bind("cache", {
    capability: "blob",
    level: "core",
    labels: { addressing: "content" },
  });
  const mail: Binding | null = await client.optional("mail");

  const [key, value]: ["X-Edge", string] = uploads.header();
  const verified: boolean = pinned.verified && selected.verified;

  void mail;
  void key;
  void value;
  void verified;
  client.close();
}

void usage;

// @ts-expect-error — stack and app are required
createClient({ register: "localhost:8216" });
