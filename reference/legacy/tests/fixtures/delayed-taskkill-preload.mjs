import cp from 'node:child_process';
import { appendFileSync } from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';

if (process.platform === 'win32' && process.env.COUNCIL_TEST_KILL_HELPER !== '1') {
  const spawn = cp.spawn;
  cp.spawn = (command, args, options) => {
    if (!String(command).toLowerCase().endsWith('taskkill.exe')) return spawn(command, args, options);
    if (process.env.COUNCIL_TEST_KILL_LOG) appendFileSync(process.env.COUNCIL_TEST_KILL_LOG, JSON.stringify({ target: args[args.indexOf('/PID') + 1] }) + '\n');
    const source = `const{spawn}=require('node:child_process');setTimeout(()=>{const c=spawn(process.argv[1],process.argv.slice(2),{stdio:'ignore',windowsHide:true});c.once('error',()=>process.exit(1));c.once('close',code=>process.exit(code??1));},2000);`;
    return spawn(process.execPath, ['-e', source, command, ...args], { ...options,
      env: { ...(options?.env ?? process.env), COUNCIL_TEST_KILL_HELPER: '1' } });
  };
  syncBuiltinESMExports();
}
