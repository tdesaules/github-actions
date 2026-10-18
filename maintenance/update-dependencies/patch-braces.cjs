// GHSA-vfj7-8cjw-p6xm: bound every recursive AST walker, including direct AST APIs.
// Keep upstream package identity/version in the SBOM; review requires exact bytes.
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync, writeFileSync } = require('node:fs');
const path = '/usr/local/renovate/node_modules/.pnpm/braces@3.0.3/node_modules/braces';
const original = {
  'compile.js': 'dc98f22eee3d511785d92a00758d5f0d48efed5f5813bdecc2de430c529b5c9f',
  'expand.js': '41ccc196ebfa7b7781a634e721eb744e4e7bcb54cba427a7e3d6806a1b9e58f7',
  'stringify.js': '379f22d77bfa1478341ccd49c5e4267464aabcbba03558bab332aac23fc6f23a'
};
const hash = data => createHash('sha256').update(data).digest('hex');
for (const [file, expected] of Object.entries(original)) {
  const target = `${path}/lib/${file}`;
  let source = readFileSync(target, 'utf8');
  assert.equal(hash(source), expected, `Upstream ${file} changed; re-review the patch`);
  const walker = file === 'stringify.js' ? 'stringify' : 'walk';
  source = source.replace(`const ${walker} = (node, parent = {}) => {`,
    `const ${walker} = (node, parent = {}, depth = 0) => {\n` +
    `    if (depth > 64) {\n` +
    `      const error = new RangeError('brace nesting exceeds the supported depth of 64');\n` +
    `      error.code = 'ERR_BRACES_DEPTH';\n` +
    `      throw error;\n` +
    `    }`);
  source = source.replace('walk(child, node)', 'walk(child, node, depth + 1)');
  source = source.replace('stringify(child)', 'stringify(child, {}, depth + 1)');
  assert.notEqual(hash(source), expected);
  writeFileSync(target, source);
  console.log(`${file} ${hash(source)}`);
}

const braces = require(path);
assert.deepEqual(braces('src/{a,b}.js'), ['src/(a|b).js']);
assert.deepEqual(braces.expand('src/{a,b}.js'), ['src/a.js', 'src/b.js']);
assert.equal(braces.stringify(braces.parse('src/{a,b}.js')), 'src/{a,b}.js');
// The original advisory payload stays below the upstream 10,000-character cap.
const nested = '{'.repeat(4000) + 'a,b' + '}'.repeat(4000);
for (const operation of ['compile', 'expand', 'stringify']) {
  assert.throws(() => braces[operation](braces.parse(nested)), { code: 'ERR_BRACES_DEPTH' });
}
// An invalid subtree takes the stringify path from expand and must also be bounded.
assert.throws(() => braces.expand(nested), { code: 'ERR_BRACES_DEPTH' });
