import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomUUID } from 'node:crypto';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
function fixture(t, runner) {
  const root = mkdtempSync(join(tmpdir(), 'council-cli-wait-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), runner);
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  return { root, service, wait(id) {
    const response = spawnSync(process.execPath, [cli, 'wait', id, '--workspace', root, '--backend', 'mock', '--wait-ms', '0'],
      { shell: false, windowsHide: true, encoding: 'utf8', timeout: 15000, env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' } });
    assert.ifError(response.error); return { ...response, result: JSON.parse(response.stdout) };
  } };
}

test('CLI wait exits successfully only for completed results and preserves terminal status', t => {
  const wire = fixture(t);
  for (const status of ['completed', 'failed', 'cancelled']) {
    const id = randomUUID(), stamp = new Date().toISOString();
    wire.service.store.saveRun({ id, createdAt: stamp, updatedAt: stamp, status, steps: [], output: `${status} evidence`,
      input: { mode: 'delegate', agents: ['codex'], permission: 'read', task: 'Saved offline result.' } });
    const response = wire.wait(id);
    assert.equal(response.status, status === 'completed' ? 0 : 1, response.stderr);
    assert.equal(response.result.runs[0].status, status); assert.equal(response.result.runs[0].output, `${status} evidence`);
  }
});

test('CLI wait timeout exits nonzero and leaves admitted work running', async t => {
  let entered, signal;
  const started = new Promise(resolve => { entered = resolve; });
  const wire = fixture(t, { doctor: async () => [], run: request => new Promise(resolve => {
    signal = request.signal;
    request.signal.addEventListener('abort', () => resolve({ agent: request.agent, status: 'cancelled', output: 'Partial offline evidence.', durationMs: 1 }), { once: true });
    entered();
  }) });
  const queued = wire.service.start({ mode: 'delegate', agents: ['codex'], task: 'Wait for local release.' });
  await started;
  const response = wire.wait(queued.id);
  assert.equal(response.status, 1, response.stderr); assert.equal(response.result.timedOut, true);
  assert.equal(response.result.runs[0].status, 'running'); assert.equal(signal.aborted, false);
});
