// Keep the upstream package identity while installing the fixed Undici release.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = '/usr/local/renovate/node_modules';
const replacement = '/usr/local/renovate/undici-fixed/node_modules/undici';
assert.equal(require(path.join(replacement, 'package.json')).version, '8.12.2');
const targets = [];
function walk(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const child = path.join(directory, entry.name);
    const metadata = path.join(child, 'package.json');
    if (fs.existsSync(metadata)) {
      const pkg = JSON.parse(fs.readFileSync(metadata, 'utf8'));
      if (pkg.name === 'undici') {
        assert.equal(pkg.version, '8.11.2', `unexpected Undici at ${child}`);
        targets.push(child);
        continue;
      }
    }
    walk(child);
  }
}
walk(root);
assert.ok(targets.length > 0, 'expected vulnerable Undici copy');
for (const target of targets) {
  fs.rmSync(target, { recursive: true });
  fs.symlinkSync(replacement, target, 'dir');
  assert.equal(require(path.join(target, 'package.json')).version, '8.12.2');
}

async function test() {
  const { MockAgent, request } = require(replacement);
  const dispatcher = new MockAgent();
  dispatcher.disableNetConnect();
  dispatcher.get('https://fixture.invalid').intercept({ path: '/', method: 'GET' }).reply(200, 'fixture');
  const response = await request('https://fixture.invalid/', { dispatcher });
  assert.equal(response.statusCode, 200);
  assert.equal(await response.body.text(), 'fixture');
  dispatcher.assertNoPendingInterceptors();
  await dispatcher.close();

  // The previous Markdown remediation is now supplied by Renovate upstream.
  assert.equal(require(`${root}/markdown-it/package.json`).version, '15.0.3');
  assert.equal(require(`${root}/zod/package.json`).version, '4.6.7');
  const MarkdownIt = require(`${root}/markdown-it`);
  assert.equal(new MarkdownIt().render('# Fixture'), '<h1>Fixture</h1>\n');
  const start = performance.now();
  new MarkdownIt().render('[a]: /u "' + 'x\n'.repeat(400000));
  new MarkdownIt().render('[' + 'a\n'.repeat(100000));
  assert.ok(performance.now() - start < 15000, 'multiline references must not render quadratically');
  console.log(`Updated ${targets.length} Undici copies to upstream 8.12.2; Zod and Markdown regressions passed`);
}
test().catch(error => { console.error(error); process.exitCode = 1; });
