import { spawnSync } from 'node:child_process';
import { realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { root, run } from './tools.mjs';

const enginePackage = 'github.com/Ejupi-Labs/council/v2/internal/engine';
const packagePatterns = ['./cmd/...', './internal/...', './scripts/...'];

export function listedCases(output) {
  const names = [];
  for (const raw of output.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || /^(ok|\?)\s/.test(line)) continue;
    if (!/^(Test|Example|Fuzz)[\p{L}\p{N}_]*$/u.test(line)) throw new Error('Unexpected Go test listing output; no cases were discarded.');
    if (names.includes(line)) throw new Error('Duplicate Go case in the discovered list.');
    names.push(line);
  }
  if (!names.length) throw new Error('The Engine package has no discovered test cases.');
  return names;
}

export function caseBatches(names, size = 8) {
  if (!Number.isSafeInteger(size) || size < 1 || !names.length || new Set(names).size !== names.length) throw new Error('Supply unique complete cases and a positive batch size.');
  const batches = [];
  for (let start = 0; start < names.length; start += size) {
    const cases = names.slice(start, start + size);
    const pattern = '^(' + cases.map(name => name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|') + ')$';
    batches.push({ cases, pattern });
  }
  return batches;
}

function captureGo(args) {
  const result = spawnSync('go', args, { cwd: root, shell: false, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
  if (result.error || result.status !== 0) throw new Error('Go test discovery failed: ' + (result.stderr || result.error?.message || result.status));
  return result.stdout;
}

export function testGo({ race = false } = {}) {
  const packages = captureGo(['list', ...packagePatterns]).trim().split(/\r?\n/).filter(Boolean);
  if (packages.filter(name => name === enginePackage).length !== 1) throw new Error('Engine package discovery is incomplete or ambiguous.');
  const flags = ['test', '-p', '1', ...(race ? ['-race'] : [])];
  const others = packages.filter(name => name !== enginePackage);
  if (others.length) run('go', [...flags, '-count=1', '-v', ...others]);

  // Keep each Go package timeout and every case's watchdog unchanged. Discover
  // and execute every parent case exactly once in attributable bounded groups.
  // The effective aggregate deadline is per batch; batching does not establish
  // that a prior timeout was harmless. Every group must still pass, with the
  // intrinsic worker/transaction concurrency inside each case unchanged.
  const cases = listedCases(captureGo([...flags, '-list', '^(Test|Example|Fuzz)', enginePackage]));
  const batches = caseBatches(cases);
  for (let i = 0; i < batches.length; i++) {
    process.stdout.write(`Engine batch ${i + 1}/${batches.length}: ${batches[i].cases.length} discovered cases\n`);
    run('go', [...flags, '-count=1', '-v', '-run', batches[i].pattern, enginePackage]);
  }
}

if (process.argv[1] && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))) {
  const args = process.argv.slice(2);
  if (args.some(arg => arg !== '--race') || args.length > 1) throw new Error('Use test-go.mjs [--race].');
  testGo({ race: args.includes('--race') });
}
