// Replace the vulnerable transitive package with its upstream fixed release.
// Keep the complete locked dependency closure local to the replacement package.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = '/usr/local/renovate/node_modules';
const replacement = '/usr/local/renovate/markdown-it-fixed/node_modules/markdown-it';
assert.equal(require(path.join(replacement, 'package.json')).version, '15.0.3');
const targets = [];
function walk(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const child = path.join(directory, entry.name);
    const metadata = path.join(child, 'package.json');
    if (fs.existsSync(metadata)) {
      const pkg = JSON.parse(fs.readFileSync(metadata, 'utf8'));
      if (pkg.name === 'markdown-it') {
        assert.equal(pkg.version, '15.0.2', `unexpected markdown-it at ${child}`);
        targets.push(child);
        continue;
      }
    }
    walk(child);
  }
}
walk(root);
assert.ok(targets.length > 0, 'expected vulnerable markdown-it copy');
for (const target of targets) {
  fs.rmSync(target, { recursive: true });
  fs.symlinkSync(replacement, target, 'dir');
  const MarkdownIt = require(target);
  assert.equal(new MarkdownIt().render('# Fixture'), '<h1>Fixture</h1>\n');
  assert.equal(require(path.join(target, 'package.json')).version, '15.0.3');
}
const MarkdownIt = require(replacement);
const start = performance.now();
new MarkdownIt().render('[a]: /u "' + 'x\n'.repeat(400000));
new MarkdownIt().render('[' + 'a\n'.repeat(100000));
assert.ok(performance.now() - start < 15000, 'multiline references must not render quadratically');
console.log(`Updated ${targets.length} markdown-it copies to upstream 15.0.3`);
