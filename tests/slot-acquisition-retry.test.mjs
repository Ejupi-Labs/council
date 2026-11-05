import test from 'node:test';
import assert from 'node:assert/strict';
import { fork } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilStore } from '../dist/store.js';
import { WorkflowEngine } from '../dist/engine.js';
import { DispatchQuarantinedError } from '../dist/dispatch-quarantine.js';
import { admissionRefusalOrigin, newStateAdmissionAttempt, runAdmissionAttempt, consumeAdmissionRefusal, admissionTransaction } from '../dist/admission-attempt.js';
import { traceSlotAcquisition } from './slot-acquisition-trace.mjs';

async function boundedMessage(child) {
  let timer;
  try{return await Promise.race([once(child,'message'),once(child,'exit').then(([code])=>{throw new Error('Owned writer exited before admission setup: '+code);}),new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('Owned writer did not confirm its lock within 3 seconds.')),3000);})]);}
  finally{clearTimeout(timer);}
}
function fixture(t) {
  const parent=realpathSync.native(tmpdir()), root=mkdtempSync(join(parent,'council-slot-acquisition-'));
  const store=new CouncilStore(join(root,'.council')); store.registerInstance('owner',process.pid); store.bindInstance('owner');
  const input={mode:'delegate',task:'Offline bounded call-slot proof.',agents:['codex'],permission:'read'};
  const children=[], failures=[], attempts=[], native=store.acquireSlot, transaction=store.transaction;
  const claim=id=>{const now=new Date().toISOString();store.saveRun({id,input,status:'queued',createdAt:now,updatedAt:now,steps:[]});store.claimRun(id,'owner');};claim('run');
  store.acquireSlot=function(...args){attempts.push(args);return native.apply(this,args);};
  store.transaction=function(...args){try{return transaction.apply(this,args);}catch(error){if(admissionRefusalOrigin(error))failures.push(error);throw error;}};
  t.after(async()=>{
    for(const child of children)if(child.exitCode===null){child.kill();await once(child,'exit').catch(()=>{});}
    store.acquireSlot=native;store.transaction=transaction;
    if(store.db.isTransaction)store.db.exec('ROLLBACK');
    for(const row of store.db.prepare("SELECT id FROM runs WHERE owner_instance='owner' AND status IN ('queued','running')").all()){
      const run=store.getRun(row.id);store.finalizeRun({...run,status:'cancelled',error:'Inspected offline acquisition fixture; no task replay.',updatedAt:new Date().toISOString()});
    }
    for(const row of store.db.prepare("SELECT token FROM slots WHERE instance_id='owner'").all())store.releaseSlot(row.token,'owner');
    store.unregisterInstance('owner');store.close();
    assert.equal(dirname(realpathSync.native(root)),parent);assert.ok(basename(root).startsWith('council-slot-acquisition-'));
    rmSync(root,{recursive:true,force:true,maxRetries:10,retryDelay:100});
  });
  return {root,store,input,claim,attempts,failures,native,transaction,
    async quarantine(ms=750){
      const filename=join(root,'incident-writer.mjs');
      writeFileSync(filename,`import{CouncilStore}from${JSON.stringify(new URL('../dist/store.js',import.meta.url).href)};const store=new CouncilStore(process.argv[2]);store.registerInstance('reporter',process.pid);const transaction=store.transaction;store.transaction=function(operation,...args){return transaction.call(this,()=>{process.send('locked');Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,Number(process.argv[3]));return operation();},...args);};store.reportProcessCleanupIncident({reportKey:'synthetic-owned-observation',instanceId:'reporter',stage:'agent_call'});store.transaction=transaction;store.unregisterInstance('reporter');store.close();process.disconnect();`);
      const child=fork(filename,[join(root,'.council'),String(ms)],{windowsHide:true,stdio:['ignore','ignore','pipe','ipc']});children.push(child);
      const exited=once(child,'exit');assert.equal((await boundedMessage(child))[0],'locked');return {child,exited};
    },
    async lock(ms=750,releaseSql=''){
      const filename=join(root,'writer-'+children.length+'.mjs');
      writeFileSync(filename,`import{DatabaseSync}from'node:sqlite';const db=new DatabaseSync(process.argv[2]);db.exec('PRAGMA busy_timeout=250;BEGIN IMMEDIATE');process.send('locked');setTimeout(()=>{db.exec(${JSON.stringify(releaseSql)});db.exec('COMMIT');db.close();process.disconnect();},Number(process.argv[3]));`);
      const child=fork(filename,[store.databasePath,String(ms)],{windowsHide:true,stdio:['ignore','ignore','pipe','ipc']});children.push(child);
      const exited=once(child,'exit');assert.equal((await boundedMessage(child))[0],'locked');return {child,exited};
    },
    settings(overrides={}){return {version:1,workspaceRoot:root,stateDir:join(root,'.council'),backend:'mock',maxConcurrency:1,timeoutMs:2000,maxOutputBytes:4096,maxCalls:20,agents:{codex:{enabled:true,profile:'codex'}},...overrides};}
  };
}

test('actual native BEGIN contention retries only one call admission with the same waiter identity', {timeout:10000},async t=>{
  const wire=fixture(t), writer=await wire.lock();
  const token=await wire.store.acquireSlotWithRetry('owner','run',1,'same-waiter');await writer.exited;
  assert.equal(typeof token,'string');assert.ok(wire.failures.length>0);assert.ok(wire.attempts.length>1);
  assert.ok(wire.attempts.every(args=>args[3]==='same-waiter'));
  assert.equal(wire.store.db.isTransaction,false);assert.equal(wire.store.health().counts.slots,1);assert.equal(wire.store.health().counts.slotWaiters,0);
  await wire.store.releaseSlotWithRetry(token,'owner');
});

test('a successfully queued undefined result ends lock retries immediately and preserves its queue generation',async t=>{
  const wire=fixture(t);wire.claim('incumbent');const held=wire.store.acquireSlot('owner','incumbent',1,'held');wire.attempts.length=0;
  assert.equal(await wire.store.acquireSlotWithRetry('owner','run',1,'waiting'),undefined);
  assert.equal(wire.attempts.length,1);const generation=wire.store.db.prepare("SELECT CAST(sequence AS TEXT) AS value FROM slot_waiters WHERE waiter_id='waiting'").get().value;
  assert.equal(await wire.store.acquireSlotWithRetry('owner','run',1,'waiting'),undefined);
  assert.equal(wire.store.db.prepare("SELECT CAST(sequence AS TEXT) AS value FROM slot_waiters WHERE waiter_id='waiting'").get().value,generation);
  wire.store.cancelSlotWaiter('owner','waiting');wire.store.releaseSlot(held,'owner');
});

for(const stage of ['body','commit','range'])test(`${stage} failure never retries or manufactures a missing acknowledgement`,async t=>{
  const wire=fixture(t), db=wire.store.db, exec=db.exec;let commits=0;
  const fault=stage==='range'?new RangeError('Unverified write-result metadata.'):Object.assign(new Error('Unverified '+stage+' acknowledgement.'),{code:'ERR_SQLITE_ERROR',errcode:5});
  if(stage==='body'){
    wire.store.acquireSlot=function(...args){wire.attempts.push(args);return this.transaction(()=>{this.db.prepare('INSERT INTO slots VALUES(?,?,?,?)').run('unknown','owner','run',Date.now());throw fault;});};
  }else db.exec=function(sql,...args){const result=exec.call(this,sql,...args);if(sql==='COMMIT'){commits++;throw fault;}return result;};
  await assert.rejects(wire.store.acquireSlotWithRetry('owner','run',1,'same'),error=>error===fault);
  db.exec=exec;assert.equal(wire.attempts.length,1);assert.equal(wire.failures.length,0);
  assert.equal(wire.store.health().counts.slots,stage==='body'?0:1);
  for(const row of db.prepare('SELECT token FROM slots').all())wire.store.releaseSlot(row.token,'owner');
  if(stage!=='body')assert.equal(commits,1);
});

test('abort after a genuine refusal retains the exact cancellation reason and performs no second attempt',{timeout:10000},async t=>{
  const wire=fixture(t), writer=await wire.lock(), controller=new AbortController(), reason={accepted:'exact cancellation'};
  const original=wire.store.transaction;wire.store.transaction=function(...args){try{return original.apply(this,args);}catch(error){if(admissionRefusalOrigin(error))queueMicrotask(()=>controller.abort(reason));throw error;}};
  await assert.rejects(wire.store.acquireSlotWithRetry('owner','run',1,'cancel',{signal:controller.signal}),error=>error===reason);
  await writer.exited;assert.equal(wire.attempts.length,1);assert.equal(wire.store.health().counts.slots,0);assert.equal(wire.store.health().counts.slotWaiters,0);
});

test('fresh owner metadata after lock release prevents slot admission',{timeout:10000},async t=>{
  const wire=fixture(t), writer=await wire.lock(750,"UPDATE runs SET owner_instance=NULL, owner_pid=NULL WHERE id='run'");
  await assert.rejects(wire.store.acquireSlotWithRetry('owner','run',1,'owner-check'),/does not own/);await writer.exited;
  assert.ok(wire.failures.length>0);assert.equal(wire.store.health().counts.slots,0);
});

test('fresh engine fault after refusal stops acquisition without a slot or agent call',{timeout:10000},async t=>{
  const wire=fixture(t), writer=await wire.lock();let fault;const original=wire.store.transaction;
  wire.store.transaction=function(...args){try{return original.apply(this,args);}catch(error){if(admissionRefusalOrigin(error))queueMicrotask(()=>{fault=new Error('Fresh host fault.');});throw error;}};
  await assert.rejects(wire.store.acquireSlotWithRetry('owner','run',1,'fault',{assertDispatch:()=>{if(fault)throw fault;}}),/Fresh host fault/);
  await writer.exited;assert.equal(wire.attempts.length,1);assert.equal(wire.store.health().counts.slots,0);
});

test('a blocking fresh check cannot extend the fixed contention deadline',{timeout:10000},async t=>{
  const wire=fixture(t), writer=await wire.lock(), descriptor=Object.getOwnPropertyDescriptor(performance,'now');let clock=100;
  Object.defineProperty(performance,'now',{configurable:true,value:()=>clock});t.after(()=>{if(descriptor)Object.defineProperty(performance,'now',descriptor);else delete performance.now;});
  await assert.rejects(wire.store.acquireSlotWithRetry('owner','run',1,'deadline',{assertDispatch:()=>{if(wire.failures.length)clock+=5001;}}),/five seconds/);
  await writer.exited;assert.equal(wire.attempts.length,1);assert.equal(wire.store.health().counts.slots,0);
});

test('opaque state-only proof cannot be consumed while an outer synchronous attempt remains active',{timeout:10000},async t=>{
  const wire=fixture(t), writer=await wire.lock(), outer=newStateAdmissionAttempt(wire.store.admissionConnection(),()=>{});let consumed, innerError;
  assert.throws(()=>runAdmissionAttempt(outer,()=>{const inner=newStateAdmissionAttempt(wire.store.admissionConnection(),()=>{});try{runAdmissionAttempt(inner,()=>admissionTransaction(wire.store.db,()=>{}));}catch(error){innerError=error;consumed=consumeAdmissionRefusal(inner,error);throw error;}}),error=>error===innerError);
  assert.equal(consumed,false);await writer.exited;
});

test('engine retries proven pre-call contention once per attempt and dispatches only after durable acquisition',{timeout:15000},async t=>{
  const wire=fixture(t);let calls=0, writer;
  const original=wire.store.acquireSlotWithRetry;wire.store.acquireSlotWithRetry=async function(...args){writer??=await wire.lock();return original.apply(this,args);};
  const runner={doctor:async()=>[],run:async request=>{calls++;assert.equal(wire.store.health().counts.slots,1);assert.ok(wire.failures.length>0);return {agent:request.agent,status:'completed',output:'One offline call.',durationMs:1};}};
  const result=await new WorkflowEngine(wire.settings(),runner,wire.store,{instanceId:'owner'}).run(wire.input,{id:'run'});await writer.exited;
  assert.equal(result.status,'completed');assert.equal(calls,1);assert.equal(result.steps.length,1);assert.equal(wire.store.health().counts.slots,0);assert.equal(wire.store.health().counts.slotWaiters,0);
});

test('legacy synchronous acquisition remains a supported engine fallback',async t=>{
  const wire=fixture(t);wire.store.acquireSlotWithRetry=undefined;let calls=0;
  const runner={doctor:async()=>[],run:async request=>{calls++;return {agent:request.agent,status:'completed',output:'Legacy store.',durationMs:1};}};
  const result=await new WorkflowEngine(wire.settings(),runner,wire.store,{instanceId:'owner'}).run(wire.input,{id:'run'});
  assert.equal(result.status,'completed');assert.equal(calls,1);assert.equal(wire.attempts.length,1);
});

test('durable quarantine committed by the blocking writer is observed before any slot or agent admission',{timeout:10000},async t=>{
  const wire=fixture(t), writer=await wire.quarantine();
  await assert.rejects(wire.store.acquireSlotWithRetry('owner','run',1,'quarantine'),error=>error instanceof DispatchQuarantinedError);
  assert.equal((await writer.exited)[0],0);assert.ok(wire.failures.length>0);
  assert.equal(wire.store.health().counts.slots,0);assert.equal(wire.store.health().counts.slotWaiters,0);
  assert.equal(wire.store.dispatchQuarantinePage().incidents.length,1);
});

test('capacity waiting remains cancellable after more than the five-second SQLite contention window',{timeout:15000},async t=>{
  const wire=fixture(t);wire.claim('incumbent');const held=wire.store.acquireSlot('owner','incumbent',1,'incumbent'), controller=new AbortController();let calls=0;
  const runner={doctor:async()=>[],run:async()=>{calls++;throw new Error('Capacity-blocked runner must not start.');}};
  const pending=new WorkflowEngine(wire.settings(),runner,wire.store,{instanceId:'owner'}).run(wire.input,{id:'run',signal:controller.signal});
  const until=Date.now()+2000;while(wire.store.health().counts.slotWaiters<1&&Date.now()<until)await delay(10);
  assert.equal(wire.store.health().counts.slotWaiters,1);await delay(5250);controller.abort('Accepted after an ordinary capacity wait.');
  const result=await pending;assert.equal(result.status,'cancelled');assert.equal(calls,0);assert.equal(wire.store.health().counts.slotWaiters,0);
  wire.store.releaseSlot(held,'owner');
});

test('sustained native BEGIN contention expires without queue, slot or agent admission',{timeout:15000},async t=>{
  const wire=fixture(t), writer=await wire.lock(6500), started=performance.now();
  await assert.rejects(wire.store.acquireSlotWithRetry('owner','run',1,'sustained'),/five seconds/);
  assert.ok(performance.now()-started>=5000);assert.ok(wire.failures.length>2);await writer.exited;
  assert.equal(wire.store.health().counts.slots,0);assert.equal(wire.store.health().counts.slotWaiters,0);
});

test('post-token quota/profile refusal retains its dispatch receipt and avoids persistence-failure notification',async t=>{
  const wire=fixture(t), notifications=[], refusal=Object.assign(new Error('Fresh profile allowance policy refusal.'),{code:'COUNCIL_TEST_ALLOWANCE'});let calls=0;
  const runner={doctor:async()=>[],run:async()=>{calls++;throw new Error('Policy-blocked runner must not start.');}};
  const result=await new WorkflowEngine(wire.settings(),runner,wire.store,{instanceId:'owner',assertAgentDispatch:()=>{throw refusal;},onStateFailure:error=>notifications.push(error)}).run(wire.input,{id:'run'});
  assert.equal(result.status,'failed');assert.equal(calls,0);assert.equal(notifications.length,0);assert.equal(wire.store.health().counts.slots,0);
  const receipt=wire.store.events('run').find(event=>event.type==='step.dispatch_blocked');assert.equal(receipt.data.code,'COUNCIL_TEST_ALLOWANCE');
});

test('child-fixture phase diagnostics distinguish actual native BEGIN refusal from a native-shaped body error',{timeout:10000},async t=>{
  const wire=fixture(t), capture=traceSlotAcquisition(wire.store,admissionRefusalOrigin), writer=await wire.lock();
  const token=await wire.store.acquireSlotWithRetry('owner','run',1,'traced');await writer.exited;wire.store.releaseSlot(token,'owner');
  assert.ok(capture().records.some(record=>record.phase==='begin'&&(record.sqliteCode&0xff)===5));
  const body=Object.assign(new Error('Sensitive body must never appear in diagnostics.'),{code:'ERR_SQLITE_ERROR',errcode:5});
  wire.store.acquireSlot=()=>{throw body;};const other=traceSlotAcquisition(wire.store,admissionRefusalOrigin);
  for(let index=0;index<12;index++)assert.throws(()=>wire.store.acquireSlot(),error=>error===body);
  assert.equal(other().records.length,8);assert.equal(other().omitted,4);assert.ok(other().records.every(record=>record.phase==='operation'));
  assert.equal(JSON.stringify(other()).includes('Sensitive'),false);
});
