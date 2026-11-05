import { mkdir, cp, readdir, lstat, realpath, unlink } from 'node:fs/promises';
import path from 'node:path';
import { root, binary, run, nodeScript } from './tools.mjs';

await mkdir(path.join(root, 'internal/webui/dist'), { recursive: true });
nodeScript('node_modules/typescript/bin/tsc', ['-p', 'ui/tsconfig.json']);
for (const item of await readdir(path.join(root, 'ui/src'), { withFileTypes: true })) {
  if (item.name.endsWith('.ts')) continue;
  await cp(path.join(root, 'ui/src', item.name), path.join(root, 'internal/webui/dist', item.name), { recursive: true });
}
// Retire stale generated files only. Never recursively remove source or runtime state.
const sourceDirectory = path.join(root, 'ui/src');
const outputDirectory = path.join(root, 'internal/webui/dist');
async function files(directory, prefix = '') {
  const result = [];
  for (const item of await readdir(directory, { withFileTypes: true })) {
    const relative = path.join(prefix, item.name);
    const absolute = path.join(directory, item.name);
    if ((await lstat(absolute)).isSymbolicLink()) throw new Error('UI build inputs and outputs must not be symlinks.');
    if (item.isDirectory()) result.push(...await files(absolute, relative));
    else if (item.isFile()) result.push(relative);
    else throw new Error('UI build entries must be ordinary files or directories.');
  }
  return result;
}
const expected = new Set((await files(sourceDirectory)).filter(file => !file.endsWith('.d.ts')).map(file => file.endsWith('.ts') ? file.slice(0, -3) + '.js' : file));
const resolvedOutput = await realpath(outputDirectory);
if (path.relative(root, resolvedOutput) !== path.join('internal', 'webui', 'dist')) throw new Error('Unexpected generated UI directory.');
for (const file of await files(outputDirectory)) {
  if (expected.has(file)) continue;
  const target = path.resolve(outputDirectory, file);
  const relative = path.relative(resolvedOutput, target);
  if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error('Stale UI output escapes the generated directory.');
  await unlink(target);
}
await mkdir(path.join(root, 'build'), { recursive: true });
run('go', ['build', '-trimpath', '-o', binary, './cmd/council'], { env: { ...process.env, CGO_ENABLED: '0' } });
console.log(`Built ${binary}`);
