import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
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
