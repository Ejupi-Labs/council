import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import test from 'node:test';
import { inflateRawSync } from 'node:zlib';
import { packagePreview } from '../../scripts/package.mjs';
import { root } from '../../scripts/tools.mjs';
import { fixture, ownerAPI } from './helpers.mjs';

function archiveContents(data) {
  const end = data.length - 22;
  assert.equal(data.readUInt32LE(end), 0x06054b50);
  const count = data.readUInt16LE(end + 10); let cursor = data.readUInt32LE(end + 16);
  const entries = [];
  for (let i = 0; i < count; i++) {
    assert.equal(data.readUInt32LE(cursor), 0x02014b50);
    const method = data.readUInt16LE(cursor + 10), compressedSize = data.readUInt32LE(cursor + 20);
    const nameLength = data.readUInt16LE(cursor + 28), extraLength = data.readUInt16LE(cursor + 30), commentLength = data.readUInt16LE(cursor + 32);
    const offset = data.readUInt32LE(cursor + 42), name = data.subarray(cursor + 46, cursor + 46 + nameLength).toString('utf8');
    assert.equal(data.readUInt32LE(offset), 0x04034b50);
    const start = offset + 30 + data.readUInt16LE(offset + 26) + data.readUInt16LE(offset + 28);
    assert.equal(method, 8);
    entries.push({ name, contents: inflateRawSync(data.subarray(start, start + compressedSize)) });
    cursor += 46 + nameLength + extraLength + commentLength;
  }
  return entries;
}

test('actual partial publication remains inspectable and outside Git and release output', { timeout: 180000 }, async t => {
  const f = await fixture(t);
  const git = (...args) => {
    const result = spawnSync('git', args, { cwd: f.project, env: f.env, encoding: 'utf8', shell: false });
    assert.equal(result.status, 0, result.stderr); return result.stdout;
  };
  const statusBefore = git('status', '--porcelain=v1', '-z'), headBefore = git('rev-parse', 'HEAD');
  const binary = path.join(f.home, process.platform === 'win32' ? 'history-fixture.test.exe' : 'history-fixture.test');
  const compiled = spawnSync('go', ['test', '-p', '1', '-c', '-o', binary, './internal/engine'], { cwd: root, env: process.env, encoding: 'utf8', shell: false, timeout: 120000 });
  assert.equal(compiled.status, 0, compiled.stderr || compiled.stdout);
  const marker = path.join(f.home, 'partial-owner-marker.json');
  const stateRoot = path.join(f.home, '.council', 'v2');
  const child = spawn(binary, ['-test.run=^TestHistoryPublicationAbruptOwnerBoundaries$', '-test.count=1'], {
    cwd: root, shell: false, stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...f.env, COUNCIL_HISTORY_CRASH_CHILD: '1', COUNCIL_HISTORY_CRASH_ROOT: stateRoot, COUNCIL_HISTORY_CRASH_REPO: f.project, COUNCIL_HISTORY_CRASH_MARKER: marker, COUNCIL_HISTORY_CRASH_STAGE: 'after_write' },
  });
  let errors = ''; child.stdout.on('data', chunk => { errors += chunk.toString(); }); child.stderr.on('data', chunk => { errors += chunk.toString(); });
  t.after(async () => { if (child.exitCode === null && child.signalCode === null) { const exited = once(child, 'exit'); child.kill(); await exited; } });
  let record;
  const deadline = Date.now() + 90000;
  while (!record && Date.now() < deadline) {
    try { record = JSON.parse(await readFile(marker, 'utf8')); } catch { /* Wait for the child's complete marker. */ }
    if (child.exitCode !== null || child.signalCode !== null) throw new Error('Controlled partial owner exited before its barrier: ' + errors);
    if (!record) await delay(25);
  }
  assert.ok(record?.operationId, 'real owner must reach its native after-write barrier');
  const exited = once(child, 'exit'); assert.equal(child.kill(), true); await exited;
  const target = path.join(stateRoot, record.filename), partial = await readFile(target);
  assert.ok(partial.length > 0 && partial.length < record.expectedBytes, 'the physical file must actually be partial');

  const owner = await f.start();
  const page = await f.browser.newPage(); await page.goto(owner.url);
  await page.waitForFunction(() => !document.querySelector('#history-operation-form button')?.disabled);
  await page.locator('#history-panel > summary').click();
  await page.locator('#history-operation-id').fill(record.operationId);
  await page.getByRole('button', { name: 'Inspect operation', exact: true }).click();
  await page.locator('#history-operation-facts').filter({ hasText: record.operationId }).waitFor();
  assert.match(await page.locator('#history-operation-facts').textContent(), /effect_started/);
  assert.match(await page.locator('#history-operation-note').textContent(), /partial file may remain/);
  const pathName = '/api/v1/history/operations/' + record.operationId + '/reconciliations';
  const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === pathName);
  await page.getByRole('button', { name: 'Verify no publication', exact: true }).click();
  const observed = await response, envelope = await observed.json();
  assert.equal(observed.status(), 200, JSON.stringify(envelope.error));
  assert.equal(envelope.data.decision, 'unresolved');
  assert.ok(envelope.data.observationCodes.includes('permit_present_or_uncertain'));
  const meta = await ownerAPI(owner, '/meta'); assert.equal(meta.envelope.data.admission.state, 'fenced');
  assert.deepEqual(await readFile(target), partial, 'inspection and reconciliation must not alter partial output');

  const release = await packagePreview({ outputDirectory: path.join(f.home, 'release'), build: false });
  for (const entry of archiveContents(await readFile(release.archive))) {
    assert.equal(entry.name.includes(record.filename), false, 'release entry includes private output name');
    assert.equal(entry.contents.includes(partial), false, 'release entry includes private partial bytes');
  }
  assert.equal(git('status', '--porcelain=v1', '-z'), statusBefore, 'history must not stage or change project files');
  assert.equal(git('rev-parse', 'HEAD'), headBefore, 'history must not commit project state');
  assert.deepEqual(await readFile(path.join(f.project, '.git', 'index')), f.indexBefore, 'project index must be retained exactly');
});
