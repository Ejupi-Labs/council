import { addFact, api, element, errorMessage, feedback, timeLabel } from './api.js';
import type { DiscoveryEntry, DiscoveryResult, HarnessId } from './types.js';
import type { ExecutionCapabilities } from './execution-types.js';
import { appendConnectionFacts } from './native-observations.js';

const harnesses: readonly [HarnessId, string][] = [
  ['codex', 'Codex'], ['claude-code', 'Claude Code'], ['deepseek-harness', 'DeepSeek Harness'],
  ['junie', 'Junie'], ['antigravity', 'Antigravity'],
];

export class AgentsView {
  private generation = 0;
  private lastResult: DiscoveryResult | null = null;
  private capabilities: ExecutionCapabilities | null = null;
  private readonly refreshButton = element<HTMLButtonElement>('refresh-agents');

  constructor(private readonly chooseTarget: (id: 'codex' | 'claude-code') => void, private readonly onCapabilities: (capabilities: ExecutionCapabilities) => void) {
    this.refreshButton.addEventListener('click', () => { void this.refresh(); });
    this.render();
  }

  async refresh(): Promise<void> {
    const generation = ++this.generation;
    this.refreshButton.disabled = true;
    element('agents-list').setAttribute('aria-busy', 'true');
    feedback('agents-status', this.lastResult
      ? 'Refreshing installation metadata. Previous observations remain visible below.'
      : 'Inspecting local installation metadata. No agent is being started.');
    try {
      const [discovery, declared] = await Promise.allSettled([api<DiscoveryResult>('/agents'), api<ExecutionCapabilities>('/execution/capabilities')]);
      if (generation !== this.generation) return;
      if (declared.status === 'fulfilled') { this.capabilities = declared.value.data; this.onCapabilities(declared.value.data); }
      if (discovery.status === 'rejected') throw discovery.reason;
      const data = discovery.value.data; this.lastResult = data;
      this.render();
      const hasErrors = data.observations.some(entry => entry.error !== null);
      feedback('agents-status', 'Observed ' + timeLabel(data.observedAt) + '. '
        + (hasErrors ? 'Some installations could not be inspected. See each card for the next action.' : 'No live connection or account access was verified.') + (declared.status === 'rejected' ? ' Current capability declarations could not be refreshed; prior declarations may be shown.' : ''), hasErrors ? 'error' : 'neutral');
    } catch (error) {
      if (generation !== this.generation) return;
      feedback('agents-status', errorMessage(error) + (this.lastResult
        ? ' Displaying previous observations from ' + timeLabel(this.lastResult.observedAt) + '; these are stale.'
        : ' Refresh installations to try again.'), 'error');
    } finally {
      if (generation === this.generation) {
        this.refreshButton.disabled = false;
        element('agents-list').setAttribute('aria-busy', 'false');
      }
    }
  }

  private render(): void {
    const list = element('agents-list');
    list.replaceChildren();
    for (const [id, name] of harnesses) {
      const entry = this.lastResult?.observations.find(item => item.harnessId === id);
      list.append(this.card(id, name, entry));
    }
  }

  private card(id: HarnessId, name: string, entry?: DiscoveryEntry): HTMLElement {
    const card = document.createElement('article');
    card.className = 'panel agent-card';
    card.dataset['harness'] = id;
    card.dataset['testid'] = 'agent-' + id;
    const header = document.createElement('header');
    const image = document.createElement('img');
    image.src = '/assets/icons/plug.svg'; image.alt = ''; image.width = 24; image.height = 24;
    const title = document.createElement('h3'); title.textContent = name;
    header.append(image, title);
    const status = document.createElement('p'); status.className = 'status-label';
    const nextAction = document.createElement('p');
    const observation = entry?.observation;
    if (entry?.error) {
      status.textContent = 'Discovery unavailable';
      status.dataset['status'] = 'unsupported_launcher';
      nextAction.textContent = entry.error.message;
    } else if (observation) {
      status.dataset['status'] = observation.installationStatus;
      const labels = { installed: 'Installation found', missing: 'Installation missing', unsupported_launcher: 'Unsupported launcher' };
      status.textContent = labels[observation.installationStatus];
      const actions = {
        install_with_vendor: 'Install using the vendor’s instructions, then refresh installations.',
        review_launcher_support: 'Review the vendor’s native launcher options. Shell wrappers are unsupported.',
        await_managed_execution: 'Installation found. Managed execution for this harness follows a later feature.',
        start_explicit_task: 'Native execution is supported. Start a task explicitly; live provider compatibility remains unqualified.',
      };
      nextAction.textContent = actions[observation.nextAction];
    } else {
      status.textContent = 'Not inspected';
      nextAction.textContent = 'Connect to the local owner to inspect installation metadata.';
    }
    const facts = document.createElement('dl'); facts.className = 'facts';
    for (const label of ['Authentication', 'Model', 'Allowance', 'Effective permissions']) addFact(facts, label, 'Unknown');
    const limitation = document.createElement('p'); limitation.className = 'agent-limitation';
    limitation.textContent = observation?.managedExecution === 'native_supported_live_unqualified' ? 'Managed execution: native ' + name + ' supported · live provider unqualified' : 'Managed execution: not implemented';
    const details = document.createElement('details');
    const summary = document.createElement('summary'); summary.textContent = 'Observation details';
    details.append(summary);
    for (const value of [
      observation?.launcherPath ? 'Launcher candidate: ' + observation.launcherPath : 'Launcher candidate: not established',
      observation ? 'Observed: ' + timeLabel(observation.observedAt) : 'No current installation observation.',
      'Source: PATH and native installation metadata',
      'Provider, account, external occupancy and usage: unknown',
      'No vendor executable or account probe was run.',
    ]) {
      const paragraph = document.createElement('p'); paragraph.textContent = value; details.append(paragraph);
    }
    const profile = this.capabilities?.connections.find(connection => connection.harnessId === id);
    if (profile) {
      const declaration = document.createElement('dl'); declaration.className = 'facts'; appendConnectionFacts(declaration, profile);
      addFact(declaration, 'Observed installed version', 'Unknown. Installation metadata does not verify the native protocol.'); details.append(declaration);
    }
    card.append(header, status, nextAction, facts, limitation, details);
    if (profile && (id === 'codex' || id === 'claude-code')) {
      const choose = document.createElement('button'); choose.type = 'button'; choose.className = 'secondary';
      choose.textContent = 'Use ' + name + ' for next task'; choose.addEventListener('click', () => this.chooseTarget(id)); card.append(choose);
    }
    return card;
  }
}
