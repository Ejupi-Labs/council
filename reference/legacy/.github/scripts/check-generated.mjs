import { spawnSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { sanitizedGitEnvironment } from '../../dist/git-env.js';

const repository = fileURLToPath(new URL('../../', import.meta.url));

export function checkGenerated(directory = repository, environment = process.env) {
  const result = spawnSync('git', ['status', '--porcelain=v1', '--untracked-files=all', '--ignored=matching', '--', 'dist', 'schemas'], {
    cwd: directory, shell: false, windowsHide: true, encoding: 'utf8', timeout: 10_000, maxBuffer: 2 * 1024 * 1024,
    env: sanitizedGitEnvironment(environment),
  });
  if (result.error || result.status !== 0) throw new Error('Unable to verify committed generated files with Git.');
  if (result.stdout.trim()) throw new Error(`Generated files must be committed and current:\n${result.stdout.trimEnd()}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 2) throw new Error('The generated-file check accepts no command-line options.');
    checkGenerated();
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
