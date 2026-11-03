import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { configDefinition, workflowDefinition } from '../dist/config.js';
import { VERSION } from '../dist/version.js';

const root = new URL('../', import.meta.url);
for (const file of ['package.json', 'plugin.json', '.codex-plugin/plugin.json', '.claude-plugin/plugin.json']) {
  const metadata = JSON.parse(readFileSync(new URL(file, root), 'utf8'));
  if (metadata.version !== VERSION) throw new Error(`Version mismatch: ${file} must match ${VERSION}.`);
}
const marketplace = JSON.parse(readFileSync(new URL('.claude-plugin/marketplace.json', root), 'utf8'));
if (marketplace.plugins[0].version !== VERSION) throw new Error('Marketplace version is inconsistent.');
mkdirSync(new URL('schemas/', root), { recursive: true });
for (const [name, definition] of [['config', configDefinition], ['workflow', workflowDefinition]]) {
  writeFileSync(new URL(`schemas/${name}.schema.json`, root), `${JSON.stringify({ $schema: 'https://json-schema.org/draft/2020-12/schema', title: `Council ${name}`, ...definition }, null, 2)}\n`);
}
