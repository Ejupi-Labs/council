import { ManagedProcess } from '../../dist/process.js';
import { fileURLToPath } from 'node:url';
const vendor = new ManagedProcess({ command: process.execPath,
  args: [fileURLToPath(new URL('./fake-native.mjs', import.meta.url))], cwd: process.cwd(), environment: process.env });
vendor.stdout.resume(); vendor.stderr.resume(); vendor.on('error', () => process.exitCode = 1);
vendor.stdin.end('tree-hang');
setInterval(() => {}, 1000);
