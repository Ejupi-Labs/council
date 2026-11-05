// Refresh with `node scripts/packagearchive/notices.mjs`; review the resulting diff.
// --check verifies the committed text without writing. Only known license files are read.
import { execFileSync } from 'node:child_process';
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../', import.meta.url));
const modules = [
  ['github.com/dustin/go-humanize', 'v1.1.0', ['LICENSE']],
  ['github.com/google/uuid', 'v1.6.0', ['LICENSE']],
  ['github.com/mattn/go-isatty', 'v0.0.24', ['LICENSE']],
  ['github.com/ncruces/go-strftime', 'v1.1.0', ['LICENSE']],
  ['github.com/remyoudompheng/bigfft', 'v0.0.0-20230129092748-24d4a6f8daec', ['LICENSE']],
  ['golang.org/x/sys', 'v0.49.0', ['LICENSE', 'PATENTS']],
  ['modernc.org/libc', 'v1.79.0', ['AUTHORS', 'LICENSE', 'LICENSE-3RD-PARTY.md']],
  ['modernc.org/mathutil', 'v1.7.1', ['AUTHORS', 'LICENSE']],
  ['modernc.org/memory', 'v1.12.1', ['AUTHORS', 'LICENSE', 'LICENSE-GO', 'LICENSE-MMAP-GO']],
  ['modernc.org/sqlite', 'v1.61.1', ['AUTHORS', 'LICENSE', 'LICENSE-3RD-PARTY.md', 'LICENSE-SQLITE', 'LICENSE-SQLITE_VEC']],
];

export async function compileNotices() {
  const environment = JSON.parse(execFileSync('go', ['env', '-json', 'GOROOT', 'GOMODCACHE', 'GOVERSION'], { cwd: root, encoding: 'utf8', shell: false }));
  if (environment.GOVERSION !== 'go1.27.3') throw new Error('Refresh notices with the selected Go 1.27.3 toolchain.');
  let text = 'Council native preview: Go and dependency notices\n\n'
    + 'Toolchain: Go 1.27.3. SQLite runtime: 3.54.1 via modernc.org/sqlite v1.61.1.\n'
    + 'The pinned runtime module inventory below is the union for Windows, macOS and\n'
    + 'Linux on AMD64/ARM64. Full upstream notice texts follow under relative source\n'
    + 'identifiers. Aggregate upstream notices also describe test/build-only material;\n'
    + 'retaining them does not claim that every listed component is linked. No source\n'
    + 'code, machine-local path, vendor state or credentials are included here.\n\n';
  async function append(identifier, filename) {
    const original = await readFile(filename, 'utf8');
    text += `===== ${identifier} =====\n\n${original}${original.endsWith('\n') ? '' : '\n'}\n`;
  }
  for (const filename of ['LICENSE', 'PATENTS']) await append(`Go 1.27.3/${filename}`, path.join(environment.GOROOT, filename));
  for (const module of ['crypto', 'net', 'sys', 'text']) {
    for (const filename of ['LICENSE', 'PATENTS']) {
      const identifier = `src/vendor/golang.org/x/${module}/${filename}`;
      await append(`Go 1.27.3/${identifier}`, path.join(environment.GOROOT, identifier));
    }
  }
  for (const [module, version, filenames] of modules) {
    for (const filename of filenames) await append(`${module}@${version}/${filename}`, path.join(environment.GOMODCACHE, `${module}@${version}`, filename));
  }
  return text;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.slice(2).some(argument => argument !== '--check') || process.argv.length > 3) throw new Error('Usage: node scripts/packagearchive/notices.mjs [--check]');
  const filename = path.join(root, 'third-party/go-NOTICES.txt');
  const text = await compileNotices();
  if (process.argv.includes('--check')) {
    if (await readFile(filename, 'utf8') !== text) throw new Error('Go dependency notices differ; refresh and review the notice inventory.');
  } else {
    await writeFile(filename, text);
    console.log('Refreshed third-party/go-NOTICES.txt from pinned public dependency distributions.');
  }
}
