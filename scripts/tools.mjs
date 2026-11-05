import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

export const root = fileURLToPath(new URL('../', import.meta.url));
export const binary = path.join(root, 'build', process.platform === 'win32' ? 'council.exe' : 'council');
export function run(executable, args, options = {}) {
  const result = spawnSync(executable, args, { cwd: root, stdio: 'inherit', shell: false, ...options });
  if (result.error) throw new Error(`Unable to run ${path.basename(executable)}: ${result.error.code ?? 'launch failed'}`);
  if (result.status !== 0) process.exit(result.status ?? 1);
}
export function nodeScript(file, args = []) { run(process.execPath, [path.join(root, file), ...args]); }
