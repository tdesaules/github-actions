import test from 'node:test';
import assert from 'node:assert/strict';
import { protocol } from '../adapter.mjs';
test('protocol', () => assert.equal(protocol, 1));
