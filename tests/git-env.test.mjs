import test from 'node:test';
import assert from 'node:assert/strict';
import { sanitizedGitEnvironment } from '../dist/git-env.js';

test('Git child environments omit checkout/index/config overrides without mutating or exposing caller values', () => {
  const source = {
    GIT_DIR: '/other/repo.git', git_index_file: '/other/index', GiT_WoRk_TrEe: '/other/tree',
    GIT_COMMON_DIR: '/other/common', GIT_OBJECT_DIRECTORY: '/other/objects',
    GIT_ALTERNATE_OBJECT_DIRECTORIES: '/other/alternate', GIT_NAMESPACE: 'unrelated',
    GIT_CONFIG: '/other/config', GIT_CONFIG_COUNT: '1', GIT_CONFIG_KEY_0: 'core.worktree', GIT_CONFIG_VALUE_0: '/other/tree',
    GIT_LITERAL_PATHSPECS: '1', GIT_GLOB_PATHSPECS: '1', git_noglob_pathspecs: '1', GiT_IcAsE_PaThSpEcS: '1',
    GIT_CONFIG_PARAMETERS: "'core.worktree=/other/tree'", GIT_CONFIG_GLOBAL: '/other/global',
    GIT_SSH_COMMAND: 'custom-ssh', GIT_SSH: 'ssh', GIT_ASKPASS: 'askpass',
    GIT_SSL_CAINFO: '/trusted/ca', HTTPS_PROXY: 'http://proxy.invalid', PATH: '/tools', HOME: '/home/example',
  };
  const copy = { ...source };
  const result = sanitizedGitEnvironment(source);
  assert.deepEqual(result, {
    GIT_SSH_COMMAND: 'custom-ssh', GIT_SSH: 'ssh', GIT_ASKPASS: 'askpass',
    GIT_SSL_CAINFO: '/trusted/ca', HTTPS_PROXY: 'http://proxy.invalid', PATH: '/tools', HOME: '/home/example',
  });
  assert.deepEqual(source, copy);
});
