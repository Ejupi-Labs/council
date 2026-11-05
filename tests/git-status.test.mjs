import test from 'node:test';
import assert from 'node:assert/strict';
import { gitStatusDiscovery, gitStatusUntrackedPaths, GIT_STATUS_COMPONENT_BYTES, GIT_STATUS_OUTPUT_BYTES } from '../dist/git-status.js';

const hash = 'a'.repeat(40), zero = '0'.repeat(40), nul = Buffer.from([0]);
const ordinary = (path, sub = 'N...') => Buffer.concat([Buffer.from(`1 .M ${sub} 100644 100644 100644 ${hash} ${hash} `), Buffer.from(path), nul]);
const rename = (path, original, sub = 'N...', score = 'R100') => Buffer.concat([Buffer.from(`2 R. ${sub} 100644 100644 100644 ${hash} ${hash} ${score} `), Buffer.from(path), nul, Buffer.from(original), nul]);
const untracked = path => Buffer.concat([Buffer.from('? '), Buffer.from(path), nul]);
const unmerged = (path, sub = 'N...', fileMode = '100644') => Buffer.from(`u UU ${sub} ${fileMode} ${fileMode} ${fileMode} ${fileMode} ${hash} ${hash} ${zero} ${path}\0`);

test('status discovery copies exact raw pathname bytes and excludes tracked, ignored and header records', () => {
  const raw = Buffer.from([0xff, 0x80, 32, 9, 10, 34, 92]), literal = Buffer.from('nested new/雪-é.txt');
  const output = Buffer.concat([Buffer.from('# future.header value\0'), ordinary('? tracked name'), unmerged('conflicted'), Buffer.from('! ignored\0'), untracked(raw), untracked(literal)]);
  const result = gitStatusUntrackedPaths(output);
  assert.deepEqual(result, Buffer.concat([raw, nul, literal, nul]));
  assert.equal(Buffer.from(raw.toString('utf8')).equals(raw), false);
  output.fill(0); assert.deepEqual(result, Buffer.concat([raw, nul, literal, nul]), 'Returned pathspec bytes do not alias the original status.');
});

test('rename and copy original path records are consumed even when they look like untracked records', () => {
  const origin = Buffer.concat([Buffer.from('? original-'), Buffer.from([0xff, 0x80])]);
  const output = Buffer.concat([rename('renamed', origin), rename('copied', '? another', 'N...', 'C75'), untracked('actual')]);
  assert.deepEqual(gitStatusUntrackedPaths(output), Buffer.from('actual\0'));
});

test('dirty submodule flags fail before returning paths while committed gitlink changes remain exportable', () => {
  for (const sub of ['S.M.', 'S..U', 'SCM.', 'SC.U', 'SCMU']) {
    for (const tracked of [ordinary('vendor', sub), rename('vendor', '? original', sub)]) {
      assert.throws(() => gitStatusUntrackedPaths(Buffer.concat([untracked('new'), tracked])), /Uncommitted submodule files cannot be preserved/);
    }
  }
  assert.deepEqual(gitStatusUntrackedPaths(Buffer.concat([ordinary('vendor', 'SC..'), untracked('new')])), Buffer.from('new\0'));
});

test('unmerged gitlinks reject dirty child evidence while clean references and regular conflicts remain parseable', () => {
  for (const sub of ['S.M.', 'S..U', 'SCM.', 'SC.U', 'SCMU']) {
    const status = Buffer.concat([untracked('must-not-return'), unmerged('conflicted submodule', sub, '160000')]);
    assert.throws(() => gitStatusDiscovery(status), /Uncommitted submodule files cannot be preserved/);
  }
  const clean = Buffer.concat([unmerged('clean reference', 'SC..', '160000'), unmerged('ordinary conflict'), untracked('actual')]);
  assert.deepEqual(gitStatusDiscovery(clean), { untracked: Buffer.from('actual\0'), gitlinks: Buffer.alloc(0) });
  for (const malformed of [unmerged('missing-nul').subarray(0, -1), Buffer.from(`u UU N... 100644 100644 100644 ${hash} ${hash} ${zero} broken\0`), unmerged('bad-mode', 'N...', '160008')]) {
    assert.throws(() => gitStatusDiscovery(Buffer.concat([untracked('must-not-return'), malformed])), /Malformed or incomplete/);
  }
});

test('independent component budgets accept combined output beyond either single limit without shortening data', () => {
  const tracked = Buffer.concat([ordinary('tracked'), rename('new-name', '? original-name')]);
  const raw = Buffer.from([0xff, 0x80, 10, 32, 97]), paths = untracked(raw), combined = Buffer.concat([tracked, paths]);
  const limits = { trackedBytes: tracked.length, untrackedBytes: raw.length + 1 };
  assert.ok(combined.length > Math.max(limits.trackedBytes, limits.untrackedBytes));
  assert.deepEqual(gitStatusUntrackedPaths(combined, limits), Buffer.concat([raw, nul]));
  assert.throws(() => gitStatusUntrackedPaths(combined, { ...limits, trackedBytes: tracked.length - 1 }), /tracked metadata exceeds/);
  assert.throws(() => gitStatusUntrackedPaths(combined, { ...limits, untrackedBytes: raw.length }), /untracked paths exceeds/);
  assert.equal(GIT_STATUS_OUTPUT_BYTES, 3 * GIT_STATUS_COMPONENT_BYTES);
});

test('rename origins and every NUL remain part of the tracked byte budget', () => {
  const output = rename('target', '? original');
  assert.deepEqual(gitStatusUntrackedPaths(output, { trackedBytes: output.length, untrackedBytes: 0 }), Buffer.alloc(0));
  assert.throws(() => gitStatusUntrackedPaths(output, { trackedBytes: output.length - 1, untrackedBytes: 0 }), /tracked metadata/);
  assert.deepEqual(gitStatusUntrackedPaths(Buffer.alloc(0), { trackedBytes: 0, untrackedBytes: 0 }), Buffer.alloc(0));
});

test('malformed framing, malformed metadata and extra rename records fail without partial path output', () => {
  const invalid = [
    Buffer.from('? unfinished'), Buffer.from('\0'), Buffer.from('? \0'), Buffer.from('x unknown\0'),
    Buffer.from('1 .M N... short\0'), ordinary('x').subarray(0, -2),
    Buffer.from(`1 ?? N... 100644 100644 100644 ${hash} ${hash} path\0`),
    Buffer.from(`1 .M Nxxx 100644 100644 100644 ${hash} ${hash} path\0`),
    Buffer.from(`1 .M N... 100648 100644 100644 ${hash} ${hash} path\0`),
    Buffer.from(`1 .M N... 100644 100644 100644 z${hash.slice(1)} ${hash} path\0`),
    Buffer.from(`1 .M N... 100644 100644 100644 ${hash} ${hash} \0`),
    rename('new', 'old', 'N...', 'R101'), rename('new', ''),
    rename('new', 'old').subarray(0, -4),
    Buffer.concat([rename('new', 'old'), Buffer.from('extra-original\0')]),
  ];
  for (const value of invalid) assert.throws(() => gitStatusUntrackedPaths(Buffer.concat([untracked('must-not-be-returned'), value])), /Malformed or incomplete/);
});

test('component limits can be reduced but cannot disable the production bounds', () => {
  for (const value of [-1, 0.5, NaN, Infinity, GIT_STATUS_COMPONENT_BYTES + 1]) {
    assert.throws(() => gitStatusUntrackedPaths(Buffer.alloc(0), { trackedBytes: value }), /component limits/);
    assert.throws(() => gitStatusUntrackedPaths(Buffer.alloc(0), { untrackedBytes: value }), /component limits/);
  }
});

test('intent gitlink candidates retain raw paths without misclassifying normal intent files or full gitlinks', () => {
  const raw = Buffer.concat([Buffer.from('nested-'), Buffer.from([0xff, 0x80])]);
  const intent = Buffer.concat([Buffer.from(`1 .A S... 000000 000000 160000 ${zero} ${zero} `), raw, nul]);
  const normalIntent = Buffer.from(`1 .A N... 000000 000000 100644 ${zero} ${zero} ordinary\0`);
  const fullGitlink = Buffer.from(`1 A. S... 000000 160000 160000 ${zero} ${hash} staged\0`);
  const cleanReference = Buffer.from(`1 .M SC.. 160000 160000 160000 ${hash} ${hash} registered\0`);
  const status = Buffer.concat([untracked('plain'), intent, normalIntent, fullGitlink, cleanReference, rename('file', '? false-candidate/')]);
  const found = gitStatusDiscovery(status);
  assert.deepEqual(found.untracked, Buffer.from('plain\0'));
  assert.deepEqual(found.gitlinks, Buffer.concat([raw, nul]));
  status.fill(0); assert.deepEqual(found.gitlinks, Buffer.concat([raw, nul]));
});

test('intent candidate bytes remain within the original tracked component budget', () => {
  const intent = Buffer.from(`1 .A S... 000000 000000 160000 ${zero} ${zero} child\0`);
  const found = gitStatusDiscovery(intent, { trackedBytes: intent.length, untrackedBytes: 0 });
  assert.equal(found.untracked.length, 0); assert.deepEqual(found.gitlinks, Buffer.from('child\0'));
  assert.throws(() => gitStatusDiscovery(intent, { trackedBytes: intent.length - 1, untrackedBytes: 0 }), /tracked metadata/);
  const ordinary = Buffer.from(`1 .M N... 100644 100644 100644 ${zero} ${zero} filename\0`);
  assert.equal(gitStatusDiscovery(ordinary).gitlinks.length, 0, 'Zero hashes alone never select or reject a candidate.');
  const sha256 = Buffer.from(`1 .A S... 000000 000000 160000 ${'0'.repeat(64)} ${'0'.repeat(64)} sha256-child\0`);
  assert.deepEqual(gitStatusDiscovery(sha256).gitlinks, Buffer.from('sha256-child\0'));
});

test('working gitlink candidates include regular and symlink replacements and rename/copy targets with nonzero hashes', () => {
  const regular = Buffer.from(`1 .T SC.. 100644 100644 160000 ${hash} ${hash} regular\0`);
  const symlink = Buffer.from(`1 .T SC.. 120000 120000 160000 ${hash} ${hash} symlink\0`);
  const renamed = Buffer.from(`2 RT SC.. 100644 100644 160000 ${hash} ${hash} R100 renamed target\0? old\0`);
  const copied = Buffer.from(`2 CT SC.. 100644 100644 160000 ${hash} ${hash} C75 copied target\0? another old\0`);
  const found = gitStatusDiscovery(Buffer.concat([regular, symlink, renamed, copied, untracked('plain')]));
  assert.deepEqual(found.gitlinks, Buffer.from('regular\0symlink\0renamed target\0copied target\0'));
  assert.deepEqual(found.untracked, Buffer.from('plain\0'));
});
