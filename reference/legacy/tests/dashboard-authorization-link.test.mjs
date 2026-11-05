import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext, Script } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';

const source = /<script nonce="[^"]+">([\s\S]*?)<\/script>/.exec(dashboardHtml('authorization-fixture'))[1];
const lines = source.split(/\r?\n/);
function line(prefix) {
  const match = lines.find(value => value.startsWith(prefix));
  assert.ok(match, 'Missing authorization browser code: ' + prefix); return match;
}
const handler = line('function reloadAuthorizationLink()');
const registration = line("window.addEventListener('hashchange',");

function harness(token = '') {
  const listeners = new Map(), reloads = [], messages = [];
  const location = { hash: '', reload(...args) { reloads.push({ hash: this.hash, args }); } };
  const context = createContext({ URLSearchParams, location, token,
    window: { addEventListener: (event, listener) => listeners.set(event, listener) },
    fetch() { assert.fail('Authorization fragment recovery must not send network requests.'); },
    console: { log() { assert.fail('Authorization tokens must not be logged.'); } },
    connection() {}, error: message => messages.push(message),
  });
  // Execute the actual early-return guard after registering the actual listener.
  const earlyReturn = source.match(/if\(!token\)\{connection\(false,'Authorization required'\);error\('[^']+'\);return;\}/);
  assert.ok(earlyReturn);
  assert.ok(source.indexOf(registration) < earlyReturn.index);
  runInContext('(function(){' + handler + registration + earlyReturn[0] + '})();', context);
  return { location, reloads, messages,
    change(hash) { location.hash = hash; listeners.get('hashchange')(); },
  };
}

test('a full authorization link reloads a public shell even after its unauthenticated early return', () => {
  const wire = harness(); assert.match(wire.messages[0], /complete link/);
  wire.change('#token=fresh-local-token');
  assert.deepEqual(wire.reloads, [{ hash: '#token=fresh-local-token', args: [] }]);
  assert.doesNotThrow(() => new Script(source));
});

test('unrelated fragments and empty token parameters never reload the dashboard', () => {
  const wire = harness('previous-session-token');
  for (const hash of ['', '#', '#history', '#other=value', '#token', '#token=', '#other=value&token=']) wire.change(hash);
  assert.equal(wire.reloads.length, 0);
  wire.change('#other=value&token=encoded%2Btoken'); assert.equal(wire.reloads.length, 1);
  assert.equal(wire.reloads[0].hash, '#other=value&token=encoded%2Btoken');
});

test('a fresh link in an old authorized tab reuses normal initialization and removes the token fragment', () => {
  const wire = harness('obsolete-token'); wire.change('#token=fresh-token');
  assert.equal(wire.reloads.length, 1);
  const origin = 'http://127.0.0.1:53160', storageKey = 'council-dashboard-token:' + origin;
  const stored = new Map([[storageKey, 'obsolete-token']]), replacements = [];
  const location = { origin, hash: wire.reloads[0].hash, pathname: '/', search: '' };
  const context = createContext({ URLSearchParams, location,
    sessionStorage: { getItem: key => stored.get(key), setItem: (key, value) => stored.set(key, value) },
    history: { replaceState(_state, _unused, url) { replacements.push(url); location.hash = ''; } },
    fetch() { assert.fail('Reading a link token must not send it in a request URL.'); },
  });
  runInContext([line("var token='',storageKey="), line('try{token=new URLSearchParams('), line('if(location.hash)history.replaceState(')].join('\n'), context);
  assert.equal(context.token, 'fresh-token'); assert.equal(stored.get(storageKey), 'fresh-token');
  assert.deepEqual(replacements, ['/']); assert.equal(location.hash, '');
});
