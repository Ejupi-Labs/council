import fs from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
const files = JSON.parse(process.env.COUNCIL_TEST_READ_FILES), counts = Object.fromEntries(files.map(path => [path, 0]));
const original = fs.readFileSync;
fs.readFileSync = function (path, ...args) {
  const value = original.call(this, path, ...args);
  if (Object.hasOwn(counts, String(path))) { counts[String(path)]++; fs.writeFileSync(path, '{ invalid after first complete read'); }
  return value;
};
syncBuiltinESMExports();
process.on('exit', () => fs.writeFileSync(process.env.COUNCIL_TEST_READ_TRACE, JSON.stringify(counts)));
