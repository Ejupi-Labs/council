import { run, nodeScript } from './tools.mjs';
import { readdir } from 'node:fs/promises';
nodeScript('scripts/build.mjs');
run('go', ['test', './cmd/...', './internal/...', './scripts/...']);
const uiTests = (await readdir(new URL('../ui/tests/', import.meta.url))).filter(name => name.endsWith('.test.mjs')).map(name => `ui/tests/${name}`);
if (uiTests.length === 0) throw new Error('Browser acceptance tests are missing.');
run(process.execPath, ['--test', '--test-concurrency=1', ...uiTests]);
const scriptTests = (await readdir(new URL('./', import.meta.url))).filter(name => name.endsWith('.test.mjs')).map(name => `scripts/${name}`);
if (scriptTests.length) run(process.execPath, ['--test', ...scriptTests]);
