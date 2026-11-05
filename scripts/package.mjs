import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstat, mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const targets = { win32: 'windows', darwin: 'darwin', linux: 'linux' };
const architectures = { x64: 'amd64', arm64: 'arm64' };
export const version = '2.0.0-dev';

// Never replace this list with a directory walk or a source-tree archive.
const documents = Object.freeze([
  ['README.md', 'scripts/packagearchive/README.md'],
  ['LICENSE', 'LICENSE'],
  ['third-party/brand-assets.md', 'third-party/brand-assets.md'],
  ['third-party/ejupilabs-icons/LICENSE', 'third-party/ejupilabs-icons/LICENSE'],
  ['third-party/go-NOTICES.txt', 'third-party/go-NOTICES.txt'],
]);

export function sha256(data) {
  return createHash('sha256').update(data).digest('hex');
}

async function regularFile(relativePath) {
  const source = path.join(root, relativePath);
  if (!(await lstat(source)).isFile()) throw new Error(`Release input must be a regular file: ${relativePath}`);
  return readFile(source);
}

export async function packagePreview({ outputDirectory = path.join(root, 'build/releases'), build = true } = {}) {
  const platform = targets[process.platform];
  const architecture = architectures[process.arch];
  if (!platform || !architecture) throw new Error('Native preview packaging supports Windows, macOS and Linux on AMD64/ARM64.');
  if ((process.env.GOOS && process.env.GOOS !== platform) || (process.env.GOARCH && process.env.GOARCH !== architecture)) {
    throw new Error('Native preview packaging requires the current host GOOS and GOARCH.');
  }
  if (build) execFileSync(process.execPath, [path.join(root, 'scripts/build.mjs')], { cwd: root, stdio: 'inherit', shell: false });

  const executable = process.platform === 'win32' ? 'council.exe' : 'council';
  const binary = path.join(root, 'build', executable);
  const metadata = JSON.parse(execFileSync(binary, ['--version', '--json'], { cwd: root, encoding: 'utf8', shell: false }));
  if (metadata?.data?.appVersion !== version || metadata?.data?.apiVersion !== 1 || metadata.error) {
    throw new Error('Built binary does not match the preview application and API version.');
  }
  const entries = [{ name: executable, data: await regularFile(`build/${executable}`), mode: 0o755 }];
  for (const [name, source] of documents) entries.push({ name, data: await regularFile(source), mode: 0o644 });
  entries.sort((left, right) => left.name.localeCompare(right.name, 'en'));
  const checksums = entries.map(entry => `${sha256(entry.data)}  ${entry.name}\n`).join('');
  entries.push({ name: 'SHA256SUMS.txt', data: Buffer.from(checksums), mode: 0o644 });

  await mkdir(outputDirectory, { recursive: true });
  const archiveName = `council-${version}-${platform}-${architecture}.zip`;
  const archive = path.join(outputDirectory, archiveName);
  const temporary = `${archive}.${process.pid}.tmp`;
  const checksum = `${archive}.sha256`;
  const checksumTemporary = `${checksum}.${process.pid}.tmp`;
  try {
    execFileSync('go', ['run', './scripts/packagearchive', temporary], {
      cwd: root,
      input: JSON.stringify(entries.map(entry => ({ ...entry, data: entry.data.toString('base64') }))),
      stdio: ['pipe', 'inherit', 'inherit'],
      shell: false,
      env: { ...process.env, CGO_ENABLED: '0' },
    });
    await writeFile(checksumTemporary, `${sha256(await readFile(temporary))}  ${archiveName}\n`, { flag: 'wx' });
    await rename(temporary, archive);
    await rename(checksumTemporary, checksum);
    return { archive, checksum, archiveName, entries: entries.map(entry => entry.name) };
  } finally {
    await rm(temporary, { force: true });
    await rm(checksumTemporary, { force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.length !== 2) throw new Error('Usage: node scripts/package.mjs');
  const result = await packagePreview();
  console.log(`Created ${result.archiveName} and ${result.archiveName}.sha256 in build/releases.`);
}
