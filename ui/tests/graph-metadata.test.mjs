import assert from 'node:assert/strict';
import test from 'node:test';
import { fixture, ownerAPI } from './helpers.mjs';

test('the shipped client validates the exact 006 pool and keeps dev.2–5 owners Stop-only', { timeout: 60000 }, async t => {
  const f = await fixture(t); const owner = await f.start(); const page = await f.browser.newPage();
  let writes = 0;
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) writes++; });
  await page.goto(owner.url);
  const actual = (await ownerAPI(owner, '/meta')).envelope.data;
  assert.equal(actual.appVersion, '2.0.0-dev.6'); assert.equal(actual.graphContract, '006-v1');
  const cases = await page.evaluate(async observed => {
    const { supportsManagedExecution, supportsOwnerStop } = await import('/api.js');
    const results = [];
    const check = (name, metadata, execute, stop = execute) => results.push({ name, execute: supportsManagedExecution(metadata), stop: supportsOwnerStop(metadata), expected: { execute, stop } });
    const clone = () => structuredClone(observed);
    const claims = [1, 2].map(n => ({ slot: String(n), attemptId: String(n).repeat(32), graphId: null, nodeId: null, executorKind: 'native', contextId: n === 1 ? 'codex-native' : 'claude-native', capturedDeclaredAccountId: null, capturedLinkRevision: null, legacyBarrier: false }));
    check('actual empty owner observation', observed, true);
    for (const count of [1, 2]) {
      const metadata = clone(); metadata.capacity.activeClaims = structuredClone(claims.slice(0, count));
      metadata.capacity.occupiedSlots = String(count); metadata.capacity.activeAttemptId = count === 1 ? claims[0].attemptId : null;
      check('controlled ' + count + '-claim metadata', metadata, true);
    }
    for (const change of ['missing_graph', 'old_workflow', 'missing_capability', 'old_limit', 'missing_count', 'numeric_count', 'noncanonical_count', 'missing_claims', 'null_claims', 'missing_marker', 'duplicate_slot', 'duplicate_attempt', 'unordered_slots', 'missing_nullable', 'unknown_claim_field', 'invalid_revision', 'null_barrier', 'wrong_sole_marker', 'local_native_context', 'local_native_account', 'local_native_link', 'unpaired_account', 'unpaired_link', 'barrier_with_sibling', 'barrier_slot_two', 'native_missing_context', 'graph_local_without_graph', 'graph_local_with_node', 'graph_nonlocal_without_node']) {
      const metadata = clone(); const pool = metadata.capacity;
      pool.activeClaims = structuredClone(claims); pool.occupiedSlots = '2'; pool.activeAttemptId = null;
      switch (change) {
        case 'missing_graph': delete metadata.graphContract; break;
        case 'old_workflow': metadata.workflowContract = '005-v1'; break;
        case 'missing_capability': metadata.capabilities = metadata.capabilities.filter(c => c !== 'task_graphs'); break;
        case 'old_limit': pool.limit = 1; break;
        case 'missing_count': delete pool.occupiedSlots; break;
        case 'numeric_count': pool.occupiedSlots = 2; break;
        case 'noncanonical_count': pool.occupiedSlots = '02'; break;
        case 'missing_claims': delete pool.activeClaims; break;
        case 'null_claims': pool.activeClaims = null; break;
        case 'missing_marker': delete pool.activeAttemptId; break;
        case 'duplicate_slot': pool.activeClaims[1].slot = '1'; break;
        case 'duplicate_attempt': pool.activeClaims[1].attemptId = claims[0].attemptId; break;
        case 'unordered_slots': pool.activeClaims.reverse(); break;
        case 'missing_nullable': delete pool.activeClaims[0].graphId; break;
        case 'unknown_claim_field': pool.activeClaims[0].futureAuthority = true; break;
        case 'invalid_revision': pool.activeClaims[0].capturedLinkRevision = '9223372036854775808'; break;
        case 'null_barrier': pool.activeClaims[0].legacyBarrier = null; break;
        case 'wrong_sole_marker': pool.activeAttemptId = claims[0].attemptId; break;
        case 'local_native_context': pool.activeClaims[0].executorKind = 'local_check'; break;
        case 'local_native_account': Object.assign(pool.activeClaims[0], { executorKind: 'local_application', contextId: null, capturedDeclaredAccountId: '3'.repeat(32), capturedLinkRevision: '1' }); break;
        case 'local_native_link': Object.assign(pool.activeClaims[0], { executorKind: 'local_external', contextId: null, capturedLinkRevision: '1' }); break;
        case 'unpaired_account': pool.activeClaims[0].capturedDeclaredAccountId = '3'.repeat(32); break;
        case 'unpaired_link': pool.activeClaims[0].capturedLinkRevision = '1'; break;
        case 'barrier_with_sibling': pool.activeClaims[0].legacyBarrier = true; break;
        case 'barrier_slot_two': pool.activeClaims = [{ ...claims[0], slot: '2', legacyBarrier: true }]; pool.occupiedSlots = '1'; pool.activeAttemptId = claims[0].attemptId; break;
        case 'native_missing_context': pool.activeClaims[0].contextId = null; break;
        case 'graph_local_without_graph': Object.assign(pool.activeClaims[0], { executorKind: 'local_graph', contextId: null }); break;
        case 'graph_nonlocal_without_node': pool.activeClaims[0].graphId = '3'.repeat(32); break;
        case 'graph_local_with_node': Object.assign(pool.activeClaims[0], { executorKind: 'local_graph', contextId: null, graphId: '3'.repeat(32), nodeId: '4'.repeat(32) }); break;
      }
      check(change, metadata, false);
    }
    for (const kind of ['local_check', 'local_application', 'local_external', 'local_graph', 'legacy_unknown_native', 'linked_native', 'graph_native']) {
      const metadata = clone(); const claim = { ...claims[0] };
      metadata.capacity = { resourceId: 'managed-native', limit: 2, occupiedSlots: '1', activeClaims: [claim], activeAttemptId: claim.attemptId };
      if (kind === 'legacy_unknown_native') Object.assign(claim, { contextId: null, legacyBarrier: true });
      else if (kind === 'graph_native') Object.assign(claim, { graphId: '3'.repeat(32), nodeId: '4'.repeat(32) });
      else if (kind === 'linked_native') Object.assign(claim, { capturedDeclaredAccountId: '3'.repeat(32), capturedLinkRevision: '9223372036854775807' });
      else { Object.assign(claim, { executorKind: kind, contextId: null }); if (kind === 'local_graph') claim.graphId = '3'.repeat(32); }
      check('supported scope ' + kind, metadata, true);
    }
    for (const version of [2, 3, 4, 5]) {
      const metadata = clone(); metadata.appVersion = '2.0.0-dev.' + version; metadata.executionContract = '00' + version + '-v1';
      metadata.capacity = { resourceId: 'managed-native', limit: 1, activeAttemptId: null };
      metadata.capabilities = ['workspace_foundation', 'managed_execution', 'economical_routing', 'verified_delivery'];
      delete metadata.graphContract; delete metadata.workflowContract;
      if (version === 5) metadata.workflowContract = '005-v1';
      if (version === 2) { metadata.managedExecution = 'native_codex'; metadata.capacity = { connectionId: 'codex-native', limit: 1, activeAttemptId: null }; metadata.capabilities = ['workspace_foundation', 'managed_execution']; delete metadata.maintenanceContract; }
      check('known dev.' + version, metadata, false, true);
      metadata.appVersion = '2.0.0-dev.99'; check('unknown from dev.' + version, metadata, false);
    }
    return results;
  }, actual);
  for (const result of cases) assert.deepEqual({ execute: result.execute, stop: result.stop }, result.expected, result.name);
  assert.equal(writes, 0, 'metadata qualification must stay observational');
});
