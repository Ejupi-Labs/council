import { DatabaseSync } from 'node:sqlite';
import { existsSync } from 'node:fs';

const [databasePath, releasePath, kind, storeUrl] = process.argv.slice(2);
const sleeper = new Int32Array(new SharedArrayBuffer(4));
function holdUntilReleased() {
  process.send?.('locked');
  const deadline = performance.now() + 15000;
  while (!existsSync(releasePath)) {
    if (performance.now() >= deadline) throw new Error('The owned admission writer was not released after a genuine refusal.');
    Atomics.wait(sleeper, 0, 0, 5);
  }
}
if (kind === 'quarantine') {
  const { CouncilStore } = await import(storeUrl);
  const store = new CouncilStore(databasePath);
  try {
    store.registerInstance('reporter', process.pid);
    const transaction = store.transaction;
    store.transaction = function (operation, ...args) { return transaction.call(this, () => { holdUntilReleased(); return operation(); }, ...args); };
    try { store.reportProcessCleanupIncident({ reportKey: 'synthetic-owned-observation', instanceId: 'reporter', stage: 'agent_call' }); }
    finally { store.transaction = transaction; }
    store.unregisterInstance('reporter');
  } finally { store.close(); }
} else {
  const db = new DatabaseSync(databasePath);
  try {
    db.exec('PRAGMA busy_timeout=250; BEGIN IMMEDIATE'); holdUntilReleased();
    if (kind === 'owner-loss') db.exec("UPDATE runs SET owner_instance=NULL, owner_pid=NULL WHERE id='run'");
    db.exec('COMMIT');
  } catch (error) { if (db.isTransaction) db.exec('ROLLBACK'); throw error; }
  finally { db.close(); }
}
process.disconnect?.();
