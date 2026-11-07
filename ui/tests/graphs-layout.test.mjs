import assert from 'node:assert/strict';
import test from 'node:test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { fixture, fixtureRequestId, openWorkspace, ownerAPI, pollOwner } from './helpers.mjs';

const root = fileURLToPath(new URL('../../', import.meta.url));

async function layoutEvidence(page, directory, stage) {
  const observed = await page.evaluate(() => {
    const viewport = innerWidth, inspector = document.getElementById('graph-input-inspector');
    const selector = node => {
      const parts = [];
      while (node instanceof Element && parts.length < 5) {
        if (node.id) { parts.unshift('#' + node.id); break; }
        const parent = node.parentElement;
        parts.unshift(node.tagName.toLowerCase() + (parent ? ':nth-child(' + ([...parent.children].indexOf(node) + 1) + ')' : ''));
        node = parent;
      }
      return parts.join(' > ');
    };
    const boxes = [...document.querySelectorAll('body *')].flatMap(node => {
      const box = node.getBoundingClientRect(), css = getComputedStyle(node);
      if (!box.width || !box.height || css.visibility === 'hidden' || box.right <= viewport && box.left >= 0) return [];
      return [{ selector: selector(node), inInspector: !!inspector?.contains(node), tag: node.tagName,
        text: node.textContent?.slice(0, 240), left: box.left, right: box.right, width: box.width,
        clientWidth: node.clientWidth, scrollWidth: node.scrollWidth, display: css.display,
        minInlineSize: css.minInlineSize, maxInlineSize: css.maxInlineSize,
        whiteSpace: css.whiteSpace, overflowWrap: css.overflowWrap, overflowX: css.overflowX }];
    });
    return { viewport, content: document.documentElement.scrollWidth, offendingBoxes: boxes.slice(0, 80) };
  });
  await writeFile(path.join(directory, stage + '.json'), JSON.stringify(observed, null, 2) + '\n');
  await page.locator('#graph-input-inspector').scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(directory, stage + '-viewport.png'), animations: 'disabled' });
  await page.locator('#graph-input-inspector').screenshot({ path: path.join(directory, stage + '-inspector.png'), animations: 'disabled' });
  assert.ok(observed.content <= observed.viewport,
    stage + ': horizontal overflow ' + observed.content + ' > ' + observed.viewport + '; observed boxes: ' + JSON.stringify(observed.offendingBoxes));
  assert.equal(observed.offendingBoxes.filter(box => box.inInspector).length, 0, stage + ': required input evidence must fit the viewport');
}

test('complete composed input sources, provenance and raw bytes fit a 360px viewport without clipping evidence', { timeout: 90000 }, async t => {
  const f = await fixture(t), owner = await f.start();
  const opened = await ownerAPI(owner, '/projects/open', 'POST', { requestId: fixtureRequestId(), selectedPath: f.project, basePath: f.project });
  assert.equal(opened.status, 201);
  const original = '\ufeffComplete receiver original 東京🙂\r\nPreserve every retained source and exact staged intent.';
  const shared = 'Complete graph-shared original π\r\nInspect all complete sources; keep local working bytes and index unchanged.';
  const declaration = (key, fakeScenario, harnessId) => ({ key, originalText: key === 'final' ? original : 'Complete controlled original for ' + key + ' 東京🙂. Inspect the exact supplied context.',
    fakeScenario, pins: { harnessId }, acceptance: { deliverable: 'candidate' }, workflow: { review: 'required' } });
  const created = await ownerAPI(owner, '/graphs', 'POST', { requestId: fixtureRequestId(), projectId: opened.envelope.data.project.id,
    sharedInstructions: shared, executionMode: 'fake', nodes: [declaration('a', 'graph-branch-a', 'codex'), declaration('b', 'graph-branch-b', 'claude-code'), declaration('final', 'graph-join-no-edit', 'codex')],
    dependencies: [{ from: 'a', to: 'final', kind: 'files' }, { from: 'b', to: 'final', kind: 'files' }], finalNodeKey: 'final',
    routing: { budget: { admissionLimit: '6', windowSeconds: 900 } } });
  assert.equal(created.status, 201, JSON.stringify(created.envelope.error));
  const graphId = created.envelope.data.graph.id;
  const settled = await pollOwner(owner, '/graphs/' + graphId, data => data.graph.state === 'completed' && data.activeClaims.length === 0, { timeoutMs: 60000 });
  const final = settled.nodes.find(node => node.key === 'final');
  const inputResponse = await ownerAPI(owner, '/graphs/' + graphId + '/inputs/' + final.inputId);
  assert.equal(inputResponse.status, 200);
  const input = inputResponse.envelope.data;
  assert.equal(input.summary.state, 'ready'); assert.equal(input.inputBinding.contributions.length, 2);
  assert.ok(input.contextPacket.contributedEvidence.length > 0, 'the actual composed packet must include its long attributed source actions');
  const directory = path.join(root, '.council/refactor/screenshots/006-layout-regression', fixtureRequestId());
  await mkdir(directory, { recursive: true });
  await writeFile(path.join(directory, 'identities.json'), JSON.stringify({ graphId, nodeId: final.id, inputId: final.inputId }, null, 2) + '\n');
  t.diagnostic('Actual mobile layout evidence: ' + directory);
  const page = await openWorkspace(f, owner, { fake: false, workflow: 'auto' });
  await page.setViewportSize({ width: 360, height: 1000 });
  let writes = 0;
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) writes++; });
  await page.locator('#open-graph-list').click({ timeout: 5000 });
  await page.locator('#graph-list button').filter({ hasText: graphId }).click({ timeout: 5000 });
  await page.getByRole('button', { name: 'Inputs and context final', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(id => document.getElementById('graph-input-facts')?.textContent.includes(id), final.inputId, { timeout: 5000 });
  await layoutEvidence(page, directory, 'complete-composed-input');

  for (const item of input.contextPacket.contributedEvidence) {
    const action = page.getByRole('button', { name: 'Read contributed evidence ' + item.ref + ' · ' + item.scope, exact: true });
    assert.equal(await action.count(), 1, 'the complete attributed source action remains available');
    assert.ok((await action.textContent()).includes(item.ref));
  }
  await page.locator('#graph-members-load').click({ timeout: 5000 });
  await page.locator('#graph-members-list > li').first().waitFor({ timeout: 5000 });
  assert.match(await page.locator('#graph-members-list').textContent(), /sealed head [a-f0-9]{64}/);
  await layoutEvidence(page, directory, 'complete-member-provenance');
  await page.getByRole('button', { name: 'Read complete receiver original', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(text => document.getElementById('graph-input-artifact')?.textContent === text, original, { timeout: 5000 });
  await layoutEvidence(page, directory, 'complete-original-bytes');
  await page.getByRole('button', { name: 'Read complete member provenance', exact: true }).click({ timeout: 5000 });
  await page.waitForFunction(() => document.getElementById('graph-input-artifact-status')?.textContent.includes('Complete exact artifact'), undefined, { timeout: 5000 });
  const provenance = await page.locator('#graph-input-artifact').textContent();
  assert.ok(provenance.includes('council-graph-a.txt') && provenance.includes('council-graph-b.txt'));
  await layoutEvidence(page, directory, 'complete-raw-provenance');
  assert.equal(writes, 0, 'responsive input inspection must remain read-only');
  assert.equal(await readFile(path.join(f.project, 'hello.txt'), 'utf8'), 'unstaged\n');
  assert.deepEqual(await readFile(path.join(f.project, '.git/index')), f.indexBefore);
});
