// Replaces ../internal/webui/dist with the fresh static export in ./out.
// The Go binary embeds that folder (ADR 0014), so it is committed and CI
// checks that a rebuild leaves it unchanged. It is generated: never edit it by
// hand (CONTRIBUTING.md).
import { cpSync, existsSync, readdirSync, rmSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const out = join(web, "out");
const dist = join(web, "..", "internal", "webui", "dist");

if (!existsSync(join(out, "index.html"))) {
  console.error("copy-export: out/index.html is missing; run next build first");
  process.exit(1);
}

rmSync(dist, { recursive: true, force: true });
cpSync(out, dist, { recursive: true });

let files = 0;
let bytes = 0;
const walk = (d) => {
  for (const name of readdirSync(d)) {
    const p = join(d, name);
    const st = statSync(p);
    if (st.isDirectory()) walk(p);
    else {
      files++;
      bytes += st.size;
    }
  }
};
walk(dist);
console.log(`copy-export: ${files} files, ${(bytes / 1024).toFixed(0)} KB -> ${relative(process.cwd(), dist)}`);
