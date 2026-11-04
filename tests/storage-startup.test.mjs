import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import test from 'node:test';
import { pathToFileURL } from 'node:url';
import { CouncilStore } from '../dist/store.js';

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), 'council-storage-startup-'));
  const stores = [], children = [];
  t.after(async () => {
    for (const child of children) if (child.exitCode === null && child.signalCode === null) {
      const closed = once(child, 'close'); child.kill('SIGKILL'); await closed;
    }
    for (const store of stores) { try { store.close(); } catch {} }
    rmSync(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return {
    directory,
    open() { const store = new CouncilStore(directory); stores.push(store); return store; },
    child(code) {
      const child = spawn(process.execPath, ['--input-type=module', '-e', code], {
        env: { ...process.env, COUNCIL_TEST_STATE: directory }, windowsHide: true, stdio: ['ignore', 'ignore', 'pipe', 'ipc'],
      });
      children.push(child);
      let diagnostics = '';
      child.stderr.on('data', chunk => { diagnostics = (diagnostics + chunk).slice(-8192); });
      const message = type => new Promise((accept, reject) => {
        const receive = value => { if (value?.type === type) { cleanup(); accept(value); } };
        const exit = code => { cleanup(); reject(new Error(`Startup child exited ${code} before ${type}: ${diagnostics}`)); };
        const error = value => { cleanup(); reject(value); };
        const cleanup = () => { child.off('message', receive); child.off('exit', exit); child.off('error', error); };
        child.on('message', receive); child.once('exit', exit); child.once('error', error);
      });
      return { child, message, diagnostics: () => diagnostics };
    },
  };
}

const moduleUrl = pathToFileURL(resolve('dist/store.js')).href;

test('a current WAL schema opens during another writer transaction without acquiring a migration lock', t => {
  const setup = fixture(t);
  const original = setup.open();
  original.putContext('committed-note', 'Available while another writer is active.', 'codex');
  const writer = new DatabaseSync(join(setup.directory, 'state.db'));
  try {
    writer.exec('BEGIN IMMEDIATE');
    const opened = setup.open();
    assert.equal(opened.getContext('committed-note').value, 'Available while another writer is active.');
    assert.equal(opened.db.prepare('PRAGMA busy_timeout').get().timeout, 250);
    assert.throws(() => opened.appendEvent('runtime-write', 'blocked', {}), error => error.errcode === 5);
  } finally { writer.exec('ROLLBACK'); writer.close(); }
});

test('independent cold processes can initialize the same new workspace', { timeout: 30000 }, async t => {
  const setup = fixture(t);
  const code = `import {CouncilStore} from ${JSON.stringify(moduleUrl)};
    process.on('message', message => {
      if (message.type !== 'start') return;
      const store = new CouncilStore(process.env.COUNCIL_TEST_STATE);
      process.send({type:'opened',schemaVersion:store.health().schemaVersion,timeout:store.db.prepare('PRAGMA busy_timeout').get().timeout});
      store.close();process.disconnect();
    });process.send({type:'ready'});`;
  const workers = [setup.child(code), setup.child(code)];
  await Promise.all(workers.map(worker => worker.message('ready')));
  const opened = workers.map(worker => worker.message('opened'));
  const exits = workers.map(worker => once(worker.child, 'close'));
  for (const worker of workers) worker.child.send({ type: 'start' });
  for (const result of await Promise.all(opened)) { assert.equal(result.schemaVersion, 5); assert.equal(result.timeout, 250); }
  await Promise.all(exits);
  for (const worker of workers) assert.equal(worker.child.exitCode, 0, worker.diagnostics());
  const store = setup.open();
  assert.equal(store.health().schemaVersion, 5);
  assert.deepEqual(store.checkIntegrity(), ['ok']);
  assert.equal(store.db.prepare('PRAGMA busy_timeout').get().timeout, 250);
});

test('a contender waits for a slow peer migration and skips DDL after that peer upgrades the schema', { timeout: 30000 }, async t => {
  const setup = fixture(t);
  const legacy = new DatabaseSync(join(setup.directory, 'state.db'));
  const timestamp = '2026-01-01T00:00:00.000Z';
  const run = { id: 'slow-migration-result', status: 'completed', createdAt: timestamp, updatedAt: timestamp,
    input: { mode: 'delegate', task: 'Offline startup migration fixture.', agents: ['codex'], permission: 'read' }, steps: [], output: 'x'.repeat(20000) };
  legacy.exec(`PRAGMA journal_mode = WAL;
    CREATE TABLE runs(id TEXT PRIMARY KEY,updated_at TEXT NOT NULL,document TEXT NOT NULL,owner_instance TEXT,owner_pid INTEGER,cancel_requested INTEGER NOT NULL DEFAULT 0);
    CREATE TABLE events(sequence INTEGER PRIMARY KEY AUTOINCREMENT,run_id TEXT NOT NULL,type TEXT NOT NULL,data TEXT NOT NULL,created_at TEXT NOT NULL);
    PRAGMA user_version = 2;`);
  legacy.prepare('INSERT INTO runs(id,updated_at,document) VALUES(?,?,?)').run(run.id, timestamp, JSON.stringify(run));
  legacy.close();
  const control = join(setup.directory, 'contender-entered');
  const code = `import {CouncilStore} from ${JSON.stringify(moduleUrl)};import{existsSync}from'node:fs';import{join}from'node:path';
    const migrate=CouncilStore.prototype.migrateMetadata;
    CouncilStore.prototype.migrateMetadata=function(){
      process.send({type:'locked'});
      const control=join(process.env.COUNCIL_TEST_STATE,'contender-entered');const until=Date.now()+15000;
      while(!existsSync(control)){if(Date.now()>until)throw new Error('Contender never reached the migration lock.');Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,10);}
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,800);
      return migrate.call(this);
    };
    const store=new CouncilStore(process.env.COUNCIL_TEST_STATE);
    process.send({type:'upgraded'});
    process.on('message',message=>{
      if(message.type==='activate'){store.registerInstance('upgraded-peer',process.pid,1);process.send({type:'registered'});}
      if(message.type==='stop'){store.unregisterInstance('upgraded-peer');store.close();process.disconnect();}
    });`;
  const worker = setup.child(code);
  const upgraded = worker.message('upgraded');
  void upgraded.catch(() => {});
  await worker.message('locked');
  const nativeExec = DatabaseSync.prototype.exec;
  let reachedLock = false;
  const ddl = [];
  DatabaseSync.prototype.exec = function (sql) {
    if (sql === 'BEGIN IMMEDIATE') { reachedLock = true; writeFileSync(control, 'release after the startup wait begins'); }
    if (/\b(?:CREATE|ALTER|DROP)\s+(?:TABLE|INDEX)\b/i.test(sql)) ddl.push(sql);
    return nativeExec.call(this, sql);
  };
  let store;
  try { store = setup.open(); } finally { DatabaseSync.prototype.exec = nativeExec; }
  assert.equal(reachedLock, true);
  assert.deepEqual(ddl, [], 'The contender must not rebuild or alter a schema already committed by its peer.');
  assert.deepEqual(store.getRun(run.id), run);
  assert.equal(store.getRunSummary(run.id).output.length, 16000);
  assert.equal(store.db.prepare('PRAGMA busy_timeout').get().timeout, 250);
  await upgraded;
  const registered = worker.message('registered');
  worker.child.send({ type: 'activate' });
  await registered;
  assert.equal(setup.open().health().counts.instances, 1, 'A live host using the current schema does not block warm startup.');
  const closed = once(worker.child, 'close');
  worker.child.send({ type: 'stop' });
  await closed;
  assert.equal(worker.child.exitCode, 0, worker.diagnostics());
});
