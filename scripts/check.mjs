import { run, nodeScript } from './tools.mjs';
run('go', ['vet', './cmd/...', './internal/...', './scripts/...']);
nodeScript('node_modules/typescript/bin/tsc', ['-p', 'ui/tsconfig.json', '--noEmit']);
run('go', ['run', './scripts/checkformat']);
