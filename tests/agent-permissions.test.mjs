import test from 'node:test';
import assert from 'node:assert/strict';
import { CLAUDE_READ_TOOLS, taskPermissionGuidance } from '../dist/agent-permissions.js';

const config = (agent, backend = 'native') => ({ backend, agents: { peer: { enabled: true, ...agent } } });

test('native Claude read guidance names its actual allowlist and routes commands back to the host', () => {
  const guidance = taskPermissionGuidance(config({ profile: 'claude' }), 'peer', 'read');
  assert.match(guidance, /Available tools.*Read, Glob, Grep/);
  assert.match(guidance, /Do not call Bash/);
  assert.match(guidance, /even for read-only checks/);
  assert.match(guidance, /host-executed check.*report it unverified/);
  assert.deepEqual(CLAUDE_READ_TOOLS, ['Read', 'Glob', 'Grep']);
  assert.throws(() => CLAUDE_READ_TOOLS.push('Bash'), TypeError);
});

test('ACP guidance describes client file capability without promising a terminal or replacing native policy', () => {
  for (const agent of [{ profile: 'junie' }, { profile: 'claude', transport: 'acp' }]) {
    const guidance = taskPermissionGuidance(config(agent), 'peer', 'read');
    assert.match(guidance, /ACP client exposes scoped text-file reads and no terminal service/);
    assert.match(guidance, /unverified host-executed check/);
    assert.doesNotMatch(guidance, /Available tools.*Read, Glob, Grep/);
  }
});

test('write and other backends retain their actual permissions without native Claude restrictions', () => {
  assert.equal(taskPermissionGuidance(config({ profile: 'claude' }), 'peer', 'write'),
    'Make changes only in the provided isolated worktree.');
  for (const backend of ['mock', 'acpx']) {
    assert.equal(taskPermissionGuidance(config({ profile: 'claude' }, backend), 'peer', 'read'),
      'Read-only task: do not modify files.');
  }
  assert.equal(taskPermissionGuidance(config({ profile: 'codex' }), 'peer', 'read'),
    'Read-only task: do not modify files.');
});
