import { addFact, element, timeLabel } from './api.js';
import type { Attempt, ConnectionCapability, Fact, UsageObservation } from './execution-types.js';

export function harnessName(id?: string): string {
  if (!id) return 'Auto · no literal harness pin';
  return ({ codex: 'Codex', 'claude-code': 'Claude Code' } as Record<string, string>)[id] ?? id;
}

export function factLabel(value?: Fact): string {
  if (!value) return 'Not recorded';
  return value.status === 'observed' && value.value !== null
    ? value.value + ' · ' + (value.source ?? 'source unknown') + (value.observedAt ? ' · ' + timeLabel(value.observedAt) : '')
    : 'Unknown';
}

export function usageLabel(usage: UsageObservation): string {
  const scope = usage.scope === 'top_level' ? 'Top-level native usage' : usage.scope === 'whole_tree' ? 'Whole-tree native usage' : usage.scope === 'unknown' ? 'Scope unknown' : 'Scope not recorded';
  return scope + ' · input ' + (usage.inputTokens ?? 'unknown') + ' · output ' + (usage.outputTokens ?? 'unknown')
    + ' · cached input ' + (usage.cachedInputTokens ?? 'unknown') + ' · cache creation ' + (usage.cacheCreationInputTokens ?? 'unknown')
    + ' · ' + usage.completeness + (usage.source ? ' · ' + usage.source : '') + (usage.observedAt ? ' · ' + timeLabel(usage.observedAt) : '');
}

export function appendConnectionFacts(facts: HTMLElement, profile?: ConnectionCapability): void {
  if (!profile) {
    addFact(facts, 'Current connection declaration', 'Unavailable. This does not establish a native version or live compatibility.');
    return;
  }
  addFact(facts, 'Current native interface declaration', profile.nativeContract);
  addFact(facts, 'Declared version assumption', profile.minimumVersion ? 'Minimum ' + profile.minimumVersion + '; documented reference ' + (profile.documentedVersion ?? 'not specified') + ' · ' + profile.versionSource : 'No minimum version declared · ' + profile.versionSource);
  addFact(facts, 'Native conversation continuity', profile.sessionContinuity === 'unsupported' ? 'Unsupported. Retained Council tasks and history do not resume a native conversation.' : profile.sessionContinuity);
  addFact(facts, 'Native session persistence', profile.sessionPersistence.replaceAll('_', ' '));
  addFact(facts, 'Live provider qualification', profile.liveQualification === 'unqualified' ? 'Unqualified. Local fake and platform checks are separate evidence.' : profile.liveQualification);
}

export function renderNativeModels(attempt: Attempt, readBytes: (id: string) => void): void {
  const preview = attempt.targetObservations.nativeModels;
  const panel = element('native-models-panel'); panel.hidden = !preview;
  const facts = element('native-models-facts'); facts.replaceChildren();
  const models = element('native-models-list'); models.replaceChildren();
  const evidence = element('native-models-evidence'); evidence.replaceChildren();
  if (!preview) return;
  addFact(facts, 'Observation scope', 'Whole-tree native-reported model use. Separate from the actual primary model and top-level counters.');
  addFact(facts, 'Preview completeness', preview.completeness + ' · omitted models ' + (preview.omittedModelCount ?? 'unknown') + (preview.models.length > 16 ? ' · only the first 16 preview entries are shown' : ''));
  addFact(facts, 'Preview source', preview.source + ' · ' + timeLabel(preview.observedAt));
  for (const model of preview.models.slice(0, 16)) {
    const item = document.createElement('li');
    const heading = document.createElement('h4'); heading.textContent = model.model;
    const values = document.createElement('dl'); values.className = 'facts';
    addFact(values, 'Reported tokens', 'Input ' + (model.inputTokens ?? 'unknown') + ' · output ' + (model.outputTokens ?? 'unknown') + ' · cached input ' + (model.cachedInputTokens ?? 'unknown') + ' · cache creation ' + (model.cacheCreationInputTokens ?? 'unknown'));
    addFact(values, 'Native client estimate', model.costEstimateUsd === null ? 'Unknown. No billed charge is reported.' : model.costEstimateUsd + ' USD · estimate, not a billed charge or subscription allowance.');
    item.append(heading, values); models.append(item);
  }
  for (const id of preview.evidenceArtifactIds) {
    const button = document.createElement('button'); button.type = 'button'; button.className = 'secondary';
    button.textContent = 'Read raw model evidence ' + id.slice(0, 8); button.addEventListener('click', () => readBytes(id)); evidence.append(button);
  }
}
