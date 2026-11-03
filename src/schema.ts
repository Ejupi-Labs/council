/** The bounded JSON Schema subset used by Council's public input contracts. */
export interface JsonSchema {
  type?: 'object' | 'array' | 'string' | 'integer' | 'number' | 'boolean';
  properties?: Record<string, JsonSchema>;
  required?: string[];
  additionalProperties?: boolean | JsonSchema;
  propertyNames?: { pattern: string };
  items?: JsonSchema;
  enum?: readonly (string | number | boolean)[];
  const?: string | number | boolean;
  default?: unknown;
  minLength?: number; maxLength?: number; pattern?: string;
  minItems?: number; maxItems?: number; minimum?: number; maximum?: number;
  description?: string;
}

export class ValidationError extends Error {
  constructor(message: string) { super(message); this.name = 'ValidationError'; }
}

export function objectSchema(properties: Record<string, JsonSchema>, required: string[] = []): JsonSchema {
  return { type: 'object', properties, required, additionalProperties: false };
}

export function parseSchema<T = Record<string, unknown>>(schema: JsonSchema, value: unknown, label = 'input'): T {
  // Bound every input branch, including permissive schemas, before allocating parsed copies.
  const pending = [{ value, depth: 0 }];
  let inspected = 0, textBytes = 0;
  while (pending.length) {
    const current = pending.pop()!;
    if (++inspected > 8192 || current.depth > 64) throw new ValidationError('Input exceeds the structural validation limit.');
    const entry = current.value;
    if (typeof entry === 'string') textBytes += Buffer.byteLength(entry);
    if (typeof entry === 'bigint' || typeof entry === 'symbol' || typeof entry === 'function' || (typeof entry === 'number' && !Number.isFinite(entry))) throw new ValidationError('Input must contain JSON-compatible values.');
    if (entry && typeof entry === 'object') {
      if (!Array.isArray(entry) && ![Object.prototype, null].includes(Object.getPrototypeOf(entry))) throw new ValidationError('Input must contain plain JSON objects.');
      for (const key in entry) if (Object.hasOwn(entry, key)) {
        if (inspected + pending.length >= 8192) throw new ValidationError('Input exceeds the structural validation limit.');
        textBytes += Buffer.byteLength(key);
        pending.push({ value: (entry as Record<string, unknown>)[key], depth: current.depth + 1 });
      }
    }
    if (textBytes > 1_048_576) throw new ValidationError('Input text exceeds 1 MiB.');
  }
  let nodes = 0;
  function parse(rule: JsonSchema, input: unknown, path: string, depth: number): unknown {
    if (++nodes > 8192 || depth > 64) throw new ValidationError('Input exceeds the structural validation limit.');
    if (input === undefined && rule.default !== undefined) input = structuredClone(rule.default);
    const fail = (detail: string): never => { throw new ValidationError(`${path}: ${detail}`); };
    if (rule.const !== undefined && input !== rule.const) fail(`must equal ${JSON.stringify(rule.const)}.`);
    if (rule.enum && !rule.enum.includes(input as string)) fail(`must be one of ${rule.enum.join(', ')}.`);
    if (rule.type === 'string') {
      if (typeof input !== 'string') fail('must be a string.');
      const text = input as string;
      if (rule.minLength !== undefined && text.length < rule.minLength) fail(`must contain at least ${rule.minLength} characters.`);
      if (rule.maxLength !== undefined && text.length > rule.maxLength) fail(`must contain at most ${rule.maxLength} characters.`);
      if (rule.pattern && !new RegExp(rule.pattern).test(text)) fail('has an invalid format.');
      return text;
    }
    if (rule.type === 'integer' || rule.type === 'number') {
      if (typeof input !== 'number' || !Number.isFinite(input) || (rule.type === 'integer' && !Number.isInteger(input))) fail(`must be a finite ${rule.type}.`);
      const number = input as number;
      if (rule.minimum !== undefined && number < rule.minimum) fail(`must be at least ${rule.minimum}.`);
      if (rule.maximum !== undefined && number > rule.maximum) fail(`must be at most ${rule.maximum}.`);
      return number;
    }
    if (rule.type === 'boolean') {
      if (typeof input !== 'boolean') fail('must be a boolean.');
      return input;
    }
    if (rule.type === 'array') {
      if (!Array.isArray(input)) fail('must be an array.');
      const array = input as unknown[];
      if (rule.minItems !== undefined && array.length < rule.minItems) fail(`must contain at least ${rule.minItems} entries.`);
      if (rule.maxItems !== undefined && array.length > rule.maxItems) fail(`must contain at most ${rule.maxItems} entries.`);
      return array.map((entry, index) => parse(rule.items ?? {}, entry, `${path}[${index}]`, depth + 1));
    }
    if (rule.type === 'object') {
      if (!input || typeof input !== 'object' || Array.isArray(input)) fail('must be an object.');
      const source = input as Record<string, unknown>;
      const result: Record<string, unknown> = {};
      for (const key of rule.required ?? []) if (!Object.hasOwn(source, key) || source[key] === undefined) fail(`missing required property ${key}.`);
      const keys = new Set([...Object.keys(rule.properties ?? {}), ...Object.keys(source)]);
      for (const key of keys) {
        const known = rule.properties && Object.hasOwn(rule.properties, key) ? rule.properties[key] : undefined;
        if (rule.propertyNames && !new RegExp(rule.propertyNames.pattern).test(key)) fail('contains an invalid property name.');
        if (!known && rule.additionalProperties === false) fail(`unknown property ${key}.`);
        const childRule = known ?? (typeof rule.additionalProperties === 'object' ? rule.additionalProperties : {});
        const entry = Object.hasOwn(source, key) ? source[key] : undefined;
        if (entry === undefined && childRule.default === undefined) continue;
        Object.defineProperty(result, key, { value: parse(childRule, entry, `${path}.${key}`, depth + 1), enumerable: true, writable: true, configurable: true });
      }
      return result;
    }
    return structuredClone(input);
  }
  return parse(schema, value, label, 0) as T;
}
