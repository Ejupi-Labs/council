import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { fileURLToPath } from 'node:url';

const repository = realpathSync.native(fileURLToPath(new URL('../', import.meta.url))), script = realpathSync.native(fileURLToPath(new URL('../scripts/room-soak.mjs', import.meta.url)));
const events = output => output.trim().split(/\r?\n/).filter(line => line.startsWith('{')).map(line => JSON.parse(line));

test('real-process room soak verifies all scenarios and drains every worker before reporting completion', { timeout: 60000 }, () => {
  const output = execFileSync(process.execPath, [script, '--duration-ms', '1000', '--checkpoint-ms', '1000'], {
    cwd: repository, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
  });
  const rows = events(output), summary = rows.at(-1);
  assert.equal(summary.event, 'finished'); assert.equal(summary.status, 'completed');
  assert.equal(summary.cycles, 5); assert.equal(summary.fakeAgentCalls, 25); assert.equal(summary.liveProviderCalls, 0);
  assert.ok(Object.values(summary.scenarios).every(count => count === 1));
  assert.equal(summary.workersStarted, 2); assert.equal(summary.workersExited, 2); assert.equal(summary.fixtureRemoved, true);
  assert.ok(summary.peakObservedLogicalFixtureBytes > 0 && summary.peakObservedLogicalFixtureBytes < summary.fixtureBudgetBytes);
  assert.match(summary.runtimeDigest, /^[a-f0-9]{64}$/); assert.match(summary.harnessDigest, /^[a-f0-9]{64}$/);
  assert.equal(summary.runtimeDigest, rows[0].runtimeDigest); assert.equal(summary.harnessDigest, rows[0].harnessDigest);
});

test('interrupted room soak reports cancellation after process drain and preserves inspectable state', { timeout: 60000 }, () => {
  const url = new URL('../scripts/room-soak.mjs', import.meta.url).href;
  const driver = `import { roomSoak, roomSoakOptions } from ${JSON.stringify(url)};
const controller = new AbortController();
const timer = setTimeout(() => controller.abort(new DOMException('Deliberate offline soak interruption.', 'AbortError')), 1000);
try { await roomSoak(roomSoakOptions(['--duration-ms', '60000', '--checkpoint-ms', '1000']), controller.signal); }
catch (error) { process.stderr.write(error.message + '\\n'); process.exitCode = error.status === 'cancelled' ? 130 : 1; }
finally { clearTimeout(timer); }`;
  let captured;
  try { execFileSync(process.execPath, ['--input-type=module', '--eval', driver], {
    cwd: repository, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
  }); assert.fail('The explicit interruption must not report success.'); } catch (error) { captured = error; }
  assert.equal(captured.status, 130, captured.stderr);
  const rows = events(captured.stdout), summary = rows.at(-1);
  assert.equal(summary.status, 'cancelled'); assert.equal(summary.fixtureRemoved, false); assert.equal(summary.liveProviderCalls, 0);
  assert.equal(summary.workersStarted, summary.workersExited);
  assert.ok(!rows.some(row => row.status === 'completed'));
  const root = realpathSync.native(summary.fixture), parent = realpathSync.native(tmpdir());
  assert.equal(dirname(root), parent); assert.ok(basename(root).startsWith('council-room-soak-'));
  let verified = false;
  try {
    for (const name of readdirSync(root).filter(name => name.startsWith('fixture-'))) {
      const state = join(root, name, 'project', '.council', 'state.db'); if (!existsSync(state)) continue;
      const db = new DatabaseSync(state, { readOnly: true });
      try {
        assert.equal(db.prepare('PRAGMA quick_check').get().quick_check, 'ok');
        assert.equal(db.prepare('SELECT COUNT(*) AS count FROM instances').get().count, 0);
        assert.equal(db.prepare('SELECT COUNT(*) AS count FROM slots').get().count, 0);
        assert.equal(db.prepare('SELECT COUNT(*) AS count FROM slot_waiters').get().count, 0);
        assert.equal(db.prepare("SELECT COUNT(*) AS count FROM runs WHERE status IN ('queued','running')").get().count, 0);
      } finally { db.close(); }
    }
    verified = true;
  } finally { if (verified) rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); }
});

test('room soak runs through aliased entrypoint and temporary paths using canonical owned fixtures', { timeout: 60000 }, () => {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-room-soak-alias-')));
  const source = join(root, 'source'), alias = join(root, 'source-alias'), actualTemp = join(root, 'actual-temp'), aliasTemp = join(root, 'temp-alias');
  mkdirSync(source); mkdirSync(join(source, 'scripts')); mkdirSync(actualTemp);
  cpSync(join(repository, 'dist'), join(source, 'dist'), { recursive: true }); writeFileSync(join(source, 'package.json'), readFileSync(join(repository, 'package.json')));
  for (const name of ['room-soak.mjs', 'coordination-soak.mjs']) writeFileSync(join(source, 'scripts', name), readFileSync(join(repository, 'scripts', name)));
  symlinkSync(source, alias, process.platform === 'win32' ? 'junction' : 'dir'); symlinkSync(actualTemp, aliasTemp, process.platform === 'win32' ? 'junction' : 'dir');
  let verified = false;
  try {
    const output = execFileSync(process.execPath, [join(alias, 'scripts', 'room-soak.mjs'), '--duration-ms', '1000', '--checkpoint-ms', '1000'], {
      cwd: alias, env: { ...process.env, TMPDIR: aliasTemp, TEMP: aliasTemp, TMP: aliasTemp }, windowsHide: true,
      stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
    });
    const rows = events(output), summary = rows.at(-1);
    assert.equal(summary?.status, 'completed', 'An aliased script entrypoint must actually run, not silently exit without workload evidence.');
    assert.equal(summary.cycles, 5); assert.equal(summary.workersStarted, 2); assert.equal(summary.workersExited, 2); assert.equal(summary.fixtureRemoved, true);
    assert.equal(readdirSync(actualTemp).length, 0, 'The native temporary target must contain no remaining owned fixture.');
    verified = true;
  } finally {
    if (verified) {
      assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-room-soak-alias-'));
      for (const path of [alias, aliasTemp]) assert.ok(realpathSync.native(path).startsWith(root + (process.platform === 'win32' ? '\\' : '/')), 'Alias cleanup must stay inside the owned fixture.');
      rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    }
  }
});

test('invalid room-soak options fail before starting a workload or creating a progress log', () => {
  let captured;
  try { execFileSync(process.execPath, [script, '--duration-ms', '0'], { cwd: repository, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 10000 });
    assert.fail('A zero-duration workload must be rejected.'); } catch (error) { captured = error; }
  assert.equal(captured.status, 1); assert.equal(captured.stdout, ''); assert.match(captured.stderr, /Duration must be/);
});
