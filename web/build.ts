import { existsSync, readdirSync, rmSync } from "node:fs";
import { join } from "node:path";

const outdir = "../internal/daemon/webdist";

// Bun's HTML build hashes filenames per build, so a stale previous build's
// assets (different hash, same purpose) would otherwise linger in outdir and
// get committed alongside the new ones. Clear file-by-file rather than
// rmSync(outdir, { recursive: true }): recursive directory removal is flaky
// on Windows (EINVAL) in current Bun.
if (existsSync(outdir)) {
  for (const name of readdirSync(outdir)) {
    rmSync(join(outdir, name), { force: true });
  }
}

const result = await Bun.build({
  entrypoints: ["./index.html"],
  outdir,
  minify: true,
  sourcemap: "none",
});

for (const artifact of result.outputs) {
  console.log(`  ${artifact.path}`);
}
if (!result.success) {
  for (const log of result.logs) console.error(log);
  process.exit(1);
}
