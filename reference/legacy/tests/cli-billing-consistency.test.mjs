import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { promisify } from 'node:util';
import { BillingManager } from '../dist/billing.js';
import { loadConfig } from '../dist/config.js';

const execute = promisify(execFile), cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const billingModule = new URL('../dist/billing.js', import.meta.url).href, configModule = new URL('../dist/config.js', import.meta.url).href;
function fixture(t, { observed = true } = {}) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-cli-billing-'));
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ backend: 'mock', agents: { worker: { enabled: true, profile: 'codex' } } }));
  const config = loadConfig(root), writer = new BillingManager(config), clock = Date.now();
  const observation = remaining => ({ mode: 'subscription', source: { kind: 'reported', label: 'Offline CLI observation ' + remaining },
    observedAt: new Date(clock - 1000).toISOString(), validUntil: new Date(clock + 30000).toISOString(),
    windows: [{ id: 'included', label: 'Included allowance', unit: 'requests', scope: 'subscription', remaining },
      { id: 'paid-credits', label: 'Paid credits', unit: 'credits', scope: 'credits', remaining: 500 }] });
  if (observed) {
    writer.importSnapshot('worker', observation(5));
    writer.update('worker', { stopAtSubscriptionLimit: true, subscriptionWindowIds: ['included'] });
  }
  t.after(() => {
    writer.close(); assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-cli-billing-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, writer, observation, clock,
    async call(preload) {
      const args = [];
      if (preload) {
        const path = join(root, 'billing-preload.mjs');
        writeFileSync(path, `import { BillingManager } from ${JSON.stringify(billingModule)};\nimport { loadConfig } from ${JSON.stringify(configModule)};\n${preload}\n`);
        args.push('--import', pathToFileURL(path).href);
      }
      const result = await execute(process.execPath, [...args, cli, 'billing', '--agent', 'worker', '--workspace', root, '--backend', 'mock'], {
        windowsHide: true, timeout: 15000, maxBuffer: 1024 * 1024,
        env: { ...process.env, COUNCIL_BACKEND: 'mock', COUNCIL_DEPTH: '0' },
      });
      return { ...result, record: JSON.parse(result.stdout) };
    },
  };
}

test('single-agent CLI billing classifies its captured observation when a second SQLite writer updates the account', { timeout: 20000 }, async t => {
  const wire = fixture(t), revision = wire.writer.get('worker').revision;
  const result = await wire.call(`
const original = BillingManager.prototype.get;
let injected = false;
BillingManager.prototype.get = function(alias) {
  const captured = original.call(this, alias);
  if (!injected && alias === 'worker') {
    injected = true;
    const writer = new BillingManager(loadConfig(${JSON.stringify(wire.root)}));
    try { writer.importSnapshot('worker', ${JSON.stringify(wire.observation(0))}); }
    finally { writer.close(); }
  }
  return captured;
};
process.on('exit', () => { if (!injected) process.exitCode = 80; });
`);
  assert.equal(result.record.revision, revision); assert.equal(result.record.windows[0].remaining, 5);
  assert.equal(result.record.source.label, 'Offline CLI observation 5');
  assert.equal(result.record.availability.status, 'available'); assert.equal(result.record.availability.windows[0].effectiveRemaining, 5);
  assert.equal(result.record.subscriptionProtection.status, 'available'); assert.equal(result.record.subscriptionProtection.allowed, true);
  assert.equal(result.record.subscriptionProtection.evaluatedAt, result.record.availability.evaluatedAt);
  const latest = wire.writer.get('worker'); assert.equal(latest.windows[0].remaining, 0); assert.equal(latest.revision, revision + 1);
  assert.equal(wire.writer.subscriptionProtectionFor(latest).status, 'exhausted');
  assert.equal(result.stderr.includes('run.started'), false);
});

test('single-agent CLI quota and subscription protection use one evaluation time across an expiry boundary', { timeout: 20000 }, async t => {
  const wire = fixture(t);
  const result = await wire.call(`
Date.now = () => ${wire.clock};
const original = BillingManager.prototype.availabilityFor;
let evaluated = false;
BillingManager.prototype.availabilityFor = function(record, now) {
  const captured = original.call(this, record, now);
  evaluated = true;
  Date.now = () => ${wire.clock + 60000};
  return captured;
};
process.on('exit', () => { if (!evaluated) process.exitCode = 81; });
`);
  assert.equal(result.record.availability.status, 'available');
  assert.equal(result.record.subscriptionProtection.status, 'available'); assert.equal(result.record.subscriptionProtection.allowed, true);
  assert.equal(result.record.availability.evaluatedAt, new Date(wire.clock).toISOString());
  assert.equal(result.record.subscriptionProtection.evaluatedAt, result.record.availability.evaluatedAt);
});

test('single-agent CLI billing preserves unknown observations and reports disabled subscription protection without inferring capacity', { timeout: 20000 }, async t => {
  const wire = fixture(t, { observed: false }), result = await wire.call();
  assert.equal(result.record.mode, 'unknown'); assert.equal(result.record.source, null); assert.equal(result.record.observedAt, null);
  assert.deepEqual(result.record.windows, []); assert.equal(result.record.availability.status, 'unknown');
  assert.equal(result.record.subscriptionProtection.enabled, false); assert.equal(result.record.subscriptionProtection.allowed, true);
  assert.equal(result.record.subscriptionProtection.status, 'disabled');
  assert.equal(result.record.subscriptionProtection.evaluatedAt, result.record.availability.evaluatedAt);
  assert.equal(result.stderr.includes('run.started'), false);
});
