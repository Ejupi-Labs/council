import type { WorkflowInput } from './types.js';
import { parseRoutingInput, parseRoutingProfile } from './routing-schema.js';
import { RoutingError } from './routing-types.js';
import type { RoutingAlternative, RoutingContext, RoutingEscalation, RoutingInput, RoutingPlan, RoutingProfile, RoutingQuotaView, RoutingStrategy, RoutingTier } from './routing-types.js';

const tiers: RoutingTier[] = ['small', 'standard', 'large'];
const preferredTiers = { routine: 'small', standard: 'standard', complex: 'large' } as const;
interface Candidate { alias: string; profile: RoutingProfile | null; quota: RoutingQuotaView; eligible: boolean; reason: string }
const lexical = (left: string, right: string): number => left < right ? -1 : left > right ? 1 : 0;
const cost = (candidate: Candidate): number => candidate.profile?.costRank ?? Infinity;
const quality = (candidate: Candidate): number => candidate.profile?.qualityRank ?? -Infinity;
const compare = (left: number, right: number): number => left < right ? -1 : left > right ? 1 : 0;

function quota(context: RoutingContext, alias: string): RoutingQuotaView {
  const value = typeof context.availability === 'function' ? context.availability(alias) : context.availability?.[alias];
  if (!value) return { availability: 'unknown', fresh: false, reason: 'No current account quota observation is available.' };
  if (!['available', 'exhausted', 'unknown'].includes(value.availability) || typeof value.fresh !== 'boolean'
    || value.dispatchBlocked !== undefined && typeof value.dispatchBlocked !== 'boolean') throw new RoutingError('ROUTING_INVALID_INPUT', `Invalid quota observation for ${alias}.`);
  return !value.fresh ? { ...value, availability: 'unknown' } : { ...value };
}

/** User ordinal ranks are compared only within one tier, without inferred prices. */
function ordered(pool: Candidate[], strategy: RoutingStrategy): Candidate[] {
  const costs = [...new Set(pool.map(cost).filter(Number.isFinite))].sort((a, b) => a - b);
  const qualities = [...new Set(pool.map(quality).filter(Number.isFinite))].sort((a, b) => b - a);
  const position = (candidate: Candidate): [number, number] => [
    Number.isFinite(cost(candidate)) ? costs.indexOf(cost(candidate)) : pool.length,
    Number.isFinite(quality(candidate)) ? qualities.indexOf(quality(candidate)) : pool.length,
  ];
  return [...pool].sort((left, right) => {
    let preference: number;
    if (strategy === 'balanced') {
      const [lc, lq] = position(left), [rc, rq] = position(right);
      preference = Math.max(lc, lq) - Math.max(rc, rq) || lc + lq - rc - rq;
    } else preference = strategy === 'economy' ? compare(cost(left), cost(right)) : compare(quality(right), quality(left));
    return preference || compare(cost(left), cost(right)) || compare(quality(right), quality(left))
      || Number(right.quota.availability === 'available') - Number(left.quota.availability === 'available') || lexical(left.alias, right.alias);
  });
}

function alternatives(candidates: Candidate[], selected: Set<string>, preferred: RoutingTier): RoutingAlternative[] {
  return [...candidates].sort((a, b) => lexical(a.alias, b.alias)).map(candidate => ({
    alias: candidate.alias, eligible: candidate.eligible, selected: selected.has(candidate.alias), tier: candidate.profile?.tier ?? null,
    costRank: candidate.profile?.costRank ?? null, qualityRank: candidate.profile?.qualityRank ?? null, quota: { ...candidate.quota },
    reason: selected.has(candidate.alias) ? 'Selected by this plan.' : !candidate.eligible ? candidate.reason : !candidate.profile
      ? 'Automatic routing requires a user-configured tier and routing profile.' : candidate.profile.tier !== preferred
        ? `Configured ${candidate.profile.tier} tier differs from the preferred ${preferred} tier.` : 'Eligible alternative under the configured preference ranks.',
  }));
}

/** Deterministic dry routing: no agent dispatch, task inference, prices, or vendor defaults. */
export function planRouting(value: RoutingInput, context: RoutingContext): RoutingPlan {
  let input: RoutingInput;
  try { input = parseRoutingInput(value); }
  catch (error) { throw new RoutingError('ROUTING_INVALID_INPUT', error instanceof Error ? error.message : 'Invalid routing input.'); }
  if (!input.task.trim()) throw new RoutingError('ROUTING_INVALID_INPUT', 'Routing task must contain non-whitespace text.');
  const strategy = input.strategy!, collaboration = input.collaboration!, preferred = preferredTiers[input.taskClass];
  const requirements = [...new Set(input.requiredCapabilities!.map(capability => capability.trim()))];
  if (requirements.some(capability => !capability)) throw new RoutingError('ROUTING_INVALID_INPUT', 'Required capabilities must contain non-whitespace text.');
  if (input.skills && new Set(input.skills).size !== input.skills.length) throw new RoutingError('ROUTING_INVALID_INPUT', 'Each assigned skill must be unique.');
  const settingsAliases = Object.keys(input.modelSettings ?? {});
  const explicit = input.agents ?? (settingsAliases.length ? settingsAliases : undefined);
  if (explicit && new Set(explicit).size !== explicit.length) throw new RoutingError('ROUTING_INVALID_INPUT', 'Each selected alias must be unique.');
  if (explicit && settingsAliases.some(alias => !explicit.includes(alias))) throw new RoutingError('ROUTING_INVALID_INPUT', 'Model settings must refer to an explicitly selected alias.');
  const count = input.participants ?? explicit?.length ?? (collaboration === 'direct' ? 1 : 2);
  if (explicit && explicit.length !== count) throw new RoutingError('ROUTING_INVALID_INPUT', 'Explicit aliases and participants must specify the same team size.');
  if (collaboration === 'direct' && count !== 1) throw new RoutingError('ROUTING_INVALID_INPUT', 'The direct preset requires exactly one selected alias.');
  if (['review', 'council'].includes(collaboration) && count < 2) throw new RoutingError('ROUTING_INVALID_INPUT', `${collaboration} requires at least two selected aliases.`);

  const candidates: Candidate[] = Object.entries(context.agents).map(([alias, agent]) => {
    let profile: RoutingProfile | null = null;
    if (agent.enabled && agent.routing != null) {
      try { profile = parseRoutingProfile(agent.routing); }
      catch (error) { throw new RoutingError('ROUTING_CONFIG_REQUIRED', `Invalid routing profile for ${alias}: ${error instanceof Error ? error.message : 'Invalid profile.'}`); }
    }
    const observation = quota(context, alias);
    const missing = requirements.filter(capability => !profile?.capabilities.map(value => value.trim()).includes(capability));
    const reason = !agent.enabled ? 'Agent alias is disabled.'
      : observation.dispatchBlocked ? `Current subscription protection blocks dispatch.${observation.reason ? ` ${observation.reason}` : ''}`
      : observation.availability === 'exhausted' ? 'A fresh account quota observation is exhausted.'
      : missing.length ? `Required capabilities are not configured: ${missing.join(', ')}.` : '';
    return { alias, profile, quota: observation, eligible: !reason, reason };
  });
  const describe = (selected: Candidate[] = []) => alternatives(candidates, new Set(selected.map(candidate => candidate.alias)), preferred);
  let selected: Candidate[];
  if (explicit) {
    selected = explicit.map(alias => {
      const candidate = candidates.find(candidate => candidate.alias === alias);
      if (!candidate || !candidate.eligible) throw new RoutingError('ROUTING_UNAVAILABLE', `Explicit alias ${alias} cannot be used: ${candidate?.reason ?? 'Alias is not configured.'} No alias or model was substituted.`, describe());
      return candidate;
    });
  } else {
    const suitable = candidates.filter(candidate => candidate.eligible && candidate.profile);
    const preferredPool = ordered(suitable.filter(candidate => candidate.profile!.tier === preferred), strategy);
    const higher = tiers.slice(tiers.indexOf(preferred) + 1).flatMap(tier => ordered(suitable.filter(candidate => candidate.profile!.tier === tier), strategy));
    if (preferredPool.length < count && !input.allowEscalation && preferredPool.length + higher.length >= count) {
      const escalation: RoutingEscalation = { preferredTier: preferred, targetTiers: [...new Set(higher.map(candidate => candidate.profile!.tier))], allowed: false,
        reason: `The ${preferred} tier has ${preferredPool.length} suitable aliases; ${count} are required. Explicitly allow escalation or select aliases.` };
      throw new RoutingError('ROUTING_ESCALATION_REQUIRED', escalation.reason, describe(), escalation);
    }
    const pool = input.allowEscalation ? [...preferredPool, ...higher] : preferredPool;
    if (pool.length < count) {
      const unconfigured = candidates.some(candidate => context.agents[candidate.alias]!.enabled && !candidate.profile);
      throw new RoutingError(unconfigured ? 'ROUTING_CONFIG_REQUIRED' : 'ROUTING_UNAVAILABLE',
        `Routing needs ${count} enabled, capable ${preferred}-tier aliases${input.allowEscalation ? ' or explicitly permitted higher tiers' : ''}, but found ${pool.length}. ${unconfigured ? 'Configure routing profiles or select exact aliases; model size and cost are never inferred.' : 'Review capability requirements and current quota observations.'}`, describe());
    }
    selected = pool.slice(0, count);
  }

  const warnings: string[] = [], reasons = [`Task class ${input.taskClass} explicitly prefers the configured ${preferred} tier.`,
    explicit ? 'Exact caller-selected aliases and model settings are preserved.' : `The ${strategy} strategy compares user-configured ranks within the preferred tier; no task classifier or pricing lookup was used.`];
  const targetTiers = [...new Set(selected.filter(candidate => candidate.profile && tiers.indexOf(candidate.profile.tier) > tiers.indexOf(preferred)).map(candidate => candidate.profile!.tier))];
  const escalation: RoutingEscalation | undefined = targetTiers.length ? { preferredTier: preferred, targetTiers, allowed: true,
    reason: explicit ? 'Higher-tier aliases were explicitly selected by the caller.' : 'The caller explicitly allowed higher-tier routing to complete the requested team.' } : undefined;
  const agentSettings: NonNullable<WorkflowInput['agentSettings']> = {};
  for (const candidate of selected) {
    const agent = context.agents[candidate.alias]!, override = input.modelSettings?.[candidate.alias];
    const model = override?.model ?? agent.model, effort = override?.effort ?? agent.effort;
    const skills = override?.skills ?? input.skills;
    Object.defineProperty(agentSettings, candidate.alias, { value: { ...(model !== undefined ? { model } : {}), ...(effort !== undefined ? { effort } : {}), ...(skills !== undefined ? { skills: [...skills] } : {}) },
      enumerable: true, writable: true, configurable: true });
    if (candidate.quota.availability === 'unknown') warnings.push(`${candidate.alias}: account quota is unknown${candidate.quota.reason ? ` (${candidate.quota.reason})` : ''}; availability is not verified.`);
    if (model === undefined) warnings.push(`${candidate.alias}: no explicit model is pinned. The provider or CLI default may change; recheck the configured routing tier, capabilities and comparative ranks when it does. No model was substituted.`);
    if (!candidate.profile) warnings.push(`${candidate.alias}: routing tier, capabilities and comparative ranks are unconfigured; its explicit selection was preserved.`);
    else {
      if (candidate.profile.costRank === null) warnings.push(`${candidate.alias}: cost rank is unknown; no cheaper-than claim is made.`);
      if (candidate.profile.qualityRank === null) warnings.push(`${candidate.alias}: quality rank is unknown.`);
    }
    if (override?.model !== undefined && override.model !== agent.model) warnings.push(`${candidate.alias}: explicit model override is preserved; the configured alias tier, cost/quality ranks and capabilities are not verified for this overridden model.`);
    reasons.push(`${candidate.alias}: ${explicit ? 'explicitly selected' : `suitable ${candidate.profile!.tier} tier`}; quota ${candidate.quota.availability}${candidate.profile ? `; configured cost rank ${candidate.profile.costRank ?? 'unknown'}, quality rank ${candidate.profile.qualityRank ?? 'unknown'}` : ''}.`);
  }
  if (escalation) reasons.push(escalation.reason);
  if (input.rounds !== undefined) warnings.push('The selected collaboration preset does not use debate rounds; the supplied rounds value is retained as workflow metadata.');
  const aliases = selected.map(candidate => candidate.alias);
  const judge = collaboration === 'council' ? [...selected].sort((a, b) => compare(quality(b), quality(a)) || lexical(a.alias, b.alias))[0]!.alias : undefined;
  const workflow: WorkflowInput = {
    mode: collaboration === 'direct' ? 'delegate' : collaboration, task: input.task, agents: aliases, permission: input.permission!,
    agentSettings,
    ...(judge ? { judge } : {}), ...(input.context !== undefined ? { context: input.context } : {}),
    ...(input.contextLimit !== undefined ? { contextLimit: input.contextLimit } : {}),
    ...(input.maxCalls !== undefined ? { maxCalls: input.maxCalls } : {}), ...(input.rounds !== undefined ? { rounds: input.rounds } : {}),
  };
  let plan;
  try { plan = context.plan(structuredClone(workflow)); }
  catch (error) { throw new RoutingError('ROUTING_UNAVAILABLE', `Selected routing plan cannot run: ${error instanceof Error ? error.message : 'Workflow validation failed.'} No alias, model, or collaboration preset was substituted.`, describe(selected), escalation); }
  return {
    workflow, selectedAliases: [...aliases], selected: selected.map(candidate => ({ alias: candidate.alias, tier: candidate.profile?.tier ?? null,
      costRank: candidate.profile?.costRank ?? null, qualityRank: candidate.profile?.qualityRank ?? null, quota: { ...candidate.quota },
      ...(agentSettings[candidate.alias]?.model !== undefined ? { model: agentSettings[candidate.alias]!.model } : {}),
      ...(agentSettings[candidate.alias]?.effort !== undefined ? { effort: agentSettings[candidate.alias]!.effort } : {}),
    })), reasons, alternatives: describe(selected), warnings, estimatedCalls: plan.estimatedCalls,
    suggestedRoomMembers: selected.map((candidate, index) => ({ agent: candidate.alias, role: index === 0 ? 'lead' : 'worker', weight: context.agents[candidate.alias]!.authority ?? 1,
      ...structuredClone(agentSettings[candidate.alias]) })),
    ...(escalation ? { escalation } : {}),
  };
}
