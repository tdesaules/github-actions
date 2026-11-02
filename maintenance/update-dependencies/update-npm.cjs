// npm bundles Undici; npm overrides do not replace files inside that bundle.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = '/npm-runtime';
const source = path.join(root, 'node_modules/undici');
const target = path.join(root, 'node_modules/npm/node_modules/undici');
const expected = require(path.join(root, 'package.json')).dependencies.undici;
const replacement = require(path.join(source, 'package.json'));
const bundled = require(path.join(target, 'package.json'));
assert.equal(replacement.version, expected);
assert.equal(replacement.name, 'undici');
assert.equal(bundled.name, 'undici');
assert.equal(bundled.version.split('.')[0], replacement.version.split('.')[0]);
fs.rmSync(target, { recursive: true });
fs.cpSync(source, target, { recursive: true });
assert.equal(JSON.parse(fs.readFileSync(path.join(target, 'package.json'))).version, expected);
