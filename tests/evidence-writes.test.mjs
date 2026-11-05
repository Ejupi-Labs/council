import test from 'node:test';
import assert from 'node:assert/strict';
import { fork, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { existsSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilStore } from '../dist/store.js';
import { WorkflowEngine, WorkflowPersistenceError } from '../dist/engine.js';
import { GitWorkspaces } from '../dist/workspaces.js';
import { admissionRefusalOrigin } from '../dist/admission-attempt.js';

const deferred=()=>{let resolve;const promise=new Promise(done=>{resolve=done;});return{resolve,promise};};
const task={mode:'delegate',task:'Retain offline evidence exactly once.',agents:['codex'],permission:'read'};
const answer=agent=>({agent,status:'completed',output:agent+' retained evidence',durationMs:1});
async function bounded(promise,label){let timer;try{return await Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error(label+' exceeded 3 seconds.')),3000);})]);}finally{clearTimeout(timer);}}
function settings(root,extra={}){return{version:1,workspaceRoot:root,stateDir:join(root,'.council'),backend:'mock',maxConcurrency:2,timeoutMs:1000,maxOutputBytes:4096,maxCalls:20,agents:{codex:{enabled:true,profile:'codex'},claude:{enabled:true,profile:'claude'}},...extra};}
function memory(){const runs=new Map(),log=[];return{runs,log,getRun:id=>runs.get(id),saveRun:run=>runs.set(run.id,structuredClone(run)),appendEvent:(runId,type,data)=>log.push({runId,type,data:structuredClone(data)})};}
function fixture(t){
  const parent=realpathSync.native(tmpdir()),root=mkdtempSync(join(parent,'council-evidence-write-')),store=new CouncilStore(join(root,'.council'));
  store.registerInstance('owner',process.pid);store.bindInstance('owner');const now=new Date().toISOString(),run={id:'owned',input:structuredClone(task),status:'queued',createdAt:now,updatedAt:now,steps:[]};store.createRun(run,'owner');
  const writers=[],refusals=[],transaction=store.transaction;let onRefusal;
  store.transaction=function(...args){try{return transaction.apply(this,args);}catch(error){if(admissionRefusalOrigin(error)){
    refusals.push(error);queueMicrotask(()=>{assert.equal(store.db.isTransaction,false);onRefusal?.();for(const writer of writers)writer.release();});
  }throw error;}};
  t.after(async()=>{
    const failures=[];for(const writer of writers){try{writer.release();const [code,signal]=await bounded(writer.done,'Owned evidence writer drain');assert.equal(code,0);assert.equal(signal,null);}catch(error){failures.push(error);writer.child.kill();try{await bounded(writer.done,'Owned evidence writer termination');}catch(failure){failures.push(failure);}}}
    store.transaction=transaction;
    try{if(store.db.isTransaction)store.db.exec('ROLLBACK');const saved=store.getRun(run.id);if(['queued','running'].includes(saved.status))store.finalizeRun({...saved,status:'failed',error:'Inspected offline fixture; no replay.'});for(const row of store.db.prepare('SELECT token FROM slots').all())store.releaseSlot(row.token,'owner');store.unregisterInstance('owner');}catch(error){failures.push(error);}finally{try{store.close();}catch(error){failures.push(error);}}
    if(failures.length)throw new AggregateError(failures,'Retained failed evidence fixture.');assert.equal(dirname(realpathSync.native(root)),parent);assert.ok(basename(root).startsWith('council-evidence-write-'));rmSync(root,{recursive:true,force:true,maxRetries:10,retryDelay:100});
  });
  return{root,store,run,refusals,setRefusal:handler=>{onRefusal=handler;},async lock(){const releasePath=join(root,'writer-'+writers.length+'.release'),child=fork(new URL('./fixtures/admission-writer.mjs',import.meta.url),[store.databasePath,releasePath,'plain'],{windowsHide:true,stdio:['ignore','ignore','pipe','ipc']});child.stderr.resume();const writer={child,done:once(child,'close'),release:()=>writeFileSync(releasePath,'release')};writers.push(writer);assert.equal((await bounded(Promise.race([once(child,'message'),writer.done.then(([code])=>{throw new Error('Writer exited before lock '+code);})]),'Owned evidence writer setup'))[0],'locked');return writer;}};
}

for(const operation of ['save','event','finalize'])test(`fixed ${operation} retries only a genuine BEGIN refusal and freezes its input`,{timeout:10000},async t=>{
  const wire=fixture(t),writer=await wire.lock(),payload={body:'original immutable event'},snapshot={...wire.run,status:operation==='finalize'?'completed':'running',output:'original snapshot'};
  wire.setRefusal(()=>{payload.body='later mutation';snapshot.output='later mutation';});
  if(operation==='save')await wire.store.saveRunWithRetry(snapshot);
  else if(operation==='event')await wire.store.appendEventWithRetry(wire.run.id,'retained.proof',payload);
  else assert.equal((await wire.store.finalizeRunWithRetry(snapshot)).output,'original snapshot');
  await writer.done;assert.ok(wire.refusals.length>0);assert.equal(wire.store.db.isTransaction,false);
  if(operation==='event'){const events=wire.store.events(wire.run.id).filter(event=>event.type==='retained.proof');assert.equal(events.length,1);assert.equal(events[0].data.body,'original immutable event');}
  else assert.equal(wire.store.getRun(wire.run.id).output,'original snapshot');
});

for(const operation of ['save','event','finalize'])for(const phase of ['body','commit','range'])test(`${operation} ${phase} failure never replays an unknown write`,async t=>{
  const wire=fixture(t),db=wire.store.db,exec=db.exec,transaction=wire.store.transaction;let attempts=0;
  const fault=phase==='range'?new RangeError('Unknown result metadata.'):Object.assign(new Error('Unknown '+phase+' acknowledgement.'),{code:'ERR_SQLITE_ERROR',errcode:5});
  if(phase==='body')wire.store.transaction=function(callback,...args){attempts++;return transaction.call(this,()=>{callback();throw fault;},...args);};
  else db.exec=function(sql,...args){const value=exec.call(this,sql,...args);if(sql==='COMMIT'){attempts++;throw fault;}return value;};
  try{await assert.rejects(operation==='save'?wire.store.saveRunWithRetry({...wire.run,status:'running'}):operation==='event'?wire.store.appendEventWithRetry(wire.run.id,'unknown.proof',{body:'evidence'}):wire.store.finalizeRunWithRetry({...wire.run,status:'completed'}),error=>error===fault);assert.equal(attempts,1);assert.equal(wire.refusals.length,0);}
  finally{db.exec=exec;wire.store.transaction=transaction;}
  if(operation==='event')assert.equal(wire.store.events(wire.run.id).filter(event=>event.type==='unknown.proof').length,phase==='body'?0:1);
});

test('pure evidence writes remain available under durable quarantine',async t=>{
  const wire=fixture(t);wire.store.reportProcessCleanupIncident({reportKey:'synthetic',instanceId:'owner',stage:'agent_call'});
  await wire.store.saveRunWithRetry({...wire.run,status:'running'});await wire.store.appendEventWithRetry(wire.run.id,'inspection.evidence',{retained:true});
  const final=await wire.store.finalizeRunWithRetry({...wire.run,status:'completed'});assert.equal(final.status,'failed');assert.match(final.error,/Inspect OS processes/);assert.equal(wire.store.events(wire.run.id).some(event=>event.type==='inspection.evidence'),true);
});

test('engine recovers actual initial-save, start-event and terminal BEGIN contention without replay',{timeout:15000},async t=>{
  const wire=fixture(t),native={save:wire.store.saveRunWithRetry,event:wire.store.appendEventWithRetry,final:wire.store.finalizeRunWithRetry},blocked=new Set();let calls=0;
  wire.store.saveRunWithRetry=async function(run){if(!blocked.has('save')){blocked.add('save');await wire.lock();}return native.save.call(this,run);};
  wire.store.appendEventWithRetry=async function(id,type,data){if(type==='step.started'&&!blocked.has('event')){blocked.add('event');await wire.lock();}return native.event.call(this,id,type,data);};
  wire.store.finalizeRunWithRetry=async function(run){if(!blocked.has('final')){blocked.add('final');await wire.lock();}return native.final.call(this,run);};
  const result=await new WorkflowEngine(settings(wire.root),{doctor:async()=>[],run:async request=>{calls++;return answer(request.agent);}},wire.store,{instanceId:'owner'}).run(task,{id:wire.run.id});
  assert.equal(result.status,'completed');assert.equal(calls,1);assert.equal(result.steps.length,1);assert.ok(wire.refusals.length>=3);assert.equal(wire.store.health().counts.slots,0);assert.equal(wire.store.health().counts.slotWaiters,0);
});

test('parallel save snapshots materialize in serial execution and retain both completed siblings',{timeout:10000},async()=>{
  const store=memory(),entered=deferred(),finishA=deferred(),finishB=deferred(),saveEntered=deferred(),saveFinish=deferred(),secondReturned=deferred();let calls=0,active=0,maximum=0,held=false;
  store.saveRunWithRetry=async snapshot=>{active++;maximum=Math.max(maximum,active);if(snapshot.steps.length===1&&!held){held=true;saveEntered.resolve();await saveFinish.promise;}store.saveRun(snapshot);active--;};
  const runner={doctor:async()=>[],run:async request=>{calls++;if(calls===2)entered.resolve();await(request.agent==='codex'?finishA.promise:finishB.promise);if(request.agent==='claude')secondReturned.resolve();return answer(request.agent);}};
  const pending=new WorkflowEngine(settings(process.cwd()),runner,store).run({...task,mode:'parallel',agents:['codex','claude']});
  try{await bounded(entered.promise,'Both parallel calls');finishA.resolve();await bounded(saveEntered.promise,'First sibling save');finishB.resolve();await bounded(secondReturned.promise,'Second sibling result');await delay(0);saveFinish.resolve();const result=await bounded(pending,'Parallel evidence drain');assert.equal(result.status,'completed');assert.equal(maximum,1);assert.equal(calls,2);assert.equal(store.getRun(result.id).steps.length,2);assert.equal(new Set(result.steps.map(step=>step.id)).size,2);}
  finally{finishA.resolve();finishB.resolve();saveFinish.resolve();await bounded(pending,'Parallel fixture drain');}
});

for(const control of ['cancel','quota'])test(`fresh ${control} after the step-start evidence barrier prevents NativeRunner with zero retained steps`,{timeout:10000},async()=>{
  const store=memory(),started=deferred(),release=deferred(),controller=new AbortController(),notifications=[];let calls=0,blocked=false;
  store.appendEventWithRetry=async(id,type,data)=>{if(type==='step.started'){started.resolve();await release.promise;}store.appendEvent(id,type,data);};
  const pending=new WorkflowEngine(settings(process.cwd()),{doctor:async()=>[],run:async()=>{calls++;return answer('codex');}},store,{assertAgentDispatch:()=>{if(blocked)throw Object.assign(new Error('Fresh allowance refusal.'),{code:'COUNCIL_TEST_ALLOWANCE'});},onStateFailure:error=>notifications.push(error)}).run(task,{signal:controller.signal});
  try{await bounded(started.promise,'Pre-call evidence barrier');if(control==='cancel')controller.abort('Accepted while evidence was pending.');else blocked=true;release.resolve();const result=await bounded(pending,'Controlled evidence completion');assert.equal(result.status,control==='cancel'?'cancelled':'failed');assert.equal(calls,0);assert.equal(result.steps.length,0);assert.equal(notifications.length,0);if(control==='quota')assert.equal(store.log.some(event=>event.type==='step.dispatch_blocked'&&event.data.code==='COUNCIL_TEST_ALLOWANCE'),true);}
  finally{release.resolve();await bounded(pending,'Controlled fixture drain');}
});

test('interval evidence rejection is observed, drained and remains failed after late cancellation',{timeout:10000},async()=>{
  const store=memory(),entered=deferred(),diagnostic=deferred(),release=deferred(),controller=new AbortController(),unhandled=[];let blocked=false,settled=false;
  const onUnhandled=reason=>unhandled.push(reason);process.on('unhandledRejection',onUnhandled);
  store.appendEventWithRetry=async(id,type,data)=>{if(type==='step.dispatch_blocked'){diagnostic.resolve();await release.promise;throw new Error('Real interval evidence failure.');}store.appendEvent(id,type,data);};
  const runner={doctor:async()=>[],run:async request=>{entered.resolve();await new Promise(resolve=>request.signal.addEventListener('abort',resolve,{once:true}));return {...answer(request.agent),status:'cancelled',output:'Drained partial evidence.'};}};
  const pending=new WorkflowEngine(settings(process.cwd()),runner,store,{assertAgentDispatch:()=>{if(blocked)throw new Error('Fresh policy change.');}}).run(task,{signal:controller.signal});pending.then(()=>{settled=true;},()=>{settled=true;});
  try{await bounded(Promise.race([entered.promise,pending.then(()=>{throw new Error('Workflow settled before runner entry.');})]),'Active call entry');blocked=true;await bounded(Promise.race([diagnostic.promise,pending.then(()=>{throw new Error('Workflow settled before interval evidence.');})]),'Interval evidence entry');await delay(10);assert.equal(settled,false);release.resolve();controller.abort('Later cancellation.');await assert.rejects(pending,error=>error instanceof WorkflowPersistenceError&&error.run.status==='failed'&&/Real interval evidence failure/.test(error.run.error));assert.equal(unhandled.length,0);}
  finally{release.resolve();controller.abort('Owned interval fixture cleanup.');await bounded(Promise.allSettled([pending]),'Interval fixture drain');process.removeListener('unhandledRejection',onUnhandled);}
});

test('async worktree intent is awaited before managed filesystem mutation',{timeout:15000},async t=>{
  const parent=realpathSync.native(tmpdir()),root=mkdtempSync(join(parent,'council-async-intent-')),state=join(root,'.council'),gate=deferred(),entered=deferred();
  execFileSync('git',['init','--quiet',root],{windowsHide:true});writeFileSync(join(root,'sample.txt'),'base\n');execFileSync('git',['-C',root,'add','sample.txt'],{windowsHide:true});execFileSync('git',['-C',root,'-c','user.name=Council Test','-c','user.email=council@example.invalid','commit','--quiet','-m','Offline base'],{windowsHide:true});
  const manager=new GitWorkspaces(root,state),controller=new AbortController();let artifact;
  const pending=manager.create('intent','codex','codex',controller.signal,async value=>{artifact=value;entered.resolve();await gate.promise;throw new Error('Intent database failed.');});
  try{await bounded(Promise.race([entered.promise,pending.then(()=>{throw new Error('Git preparation settled before its intent callback.');})]),'Async Git intent');assert.equal(existsSync(join(state,'worktrees')),false);assert.equal(existsSync(join(state,'workspace-manifests')),false);assert.equal(existsSync(artifact.path),false);gate.resolve();await assert.rejects(pending,/Intent database failed/);assert.equal(existsSync(artifact.path),false);}
  finally{gate.resolve();controller.abort('Owned intent fixture cleanup.');await bounded(Promise.allSettled([pending]),'Intent fixture drain');manager.forgetRun('intent');assert.equal(dirname(realpathSync.native(root)),parent);rmSync(root,{recursive:true,force:true,maxRetries:10,retryDelay:100});}
});

test('unknown terminal acknowledgement remains failed while a known committed terminal event failure stays distinct',async()=>{
  const store=memory();let finalizations=0,calls=0;
  store.finalizeRunWithRetry=async run=>{finalizations++;store.saveRun(run);throw new RangeError('Unknown terminal acknowledgement.');};
  await assert.rejects(new WorkflowEngine(settings(process.cwd()),{doctor:async()=>[],run:async request=>{calls++;return answer(request.agent);}},store).run(task),error=>{
    assert.ok(error instanceof WorkflowPersistenceError);assert.equal(error.run.status,'failed');assert.match(error.run.error,/Unknown terminal acknowledgement/);assert.equal(store.getRun(error.run.id).status,'completed');return true;
  });
  assert.equal(finalizations,1);assert.equal(calls,1);
});
