/**
 * Types for the JS/TS binder. One package serves both languages — TS gets
 * these declarations, JS gets the runtime; the API is identical.
 * types-test.mts compiles against this file in CI, so it cannot drift.
 */

export interface CreateClientOptions {
  /** gRPC address of the register. Default: "register:50051". */
  register?: string;
  /** The stack this app runs in (wiring is resolved per stack). */
  stack: string;
  /** The app name — with stack and task, forms the edge identity. */
  app: string;
  /** Override the core.proto path (default: bundled monorepo path, or REDSTONE_PROTO). */
  proto?: string;
}

export interface BindOpts {
  /** Explicit capability — omit for declaration-driven binds. */
  capability?: string;
  /** Explicit level — must agree with the declaration if one exists. */
  level?: string;
  /** Pin a specific give by name (deliberate coupling, still verified). */
  name?: string;
  /** Label selectors. */
  labels?: Record<string, string>;
}

export interface Binding {
  name: string;
  capability: string;
  requested_level: string;
  effective_level: string;
  verified: boolean;
  endpoint: string;
  public: string;
  implementation: string;
  /** Capability flags (google.protobuf.Struct shape — treat as opaque). */
  flags: Record<string, unknown>;
  task: string;
  /** Edge identity for capability calls: ["X-Edge", "stack/app/task"]. */
  header(): ["X-Edge", string];
}

export interface Client {
  /** Bind a task. Rejects with the register's written refusal reasons. */
  bind(task: string, opts?: BindOpts): Promise<Binding>;
  /** Optional bind: unresolved resolves to null — run with the feature off. */
  optional(task: string, opts?: BindOpts): Promise<Binding | null>;
  close(): void;
}

export function createClient(options: CreateClientOptions): Client;
