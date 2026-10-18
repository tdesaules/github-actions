// Build-stage dependency closure from the digest-pinned upstream installation.
import { cpSync, existsSync, mkdirSync, readFileSync } from "node:fs";
import { basename, dirname, join } from "node:path";

const installed = new Map();
function collect(source) {
  const pkg = JSON.parse(readFileSync(join(source, "package.json"), "utf8"));
  const previous = installed.get(pkg.name);
  if (previous) {
    if (previous !== pkg.version) throw new Error(`Conflicting versions of ${pkg.name}`);
    return;
  }
  installed.set(pkg.name, pkg.version);
  const destination = join("/runtime/opt/markdown/node_modules", pkg.name);
  mkdirSync(dirname(destination), { recursive: true });
  cpSync(source, destination, {
    recursive: true,
    filter: (path) => path === source || basename(path) !== "node_modules"
  });
  for (const name of Object.keys(pkg.dependencies || {})) {
    let parent = source;
    while (!existsSync(join(parent, "node_modules", name, "package.json"))) {
      const next = dirname(parent);
      if (next === parent) throw new Error(`Missing dependency ${name}`);
      parent = next;
    }
    collect(join(parent, "node_modules", name));
  }
}
collect("/usr/local/lib/node_modules/markdownlint-cli2/node_modules/markdownlint");
