// Literal-file adapter: no CLI glob engine, fixing, or source modification.
import { existsSync } from "node:fs";
import { lint, readConfig } from "markdownlint/promise";

const config = existsSync(".markdownlint.json")
  ? await readConfig(".markdownlint.json")
  : undefined;
const results = await lint({ files: [process.argv[2]], config });
for (const [file, errors] of Object.entries(results)) {
  for (const error of errors) {
    console.error(`${file}:${error.lineNumber} ${error.ruleNames.join("/")} ${error.ruleDescription}${error.errorDetail ? `: ${error.errorDetail}` : ""}`);
    process.exitCode = 1;
  }
}
