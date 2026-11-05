import { lstatSync, readFileSync, realpathSync, statSync } from 'node:fs';
import { isUtf8 } from 'node:buffer';
import { isAbsolute, join, relative, resolve, sep } from 'node:path';
import { parseSchema, type JsonSchema } from './schema.js';
import { assertUniqueObjectKeys } from './control-json.js';
import { MAX_AGENT_JSON_DEPTH } from './agent-json.js';
import { modelMapDefinition, modelMapOverlayDefinition, modelMapQueryDefinition } from './model-map-schema.js';
import { MODEL_MAP_CATALOG } from './model-map-data.js';
import type { ModelMapCatalog, ModelMapConfig, ModelMapObservation, ModelMapOverlay, ModelMapQuery,
  ModelMapRanking, ModelMapSuggestion, ModelMapView, ModelProfile } from './model-map-types.js';

function nonempty(value: string, label: string): void {
  if (!value.trim()) throw new Error(`${label} must contain non-whitespace text.`);
}
function date(value: string, label: string): void {
  const parsed = new Date(value);
  if (!Number.isFinite(parsed.getTime()) || !/^\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}:\d{2}(?:\.\d{1,3})?(?:Z|[+-]\d{2}:\d{2}))?$/.test(value)) throw new Error(`${label} must be an ISO date or timestamp with a timezone.`);
  if (new Date(value.slice(0, 10)).toISOString().slice(0, 10) !== value.slice(0, 10)
    || value.includes('T') && (Number(value.slice(11, 13)) > 23 || Number(value.slice(14, 16)) > 59 || Number(value.slice(17, 19)) > 59)) {
    throw new Error(`${label} must be a valid calendar date and time.`);
  }
}
function unique(values: string[], label: string): void {
  for (const value of values) nonempty(value, label);
  if (new Set(values).size !== values.length) throw new Error(`${label} must be unique.`);
}
function byId<T extends { id: string }>(values: T[], label: string): Map<string, T> {
  unique(values.map(value => value.id), `${label} IDs`);
  return new Map(values.map(value => [value.id, value]));
}
function modelLookup(models: ModelProfile[]): Map<string, ModelProfile> {
  const lookup = new Map<string, ModelProfile>();
  for (const model of models) for (const identifier of [model.id, ...model.aliases ?? []]) {
    nonempty(identifier, 'Exact model ID or alias');
    if (lookup.has(identifier)) throw new Error(`Model ID or exact alias ${JSON.stringify(identifier)} is ambiguous.`);
    lookup.set(identifier, model);
  }
  return lookup;
}

/** Walk this fixed, acyclic schema without imposing a whole-catalog node or evidence-size cap. */
function parseCatalog<T>(schema: JsonSchema, value: unknown, label: string): T {
  const parse = (rule: JsonSchema, input: unknown, path: string): unknown => {
    if (input === undefined && rule.default !== undefined) input = structuredClone(rule.default);
    if (rule.type === 'array') {
      if (!Array.isArray(input)) throw new Error(`${path}: must be an array.`);
      if (rule.minItems !== undefined && input.length < rule.minItems) throw new Error(`${path}: must contain at least ${rule.minItems} entries.`);
      if (rule.maxItems !== undefined && input.length > rule.maxItems) throw new Error(`${path}: must contain at most ${rule.maxItems} entries.`);
      return input.map((entry, index) => parse(rule.items!, entry, `${path}[${index}]`));
    }
    if (rule.type === 'object') {
      if (!input || typeof input !== 'object' || Array.isArray(input) || ![Object.prototype, null].includes(Object.getPrototypeOf(input))) throw new Error(`${path}: must be a plain JSON object.`);
      const source = input as Record<string, unknown>, properties = rule.properties ?? {};
      for (const key of Object.keys(source)) if (!Object.hasOwn(properties, key)) throw new Error(`${path}: unknown property ${key}.`);
      for (const key of rule.required ?? []) if (!Object.hasOwn(source, key) || source[key] === undefined) throw new Error(`${path}: missing required property ${key}.`);
      const result: Record<string, unknown> = {};
      for (const [key, child] of Object.entries(properties)) {
        const entry = Object.hasOwn(source, key) ? source[key] : undefined;
        if (entry === undefined && child.default === undefined) continue;
        Object.defineProperty(result, key, { value: parse(child, entry, `${path}.${key}`), enumerable: true, writable: true, configurable: true });
      }
      return result;
    }
    return parseSchema(rule, input, path);
  };
  return parse(schema, value, label) as T;
}

/** Validate identities and evidence without translating model names or combining benchmark scores. */
export function validateModelMap(value: unknown): ModelMapCatalog {
  const catalog = parseCatalog<ModelMapCatalog>(modelMapDefinition, value, 'model map');
  date(catalog.asOf, 'Model map asOf');
  const sources = byId(catalog.sources, 'Source'), observations = byId(catalog.observations, 'Observation');
  const models = byId(catalog.models, 'Model'), tasks = byId(catalog.tasks, 'Task');
  modelLookup(catalog.models);
  for (const source of catalog.sources) {
    nonempty(source.title, 'Source title'); nonempty(source.publisher, 'Source publisher');
    date(source.retrievedAt, 'Source retrievedAt'); if (source.publishedAt) date(source.publishedAt, 'Source publishedAt');
    let url: URL;
    try { url = new URL(source.url); } catch { throw new Error(`Source ${source.id} must have an absolute HTTP or HTTPS URL.`); }
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) throw new Error(`Source ${source.id} must have an HTTP or HTTPS URL without credentials.`);
  }
  for (const task of catalog.tasks) { nonempty(task.label, 'Task label'); nonempty(task.description, 'Task description'); }
  const cohorts = new Map<string, string>();
  for (const observation of catalog.observations) {
    if (!sources.has(observation.sourceId)) throw new Error(`Observation ${observation.id} references a missing source.`);
    if (!models.has(observation.modelId)) throw new Error(`Observation ${observation.id} must reference an exact catalog model ID.`);
    for (const field of ['benchmark', 'version', 'metric', 'unit', 'harness', 'cohort'] as const) nonempty(observation[field], `Observation ${field}`);
    if (observation.effort !== null) nonempty(observation.effort, 'Observation effort');
    const protocol = JSON.stringify([observation.benchmark, observation.version, observation.metric, observation.unit, observation.direction, observation.harness]);
    const previous = cohorts.get(observation.cohort);
    if (previous !== undefined && previous !== protocol) throw new Error(`Cohort ${observation.cohort} mixes incompatible benchmark versions, metrics, units, score directions or harnesses.`);
    cohorts.set(observation.cohort, protocol);
  }
  for (const model of catalog.models) {
    nonempty(model.name, 'Model name'); nonempty(model.provider, 'Model provider');
    unique(model.strengths, 'Model strengths'); unique(model.limitations, 'Model limitations'); unique(model.evidenceIds, 'Model evidence IDs');
    if (model.harnesses) unique(model.harnesses, 'Model harnesses');
    for (const id of model.evidenceIds) {
      const observation = observations.get(id);
      if (!observation || observation.modelId !== model.id) throw new Error(`Model ${model.id} has missing or foreign benchmark evidence ${id}.`);
    }
    if (model.referencePricing) {
      nonempty(model.referencePricing.currency, 'Reference pricing currency');
      if (!sources.has(model.referencePricing.sourceId)) throw new Error(`Model ${model.id} reference pricing has a missing source.`);
    }
    const recommendations = new Set<string>();
    for (const recommendation of model.recommendations) {
      if (!tasks.has(recommendation.task)) throw new Error(`Model ${model.id} recommendation references an unknown task.`);
      unique(recommendation.sourceIds, 'Recommendation source IDs'); nonempty(recommendation.reason, 'Recommendation reason');
      for (const id of recommendation.sourceIds) if (!sources.has(id)) throw new Error(`Model ${model.id} recommendation references a missing source.`);
      const key = JSON.stringify([recommendation.task, recommendation.role]);
      if (recommendations.has(key)) throw new Error(`Model ${model.id} repeats a recommendation for the same task and role.`);
      recommendations.add(key);
      if (recommendation.basis === 'benchmark' && !model.evidenceIds.some(id => recommendation.sourceIds.includes(observations.get(id)!.sourceId))) {
        throw new Error(`Model ${model.id} benchmark recommendation requires linked observation evidence from a cited source.`);
      }
      if (recommendation.basis === 'vendor-guidance' && !recommendation.sourceIds.some(id => sources.get(id)!.type === 'vendor')) {
        throw new Error(`Model ${model.id} vendor guidance requires a vendor source.`);
      }
    }
    if (model.status === 'verified' && !model.evidenceIds.length && !model.recommendations.some(recommendation => recommendation.sourceIds.some(id => sources.get(id)!.type !== 'methodology'))) {
      throw new Error(`Verified model ${model.id} requires benchmark evidence or cited primary guidance.`);
    }
  }
  return catalog;
}

function inside(root: string, path: string): boolean {
  const suffix = relative(root, path);
  return Boolean(suffix) && !isAbsolute(suffix) && suffix !== '..' && !suffix.startsWith(`..${sep}`);
}
function physicallyInside(root: string, path: string): boolean {
  if (!inside(root, path)) return false;
  // path.relative folds case on Windows even when the containing NTFS directory
  // is case-sensitive. Check the actual ancestor, as loadConfig does, rather than
  // accepting a same-spelled sibling's local evidence as this workspace's data.
  const suffix = relative(root, path);
  const selectedRoot = resolve(path, ...suffix.split(sep).filter(Boolean).map(() => '..'));
  const expected = statSync(root, { bigint: true }), actual = statSync(selectedRoot, { bigint: true });
  return expected.dev === actual.dev && expected.ino === actual.ino;
}
function checkOverlayDepth(text: string): void {
  let depth = 0, quoted = false, escaped = false;
  for (const character of text) {
    if (quoted) {
      if (escaped) escaped = false;
      else if (character === '\\') escaped = true;
      else if (character === '"') quoted = false;
    } else if (character === '"') quoted = true;
    else if (character === '{' || character === '[') {
      if (++depth > MAX_AGENT_JSON_DEPTH) throw new Error(`Model map override exceeds the JSON nesting safety depth of ${MAX_AGENT_JSON_DEPTH}. No evidence was truncated.`);
    } else if (character === '}' || character === ']') depth--;
  }
}
function localOverlay(config: ModelMapConfig): { value?: ModelMapOverlay; path: string } {
  const workspace = realpathSync.native(resolve(config.workspaceRoot)), state = resolve(workspace, config.stateDir);
  if (!inside(workspace, state)) throw new Error('Model map overrides must stay in the workspace state directory.');
  const path = join(state, 'model-map.json');
  let stat;
  try {
    const physicalState = realpathSync.native(state);
    if (!physicallyInside(workspace, physicalState)) throw new Error('Model map state directory resolves outside the workspace.');
    stat = lstatSync(path);
  } catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return { path }; throw error; }
  if (!stat.isFile() || stat.isSymbolicLink()) throw new Error('Model map overrides must be a regular local JSON file, not a symbolic link.');
  const bytes = readFileSync(path);
  if (!isUtf8(bytes)) throw new Error('The local model map override must contain valid UTF-8. No evidence was replaced or truncated.');
  const text = bytes.toString('utf8').replace(/^\uFEFF/, '');
  checkOverlayDepth(text);
  let parsed: unknown;
  try { parsed = JSON.parse(text); } catch { throw new Error('The local model map override must contain valid JSON.'); }
  assertUniqueObjectKeys(text, 'Model map override');
  return { value: parseCatalog<ModelMapOverlay>(modelMapOverlayDefinition, parsed, 'model map override'), path };
}
function merge<T extends { id: string }>(base: T[], replacements: T[] | undefined, label: string): T[] {
  if (!replacements) return base;
  byId(replacements, `${label} override`);
  const values = new Map(base.map(value => [value.id, value]));
  for (const value of replacements) values.set(value.id, value);
  return [...values.values()];
}
function matchingCatalog(catalog: ModelMapCatalog, models: ModelProfile[]): ModelMapCatalog {
  const selected = new Set(models.map(model => model.id));
  const observations = catalog.observations.filter(observation => selected.has(observation.modelId));
  const cited = new Set(observations.map(observation => observation.sourceId));
  for (const model of models) {
    for (const recommendation of model.recommendations) for (const sourceId of recommendation.sourceIds) cited.add(sourceId);
    if (model.referencePricing) cited.add(model.referencePricing.sourceId);
  }
  return { ...catalog, models, observations,
    sources: catalog.sources.filter(source => cited.has(source.id) || source.type === 'methodology') };
}
function rankings(observations: ModelMapObservation[]): ModelMapRanking[] {
  const groups = new Map<string, ModelMapObservation[]>();
  for (const observation of observations) {
    const group = groups.get(observation.cohort) ?? []; group.push(observation); groups.set(observation.cohort, group);
  }
  return [...groups].map(([cohort, group]) => {
    const first = group[0]!;
    group.sort((a, b) => (first.direction === 'higher' ? b.score - a.score : a.score - b.score) || a.id.localeCompare(b.id, 'en'));
    let rank = 0;
    return { cohort, benchmark: first.benchmark, version: first.version, metric: first.metric, unit: first.unit, direction: first.direction,
      entries: group.map((observation, index) => {
        if (index === 0 || observation.score !== group[index - 1]!.score) rank = index + 1;
        return { rank, observationId: observation.id, modelId: observation.modelId, score: observation.score, harness: observation.harness, effort: observation.effort };
      }) };
  });
}

/** Read-only planning reference. Does not choose a running model, update weights, or inspect account availability. */
export function modelMapView(config: ModelMapConfig, value: ModelMapQuery = {}, baseCatalog: ModelMapCatalog = MODEL_MAP_CATALOG): ModelMapView {
  const query = parseSchema<ModelMapQuery>(modelMapQueryDefinition, value, 'model map query');
  for (const [key, filter] of Object.entries(query)) if (typeof filter === 'string') nonempty(filter, `Model map ${key} filter`);
  const base = validateModelMap(baseCatalog), overlay = localOverlay(config);
  const catalog = overlay.value ? validateModelMap({ ...base,
    sources: merge(base.sources, overlay.value.sources, 'Source'), observations: merge(base.observations, overlay.value.observations, 'Observation'),
    models: merge(base.models, overlay.value.models, 'Model'), tasks: merge(base.tasks, overlay.value.tasks, 'Task') }) : base;
  const lookup = modelLookup(catalog.models), selectedModel = query.model === undefined ? undefined : lookup.get(query.model);
  const warnings = [
    'Priorities are task-and-role planning ordinals, not intelligence scores. Tied priorities are not broken by benchmark averages.',
    'Benchmark ranks are computed within each catalog cohort, which may contain only a subset of the owner\'s results. They are not owner-global ranks or claims of statistical significance. Recorded harness and effort remain attached; different cohorts are never combined.',
    'Native defaults and account access are unresolved here. Reference prices are dated evidence, not account billing or a spending estimate.',
  ];
  if (query.model !== undefined && !selectedModel) warnings.push(`Unknown exact model ID or alias ${JSON.stringify(query.model)}. No substitute was selected.`);
  if (query.task !== undefined && !catalog.tasks.some(task => task.id === query.task)) warnings.push(`Unknown exact task ${JSON.stringify(query.task)}. No task category was guessed.`);
  if (selectedModel?.status === 'unverified') warnings.push(`The exact entry ${selectedModel.id} is unverified and is excluded from the recommendation hierarchy.`);
  const harness = query.harness;
  const models = catalog.models.filter(model => (query.model === undefined || model.id === selectedModel?.id)
    && (harness === undefined || model.harnesses?.includes(harness))
    && (query.task === undefined || model.recommendations.some(recommendation => recommendation.task === query.task))
    && (query.role === undefined || model.recommendations.some(recommendation => recommendation.role === query.role && (query.task === undefined || recommendation.task === query.task))));
  if (harness !== undefined && !catalog.models.some(model => model.harnesses?.includes(harness))) warnings.push(`No catalog profile declares harness ${JSON.stringify(harness)}. Harness names are not model identities.`);
  if (harness !== undefined) warnings.push('The harness filter uses declared compatibility only. Benchmark rows retain their published harness and effort; compatibility does not establish local replication or account availability.');
  const allSuggestions: ModelMapSuggestion[] = catalog.models.filter(model => model.status !== 'unverified').flatMap(model => model.recommendations.map(recommendation => ({
    ...recommendation, modelId: model.id, modelName: model.name, status: model.status, tied: false,
  })));
  const ties = new Map<string, number>();
  for (const suggestion of allSuggestions) {
    const key = JSON.stringify([suggestion.task, suggestion.role, suggestion.priority]); ties.set(key, (ties.get(key) ?? 0) + 1);
  }
  for (const suggestion of allSuggestions) suggestion.tied = ties.get(JSON.stringify([suggestion.task, suggestion.role, suggestion.priority]))! > 1;
  const selected = new Set(models.map(model => model.id)), taskOrder = new Map(catalog.tasks.map((task, index) => [task.id, index]));
  const roleOrder = new Map(['lead', 'worker', 'reviewer', 'advisor', 'economy'].map((role, index) => [role, index]));
  const hierarchy = allSuggestions.filter(suggestion => selected.has(suggestion.modelId) && (query.task === undefined || suggestion.task === query.task)
    && (query.role === undefined || suggestion.role === query.role)).sort((a, b) => taskOrder.get(a.task)! - taskOrder.get(b.task)!
      || roleOrder.get(a.role)! - roleOrder.get(b.role)! || a.priority - b.priority || a.modelId.localeCompare(b.modelId, 'en'));
  const bindings = Object.entries(config.agents).map(([alias, agent]) => {
    const model = agent.model ?? null, matched = model === null ? undefined : lookup.get(model);
    return { alias, profile: agent.profile, model, enabled: agent.enabled,
      match: model === null ? 'unresolved' as const : matched ? 'exact' as const : 'unknown' as const,
      ...(matched ? { catalogModelId: matched.id } : {}) };
  });
  const benchmarkRankings = rankings(catalog.observations).map(group => ({ ...group,
    entries: group.entries.filter(entry => selected.has(entry.modelId)) })).filter(group => group.entries.length);
  return { catalog: query.includeFullCatalog === true ? catalog : matchingCatalog(catalog, models),
    catalogScope: query.includeFullCatalog === true ? 'full' : 'matching-models', totalModels: catalog.models.length,
    query, models, hierarchy, rankings: benchmarkRankings, bindings, warnings, overrideApplied: overlay.value !== undefined, overridePath: overlay.path };
}
