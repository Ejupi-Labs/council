import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { chmod, mkdir, mkdtemp, readFile, realpath, lstat, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { createInterface } from 'node:readline';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { inflateRawSync } from 'node:zlib';
import { packagePreview, sha256, version } from './package.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const executable = process.platform === 'win32' ? 'council.exe' : 'council';
const expected = [executable, 'README.md', 'LICENSE', 'third-party/brand-assets.md',
  'third-party/ejupilabs-icons/LICENSE', 'third-party/go-NOTICES.txt', 'SHA256SUMS.txt'].sort();

// Read the portable ZIP central directory independently of the Go writer. A generic
// archive extractor is deliberately not a release user's runtime dependency.
function readZip(data) {
  const end = data.length - 22;
  assert.equal(data.readUInt32LE(end), 0x06054b50, 'archive must have an uncommented ZIP end record');
  const count = data.readUInt16LE(end + 10);
  let cursor = data.readUInt32LE(end + 16);
  const entries = new Map();
  for (let i = 0; i < count; i++) {
    assert.equal(data.readUInt32LE(cursor), 0x02014b50);
    const method = data.readUInt16LE(cursor + 10);
    const compressedSize = data.readUInt32LE(cursor + 20);
    const size = data.readUInt32LE(cursor + 24);
    const nameLength = data.readUInt16LE(cursor + 28);
    const extraLength = data.readUInt16LE(cursor + 30);
    const commentLength = data.readUInt16LE(cursor + 32);
    const mode = data.readUInt32LE(cursor + 38) >>> 16;
    const offset = data.readUInt32LE(cursor + 42);
    const name = data.subarray(cursor + 46, cursor + 46 + nameLength).toString('utf8');
    assert.ok(expected.includes(name), 'archive contains only allowlisted runtime and notice files');
    assert.ok(!entries.has(name), 'archive must not contain duplicate filenames');
    assert.equal(data.readUInt32LE(offset), 0x04034b50);
    assert.equal(data.readUInt16LE(offset + 8), method);
    const start = offset + 30 + data.readUInt16LE(offset + 26) + data.readUInt16LE(offset + 28);
    const compressed = data.subarray(start, start + compressedSize);
    assert.equal(method, 8, 'archive uses portable deflate compression');
    const contents = inflateRawSync(compressed);
    assert.equal(contents.length, size);
    entries.set(name, { contents, mode });
    cursor += 46 + nameLength + extraLength + commentLength;
  }
  assert.deepEqual([...entries.keys()].sort(), expected);
  return entries;
}

function privateEnvironment(home, emptyPath) {
  const environment = { ...process.env };
  // Windows environment names are case-insensitive, unlike JavaScript object keys.
  for (const key of Object.keys(environment)) {
    if (['path', 'home', 'userprofile', 'homedrive', 'homepath', 'node_options'].includes(key.toLowerCase())) delete environment[key];
  }
  return { ...environment, PATH: emptyPath, HOME: home, USERPROFILE: home, GOPROXY: 'off', GOSUMDB: 'off' };
}

async function stop(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  const exited = once(child, 'exit');
  child.kill('SIGTERM');
  await exited;
}

test('native preview has exact contents, complete notices and an independently verifiable checksum', { timeout: 120_000 }, async t => {
  const temporary = await mkdtemp(path.join(os.tmpdir(), 'council-package-'));
  let child;
  t.after(async () => {
    if (child) await stop(child);
    const resolved = await realpath(temporary);
    const temporaryRoot = await realpath(os.tmpdir());
    if (path.relative(temporaryRoot, path.dirname(resolved)) !== '' || !path.basename(resolved).startsWith('council-package-') || (await lstat(temporary)).isSymbolicLink()) {
      throw new Error('Refusing cleanup outside the created package fixture.');
    }
    await rm(resolved, { recursive: true, force: true });
  });
  execFileSync('go', ['test', './scripts/packagearchive'], { cwd: root, shell: false, stdio: 'pipe' });
  const release = await packagePreview({ outputDirectory: path.join(temporary, 'releases') });
  const data = await readFile(release.archive);
  assert.equal(await readFile(release.checksum, 'utf8'), `${sha256(data)}  ${release.archiveName}\n`);
  assert.match(release.archiveName, /^council-2\.0\.0-dev-(windows|darwin|linux)-(amd64|arm64)\.zip$/);
  const entries = readZip(data);
  const manifest = entries.get('SHA256SUMS.txt').contents.toString('utf8');
  const records = manifest.trimEnd().split('\n').map(line => line.match(/^([a-f0-9]{64})  (.+)$/));
  assert.ok(records.every(Boolean), 'each checksum record has one exact relative filename');
  assert.deepEqual(records.map(record => record[2]).sort(), expected.filter(name => name !== 'SHA256SUMS.txt'));
  for (const [, digest, name] of records) assert.equal(sha256(entries.get(name).contents), digest, `checksum matches ${name}`);
  assert.equal(entries.get(executable).mode & 0o777, 0o755, 'Unix extraction preserves executable permissions');
  assert.ok(entries.get(executable).contents.equals(await readFile(path.join(root, 'build', executable))), 'package contains the freshly built native binary');
  const notices = entries.get('third-party/go-NOTICES.txt').contents.toString('utf8');
  const goModule = await readFile(path.join(root, 'go.mod'), 'utf8');
  for (const [, module, pinned] of goModule.matchAll(/^\s+([^\s]+) (v[^\s]+)(?: \/\/ indirect)?$/gm)) {
    assert.ok(notices.includes(`===== ${module}@${pinned}/LICENSE =====`), `license inventory covers ${module}@${pinned}`);
  }
  for (const notice of ['Go 1.27.3/LICENSE', 'Go 1.27.3/PATENTS', 'LICENSE-3RD-PARTY.md', 'LICENSE-SQLITE', 'LICENSE-MMAP-GO']) {
    assert.ok(notices.includes(notice), `retained upstream notice ${notice}`);
  }
  assert.match(entries.get('README.md').contents.toString('utf8'), /Task execution is unavailable in this preview/);
  for (const { contents } of entries.values()) {
    for (const forbidden of [root.replaceAll('\\', '/').replace(/\/$/, ''), root.replaceAll('/', '\\').replace(/\\$/, '')]) {
      assert.equal(contents.includes(Buffer.from(forbidden)), false, 'release contains no developer workspace path');
    }
  }
  const repeat = await packagePreview({ outputDirectory: path.join(temporary, 'repeat'), build: false });
  assert.equal(sha256(await readFile(repeat.archive)), sha256(data), 'fixed input produces reproducible ZIP bytes');

  const extracted = path.join(temporary, 'extracted');
  for (const [name, { contents, mode }] of entries) {
    const filename = path.join(extracted, name);
    await mkdir(path.dirname(filename), { recursive: true });
    await writeFile(filename, contents);
    await chmod(filename, mode & 0o777);
  }
  const elsewhere = path.join(temporary, 'unrelated-working-directory');
  const home = path.join(temporary, 'isolated-home');
  const emptyPath = path.join(temporary, 'empty-path');
  for (const directory of [elsewhere, home, emptyPath]) await mkdir(directory);
  const environment = privateEnvironment(home, emptyPath);
  const binary = path.join(extracted, executable);
  const options = { cwd: elsewhere, env: environment, shell: false, encoding: 'utf8', timeout: 10_000 };
  const metadata = JSON.parse(execFileSync(binary, ['--version', '--json'], options));
  assert.equal(metadata.data.appVersion, version);
  assert.equal(metadata.data.apiVersion, 1);
  const policy = JSON.parse(execFileSync(binary, ['policy', 'get', '--json'], options));
  assert.ok(policy.data && !policy.error, 'extracted binary can initialize private storage without Node, Go or Git on PATH');

  child = spawn(binary, ['ui', '--no-open', '--json'], { cwd: elsewhere, env: environment, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
  const lines = createInterface({ input: child.stdout });
  const launch = await Promise.race([
    once(lines, 'line').then(([line]) => JSON.parse(line)),
    once(child, 'exit').then(() => { throw new Error('Extracted preview stopped before reporting UI startup.'); }),
  ]);
  lines.close();
  assert.ok(launch.data?.url && !launch.error, 'extracted binary reports native UI startup');
  const url = new URL(launch.data.url);
  assert.equal(url.hostname, '127.0.0.1');
  const response = await fetch(url.origin, { signal: AbortSignal.timeout(10_000) });
  assert.equal(response.status, 200);
  const html = await response.text();
  assert.match(html, /Ejupi Labs/);
  const resources = [...html.matchAll(/(?:src|href)="([^"]+)"/g)].map(match => match[1]).filter(value => !value.startsWith('#'));
  assert.ok(resources.length > 0, 'embedded page identifies its embedded public resources');
  for (const resource of resources) {
    const asset = new URL(resource, url.origin);
    assert.equal(asset.origin, url.origin, 'runtime resources remain local and embedded');
    assert.equal((await fetch(asset, { signal: AbortSignal.timeout(10_000) })).status, 200, 'extracted preview serves its embedded assets');
  }
  await stop(child);
});
