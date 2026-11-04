import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import fs from 'node:fs/promises';
import { syncBuiltinESMExports } from 'node:module';
import { setImmediate as nextTurn, setTimeout as delay } from 'node:timers/promises';
import { AcpFiles, MAX_ACP_FILE_BYTES } from '../dist/acp-files.js';

function fixture(t) {
  const temporary = mkdtempSync(join(tmpdir(), 'council-acp-files-'));
  const root = join(temporary, 'project'); mkdirSync(root);
  const outside = join(temporary, 'outside'); mkdirSync(outside);
  t.after(() => rmSync(temporary, { recursive: true, force: true }));
  return { root, outside, files: new AcpFiles(root, 'write'), reader: new AcpFiles(root, 'read') };
}

test('reads UTF-8 and preserves Unicode, CRLF, empty lines and one-based slices', async t => {
  const { root, reader } = fixture(t);
  const path = join(root, 'Unicode 🌍 notes.txt');
  const text = 'first 🌍\r\n\r\nthird ü\nlast'; writeFileSync(path, text);
  assert.deepEqual(await reader.read({ path }), { content: text });
  assert.deepEqual(await reader.read({ path, line: 1, limit: 1 }), { content: 'first 🌍\r\n' });
  assert.deepEqual(await reader.read({ path, line: 2, limit: 2 }), { content: '\r\nthird ü\n' });
  assert.deepEqual(await reader.read({ path, line: 4 }), { content: 'last' });
  assert.deepEqual(await reader.read({ path, line: 999 }), { content: '' });
  assert.equal(await reader.contains(path), true);
});

test('writes atomically, creates contained parent directories and returns null', async t => {
  const { root, files } = fixture(t);
  const path = join(root, 'new', 'nested', 'file.txt');
  assert.equal(await files.contains(path), true);
  assert.equal(await files.write({ path, content: 'Unicode 🌍 text\n' }), null);
  assert.equal(readFileSync(path, 'utf8'), 'Unicode 🌍 text\n');
  assert.equal(await files.write({ path, content: 'replaced\n' }), null);
  assert.equal(readFileSync(path, 'utf8'), 'replaced\n');
  assert.deepEqual(readdirSync(dirname(path)), ['file.txt']);
});

test('Windows case-insensitive directory spelling remains valid for atomic writes', { skip: process.platform !== 'win32' }, async t => {
  const { root, files } = fixture(t);
  mkdirSync(join(root, 'src'));
  const path = join(root, 'src', 'file.txt'); writeFileSync(path, 'original');
  const alternate = join(root, 'SRC', 'file.txt');
  assert.equal(await files.contains(alternate), true);
  assert.equal(await files.write({ path: alternate, content: 'updated' }), null);
  assert.equal(readFileSync(path, 'utf8'), 'updated');
  assert.deepEqual(readdirSync(dirname(path)), ['file.txt']);
  const rootAlias = join(root.toUpperCase(), 'src', 'file.txt');
  assert.equal(await files.write({ path: rootAlias, content: 'root spelling preserved' }), null);
  assert.equal(readFileSync(path, 'utf8'), 'root spelling preserved');
});

test('an explicitly configured root junction still maps to its selected canonical project', async t => {
  const { root, outside } = fixture(t);
  const alias = join(outside, 'configured-root'); symlinkSync(root, alias, 'junction');
  const path = join(root, 'file.txt'); writeFileSync(path, 'original');
  const files = new AcpFiles(alias, 'write');
  assert.deepEqual(await files.read({ path: join(alias, 'file.txt') }), { content: 'original' });
  assert.equal(await files.write({ path: join(alias, 'file.txt'), content: 'updated through configured root' }), null);
  assert.equal(readFileSync(path, 'utf8'), 'updated through configured root');
});

test('case-sensitive NTFS siblings cannot escape the selected root through a case-folded prefix', { skip: process.platform !== 'win32', timeout: 10000 }, async t => {
  const parent = mkdtempSync(join(tmpdir(), 'council-acp-case-sensitive-'));
  t.after(() => rmSync(parent, { recursive: true, force: true }));
  try {
    execFileSync(join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'fsutil.exe'), ['file', 'setCaseSensitiveInfo', parent, 'enable'], { windowsHide: true, stdio: 'pipe' });
  } catch { t.skip('Per-directory NTFS case sensitivity is unavailable for this volume or user.'); return; }
  const root = join(parent, 'project'), outside = join(parent, 'PROJECT');
  mkdirSync(root); mkdirSync(outside);
  const path = join(outside, 'private.txt'); writeFileSync(path, 'outside fixture');
  const files = new AcpFiles(root, 'write');
  assert.equal(await files.contains(path), false);
  await assert.rejects(files.read({ path }), /outside.*root/);
  await assert.rejects(files.write({ path, content: 'overwrite' }), /outside.*root/);
  await assert.rejects(files.write({ path: join(outside, 'new', 'file.txt'), content: 'create outside' }), /outside.*root/);
  assert.equal(readFileSync(path, 'utf8'), 'outside fixture');
  assert.deepEqual(readdirSync(outside), ['private.txt']);
  assert.deepEqual(readdirSync(root), []);
});

test('read policy refuses writes without creating files or directories', async t => {
  const { root, reader } = fixture(t);
  const path = join(root, 'missing', 'file.txt');
  await assert.rejects(reader.write({ path, content: 'disallowed' }), /read-only/);
  assert.equal(existsSync(dirname(path)), false);
});

test('absolute paths outside the root and normalized traversal escapes are rejected', async t => {
  const { root, outside, files } = fixture(t);
  const path = join(outside, 'secret.txt'); writeFileSync(path, 'outside stays private');
  for (const unsafe of [path, join(root, '..', 'outside', 'secret.txt'), 'relative.txt']) {
    assert.equal(await files.contains(unsafe), false);
    await assert.rejects(files.read({ path: unsafe }), /outside|absolute/);
    await assert.rejects(files.write({ path: unsafe, content: 'overwrite' }), /outside|absolute/);
  }
  assert.equal(readFileSync(path, 'utf8'), 'outside stays private');
});

test('Git controls and Council metadata are unavailable for both read and write', async t => {
  const { root, files } = fixture(t);
  for (const name of ['.git', '.council', '.GIT', '.council.']) {
    const path = join(root, name, 'private.txt');
    assert.equal(await files.contains(path), false);
    await assert.rejects(files.read({ path }), /Git control|Council state/);
    await assert.rejects(files.write({ path, content: 'blocked' }), /Git control|Council state/);
  }
  assert.equal(await files.contains(join(root, '.gitignore')), true);
});

for (const [name, alias] of [['.git', 'GIT~1'], ['.council', 'COUNCI~1']]) {
  test(`Windows short names cannot expose ${name} metadata or create descendants`, { skip: process.platform !== 'win32' }, async t => {
    const { root, files, reader } = fixture(t);
    const metadata = join(root, name); mkdirSync(metadata);
    const original = join(metadata, 'private.txt'); writeFileSync(original, 'private fixture');
    const alternate = join(root, alias);
    if (!existsSync(alternate)) { t.skip('This volume does not create the expected Windows 8.3 alias.'); return; }
    assert.equal(await reader.contains(join(alternate, 'private.txt')), false);
    await assert.rejects(reader.read({ path: join(alternate, 'private.txt') }), /Git control|Council state/);
    await assert.rejects(files.write({ path: join(alternate, 'new', 'file.txt'), content: 'blocked' }), /Git control|Council state/);
    assert.equal(readFileSync(original, 'utf8'), 'private fixture');
    assert.deepEqual(readdirSync(metadata), ['private.txt']);
  });
}

test('parent symlinks or junctions cannot redirect reads, writes or new directories', async t => {
  const { root, outside, files } = fixture(t);
  const secret = join(outside, 'secret.txt'); writeFileSync(secret, 'outside');
  const link = join(root, 'link'); symlinkSync(outside, link, 'junction');
  for (const path of [join(link, 'secret.txt'), join(link, 'new', 'file.txt')]) {
    assert.equal(await files.contains(path), false);
    await assert.rejects(files.read({ path }), /Symlinks|junctions/);
    await assert.rejects(files.write({ path, content: 'blocked' }), /Symlinks|junctions/);
  }
  assert.equal(readFileSync(secret, 'utf8'), 'outside'); assert.equal(existsSync(join(outside, 'new')), false);
});

test('a final file symlink is refused before reading or replacing its target', async t => {
  const { root, outside, files } = fixture(t);
  const target = join(outside, 'secret.txt'); writeFileSync(target, 'original outside text');
  const link = join(root, 'final-link.txt');
  try { symlinkSync(target, link, 'file'); }
  catch (error) { if (['EPERM', 'EACCES'].includes(error.code)) { t.skip('File symlinks require Windows Developer Mode or elevated rights.'); return; } throw error; }
  assert.equal(await files.contains(link), false);
  await assert.rejects(files.read({ path: link }), /Symlinks|junctions/);
  await assert.rejects(files.write({ path: link, content: 'blocked' }), /Symlinks|junctions/);
  assert.equal(readFileSync(target, 'utf8'), 'original outside text');
});

test('directory endpoints and non-directory ancestors are not regular text files', async t => {
  const { root, files } = fixture(t);
  const regular = join(root, 'file.txt'); writeFileSync(regular, 'text');
  await assert.rejects(files.read({ path: root }), /regular/);
  await assert.rejects(files.write({ path: join(regular, 'nested'), content: 'blocked' }), /ancestor/);
});

test('invalid parameters, invalid UTF-8 and oversized payloads fail before mutation', async t => {
  const { root, files } = fixture(t);
  const path = join(root, 'file.txt'); writeFileSync(path, 'original');
  for (const params of [null, [], {}, { path: 7 }, { path, line: 0 }, { path, line: 1.5 }, { path, limit: -1 }, { path, limit: Number.POSITIVE_INFINITY }, { path: path + '\0' }]) await assert.rejects(files.read(params));
  for (const params of [null, [], {}, { path, content: 7 }, { path, content: 'x'.repeat(MAX_ACP_FILE_BYTES + 1) }, { path, content: '🌍'.repeat(MAX_ACP_FILE_BYTES / 3) }, { path, content: '\ud800' }]) await assert.rejects(files.write(params));
  assert.equal(readFileSync(path, 'utf8'), 'original');
  writeFileSync(path, Buffer.from([0xff, 0xfe])); await assert.rejects(files.read({ path }), /UTF-8/);
  writeFileSync(path, Buffer.alloc(MAX_ACP_FILE_BYTES + 1, 120)); await assert.rejects(files.read({ path, limit: 1 }), /1 MiB/);
});

test('JSON-escaped responses are bounded independently of file size', async t => {
  const { root, reader } = fixture(t);
  const path = join(root, 'escaped.txt'); writeFileSync(path, '\0'.repeat(400_000));
  await assert.rejects(reader.read({ path }), /JSON encoding/);
});

test('aborted file operations do not modify the project', async t => {
  const { root, files } = fixture(t);
  const path = join(root, 'file.txt'); writeFileSync(path, 'original');
  const controller = new AbortController(); controller.abort();
  await assert.rejects(files.read({ path }, controller.signal));
  await assert.rejects(files.write({ path, content: 'blocked' }, controller.signal));
  assert.equal(readFileSync(path, 'utf8'), 'original');
});

for (const operation of ['read', 'write']) test(`cancellation interrupts delayed ${operation} metadata without issuing later file operations`, { timeout: 5000 }, async t => {
  const { root, files } = fixture(t);
  const path = join(root, 'existing.txt'); writeFileSync(path, 'original');
  await files.contains(path); // Resolve the shared canonical-root lookup before injection.
  const controller = new AbortController();
  const originals = Object.fromEntries(['lstat', 'realpath', 'open', 'mkdir', 'rename', 'unlink'].map(name => [name, fs[name]]));
  const calls = [];
  let notify, release;
  const started = new Promise(resolve => { notify = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  let stalled = false, observed;
  try {
    for (const [name, original] of Object.entries(originals)) fs[name] = async (...args) => {
      calls.push(name);
      if (name === 'lstat' && args[0] === path && !stalled) { stalled = true; notify(); await gate; }
      return original(...args);
    };
    syncBuiltinESMExports();
    const work = operation === 'read' ? files.read({ path }, controller.signal) : files.write({ path, content: 'must not be written' }, controller.signal);
    observed = work.then(() => ({ completed: true }), error => ({ error }));
    await started;
    controller.abort();
    const settled = await Promise.race([observed, delay(200).then(() => ({ stalled: true }))]);
    assert.equal(settled.error?.name, 'AbortError', 'Metadata cancellation must settle without waiting for its filesystem reply.');
    const countAtAbort = calls.length;
    release();
    await nextTurn(); await nextTurn();
    assert.equal(calls.length, countAtAbort, 'A late metadata reply must not resume file operations.');
    assert.equal(calls.includes('open'), false);
    assert.equal(calls.includes('rename'), false);
    assert.equal(readFileSync(path, 'utf8'), 'original');
    assert.deepEqual(readdirSync(root), ['existing.txt']);
  } finally {
    release();
    if (observed) await observed;
    for (const [name, original] of Object.entries(originals)) fs[name] = original;
    syncBuiltinESMExports();
  }
});

test('Windows atomic file publication retries transient sharing failures without changing its target', { skip: process.platform !== 'win32' }, async t => {
  const { root, files } = fixture(t);
  const path = join(root, 'file.txt'); writeFileSync(path, 'original');
  const original = fs.rename;
  const targets = [];
  try {
    fs.rename = async (...args) => {
      targets.push(args[1]);
      if (targets.length < 3) throw Object.assign(new Error('Injected transient sharing violation.'), { code: 'EPERM' });
      return original(...args);
    };
    syncBuiltinESMExports();
    assert.equal(await files.write({ path, content: 'published after retry' }), null);
    assert.deepEqual(targets, [path, path, path]);
    assert.equal(readFileSync(path, 'utf8'), 'published after retry');
    assert.deepEqual(readdirSync(root), ['file.txt']);
  } finally { fs.rename = original; syncBuiltinESMExports(); }
});

test('cancelling Windows sharing retries leaves the original file and removes staging data', { skip: process.platform !== 'win32' }, async t => {
  const { root, files } = fixture(t);
  const path = join(root, 'file.txt'); writeFileSync(path, 'original');
  const original = fs.rename;
  const controller = new AbortController();
  let notify, calls = 0;
  const attempted = new Promise(resolve => { notify = resolve; });
  let observed;
  try {
    fs.rename = async () => {
      calls++; notify();
      throw Object.assign(new Error('Injected transient sharing violation.'), { code: 'EBUSY' });
    };
    syncBuiltinESMExports();
    observed = files.write({ path, content: 'must not be published' }, controller.signal).then(() => undefined, error => error);
    await attempted;
    controller.abort();
    assert.equal((await observed)?.name, 'AbortError');
    assert.equal(calls, 1, 'Cancellation must prevent another publication attempt.');
    assert.equal(readFileSync(path, 'utf8'), 'original');
    assert.deepEqual(readdirSync(root), ['file.txt']);
  } finally { controller.abort(); if (observed) await observed; fs.rename = original; syncBuiltinESMExports(); }
});
