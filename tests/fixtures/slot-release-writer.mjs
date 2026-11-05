import { DatabaseSync } from 'node:sqlite';
import { writeFileSync } from 'node:fs';

const database = new DatabaseSync(process.argv[2]);
database.exec('PRAGMA busy_timeout=250'); database.exec('BEGIN IMMEDIATE');
let closed = false;
const finish = () => {
  if (closed) return;
  closed = true; clearTimeout(deadline);
  if (database.isTransaction) database.exec('COMMIT');
  database.close(); process.disconnect?.();
};
const deadline = setTimeout(finish, 15000);
process.on('disconnect', finish);
process.on('message', value => {
  if (value.type === 'release') finish();
  else if (value.type === 'cancel-and-relock') {
    const result = database.prepare('UPDATE runs SET cancel_requested=1 WHERE id=?').run(value.runId);
    if (result.changes !== 1) throw new Error('The owned cancellation fixture run is unavailable.');
    database.exec('COMMIT'); database.exec('BEGIN IMMEDIATE');
    writeFileSync(process.argv[3], JSON.stringify({ accepted: true }));
  }
});
process.send?.({ type: 'locked' });
