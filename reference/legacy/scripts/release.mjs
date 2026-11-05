import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, isAbsolute, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { crc32, deflateRawSync } from 'node:zlib';
import { VERSION } from '../dist/version.js';

const root = fileURLToPath(new URL('../', import.meta.url));
const destination = join(root, '.council', 'releases');
mkdirSync(destination, { recursive: true });
const npm = [process.env.npm_execpath, join(dirname(process.execPath), 'node_modules', 'npm', 'bin', 'npm-cli.js'), resolve(dirname(process.execPath), '..', 'lib', 'node_modules', 'npm', 'bin', 'npm-cli.js')].find(path => path && existsSync(path));
if (!npm) throw new Error('Run release:local through npm, or install npm beside Node.');
const packed = JSON.parse(execFileSync(process.execPath, [npm, 'pack', '--ignore-scripts', '--json', '--pack-destination', destination], { cwd: root, encoding: 'utf8', windowsHide: true }))[0];
if (packed.version !== VERSION) throw new Error('Release metadata version mismatch.');

// Standard ZIP with UTF-8 filenames, fixed timestamps, CRC-32 and raw DEFLATE.
// No platform tools or third-party runtime packages are required.
const records = [], directory = [];
let offset = 0;
for (const file of packed.files.toSorted((a, b) => Buffer.compare(Buffer.from(a.path), Buffer.from(b.path)))) {
  const path = file.path.replaceAll('\\', '/');
  if (isAbsolute(path) || path.split('/').some(part => part === '..') || /^(node_modules|\.council|\.serena|tests)\//.test(path)) throw new Error('Unsafe or private release path.');
  const name = Buffer.from(path, 'utf8');
  const data = readFileSync(join(root, path));
  const compressed = deflateRawSync(data, { level: 9 });
  const checksum = crc32(data) >>> 0;
  const local = Buffer.alloc(30);
  local.writeUInt32LE(0x04034b50, 0); local.writeUInt16LE(20, 4);
  local.writeUInt16LE(0x0800, 6); local.writeUInt16LE(8, 8); local.writeUInt16LE(0x0021, 12);
  local.writeUInt32LE(checksum, 14); local.writeUInt32LE(compressed.length, 18); local.writeUInt32LE(data.length, 22); local.writeUInt16LE(name.length, 26);
  records.push(local, name, compressed);
  const central = Buffer.alloc(46);
  central.writeUInt32LE(0x02014b50, 0); central.writeUInt16LE(0x0314, 4); central.writeUInt16LE(20, 6);
  central.writeUInt16LE(0x0800, 8); central.writeUInt16LE(8, 10); central.writeUInt16LE(0x0021, 14);
  central.writeUInt32LE(checksum, 16); central.writeUInt32LE(compressed.length, 20); central.writeUInt32LE(data.length, 24); central.writeUInt16LE(name.length, 28);
  central.writeUInt32LE((0o100644 << 16) >>> 0, 38); central.writeUInt32LE(offset, 42);
  directory.push(central, name);
  offset += local.length + name.length + compressed.length;
}
const centralSize = directory.reduce((size, value) => size + value.length, 0);
const end = Buffer.alloc(22);
end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(packed.files.length, 8); end.writeUInt16LE(packed.files.length, 10);
end.writeUInt32LE(centralSize, 12); end.writeUInt32LE(offset, 16);
const zipName = `council-plugin-${VERSION}.zip`;
writeFileSync(join(destination, zipName), Buffer.concat([...records, ...directory, end]));
const artifacts = [packed.filename, zipName].map(filename => {
  const data = readFileSync(join(destination, filename));
  return { filename, bytes: data.length, sha256: createHash('sha256').update(data).digest('hex') };
});
writeFileSync(join(destination, 'SHA256SUMS'), artifacts.map(file => `${file.sha256}  ${file.filename}`).join('\n') + '\n');
const release = { version: VERSION, productionDependencies: 0, artifacts };
writeFileSync(join(destination, 'release.json'), JSON.stringify(release, null, 2) + '\n');
process.stdout.write(JSON.stringify({ ...release, directory: destination }, null, 2) + '\n');
