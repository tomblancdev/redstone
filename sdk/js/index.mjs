/**
 * The JS binder for redstone — deliberately micro: bind, optional, edge
 * identity. Runtime proto loading (no codegen step); if this file grows
 * features, it is failing.
 */
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import grpc from "@grpc/grpc-js";
import { loadSync } from "@grpc/proto-loader";

const DEFAULT_PROTO = join(dirname(fileURLToPath(import.meta.url)), "../../proto/redstone/core/v1/core.proto");

/**
 * createClient({ register, stack, app, proto? })
 *   .bind(task, { capability, level, name, labels })  -> Binding (throws on refusal)
 *   .optional(task, opts)                             -> Binding | null
 *   .close()
 *
 * Zero opts = declaration-driven: capability and level come from the stack
 * file, the single source of truth. Binding.header() -> ["X-Edge",
 * "stack/app/task"] — send it on capability calls so shared adapters can
 * pull this edge's with/wire config from the register.
 */
export function createClient({ register = "register:50051", stack, app, proto = process.env.REDSTONE_PROTO ?? DEFAULT_PROTO }) {
  const def = loadSync(proto, { keepCase: true, longs: Number, defaults: true });
  const core = grpc.loadPackageDefinition(def).redstone.core.v1;
  const client = new core.RegisterService(register, grpc.credentials.createInsecure());

  function bind(task, { capability = "", level = "", name = "", labels = {} } = {}) {
    return new Promise((resolve, reject) => {
      client.Bind(
        { capability, level, name, labels, stack, consumer: app, as: task },
        { deadline: new Date(Date.now() + 8000) },
        (err, resp) => {
          if (err) {
            // FAILED_PRECONDITION details carry the refusal JSON verbatim.
            return reject(new Error(`bind ${stack}/${app}.${task}: ${err.details || err.message}`));
          }
          resolve({
            ...resp,
            task,
            header: () => ["X-Edge", `${stack}/${app}/${task}`],
          });
        },
      );
    });
  }

  return {
    bind,
    optional: (task, opts) => bind(task, opts).catch(() => null),
    close: () => grpc.closeClient(client),
  };
}
