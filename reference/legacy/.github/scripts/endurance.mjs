import { spawn } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const repository = fileURLToPath(new URL('../../', import.meta.url));
const platforms = Object.freeze({ ubuntu: 'linux', windows: 'win32', macos: 'darwin' });
const durations = Object.freeze({ '60s': 60_000, '5m': 300_000, '10m': 600_000, '30m': 1_800_000 });
const workloads = Object.freeze({ service: 'soak.mjs', storage: 'storage-soak.mjs', artifacts: 'artifact-soak.mjs', coordination: 'coordination-soak.mjs', rooms: 'room-soak.mjs' });

export function endurancePlan(environment = process.env, platform = process.platform) {
  const os = environment.COUNCIL_ENDURANCE_OS;
  const workload = environment.COUNCIL_ENDURANCE_WORKLOAD;
  const duration = environment.COUNCIL_ENDURANCE_DURATION;
  if (!Object.hasOwn(platforms, os)) throw new Error('Choose ubuntu, windows or macos for the endurance OS.');
  if (platforms[os] !== platform) throw new Error('The selected endurance OS does not match this runner.');
  if (!Object.hasOwn(workloads, workload)) throw new Error('Choose service, storage, artifacts, coordination or rooms for the endurance workload.');
  if (!Object.hasOwn(durations, duration)) throw new Error('Choose 60s, 5m, 10m or 30m for the endurance duration.');
  const script = fileURLToPath(new URL(`../../scripts/${workloads[workload]}`, import.meta.url));
  const args = ['--expose-gc', script, '--duration-ms', String(durations[duration])];
  if (workload === 'service') args.push('--label', 'github-manual');
  return Object.freeze({ command: process.execPath, args: Object.freeze(args), cwd: repository });
}

export async function runEndurance(environment = process.env, spawnChild = spawn, platform = process.platform) {
  const plan = endurancePlan(environment, platform);
  const child = spawnChild(plan.command, plan.args, {
    cwd: plan.cwd,
    shell: false,
    windowsHide: true,
    stdio: 'inherit',
    env: { ...environment, COUNCIL_BACKEND: 'mock', COUNCIL_DEPTH: '0' },
  });
  return await new Promise((resolve, reject) => {
    const startedError = () => { child.removeListener('close', closed); reject(new Error('The offline endurance process could not start.')); };
    const closed = (code, signal) => {
      child.removeListener('error', startedError);
      resolve(signal || !Number.isInteger(code) || code < 0 || code > 255 ? 1 : code);
    };
    child.once('error', startedError);
    child.once('close', closed);
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 2) throw new Error('Endurance options must be supplied through the workflow environment.');
    process.exitCode = await runEndurance();
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
