export class ValidationError extends Error {
    constructor(message) { super(message); this.name = 'ValidationError'; }
}
export function objectSchema(properties, required = []) {
    return { type: 'object', properties, required, additionalProperties: false };
}
export function parseSchema(schema, value, label = 'input') {
    // Bound every input branch, including permissive schemas, before allocating parsed copies.
    const pending = [{ value, depth: 0 }];
    let inspected = 0;
    while (pending.length) {
        const current = pending.pop();
        if (++inspected > 8192 || current.depth > 64)
            throw new ValidationError('Input exceeds the structural validation limit.');
        const entry = current.value;
        if (typeof entry === 'bigint' || typeof entry === 'symbol' || typeof entry === 'function' || (typeof entry === 'number' && !Number.isFinite(entry)))
            throw new ValidationError('Input must contain JSON-compatible values.');
        if (entry && typeof entry === 'object') {
            if (!Array.isArray(entry) && ![Object.prototype, null].includes(Object.getPrototypeOf(entry)))
                throw new ValidationError('Input must contain plain JSON objects.');
            for (const key in entry)
                if (Object.hasOwn(entry, key)) {
                    if (inspected + pending.length >= 8192)
                        throw new ValidationError('Input exceeds the structural validation limit.');
                    pending.push({ value: entry[key], depth: current.depth + 1 });
                }
        }
    }
    let nodes = 0;
    function parse(rule, input, path, depth) {
        if (++nodes > 8192 || depth > 64)
            throw new ValidationError('Input exceeds the structural validation limit.');
        if (input === undefined && rule.default !== undefined)
            input = structuredClone(rule.default);
        const fail = (detail) => { throw new ValidationError(`${path}: ${detail}`); };
        if (Array.isArray(rule.type)) {
            for (const type of rule.type) {
                try {
                    return parse({ ...rule, type }, input, path, depth);
                }
                catch (error) {
                    if (!(error instanceof ValidationError))
                        throw error;
                }
            }
            return fail(`must match one of: ${rule.type.join(', ')}.`);
        }
        if (rule.type === 'null') {
            if (input !== null)
                fail('must be null.');
            return null;
        }
        if (rule.const !== undefined && input !== rule.const)
            fail(`must equal ${JSON.stringify(rule.const)}.`);
        if (rule.enum && !rule.enum.includes(input))
            fail(`must be one of ${rule.enum.join(', ')}.`);
        if (rule.type === 'string') {
            if (typeof input !== 'string')
                fail('must be a string.');
            const text = input;
            if (rule.minLength !== undefined && text.length < rule.minLength)
                fail(`must contain at least ${rule.minLength} characters.`);
            if (rule.maxLength !== undefined && text.length > rule.maxLength)
                fail(`must contain at most ${rule.maxLength} characters.`);
            if (rule.pattern && !new RegExp(rule.pattern).test(text))
                fail('has an invalid format.');
            return text;
        }
        if (rule.type === 'integer' || rule.type === 'number') {
            if (typeof input !== 'number' || !Number.isFinite(input) || (rule.type === 'integer' && !Number.isInteger(input)))
                fail(`must be a finite ${rule.type}.`);
            const number = input;
            if (rule.minimum !== undefined && number < rule.minimum)
                fail(`must be at least ${rule.minimum}.`);
            if (rule.maximum !== undefined && number > rule.maximum)
                fail(`must be at most ${rule.maximum}.`);
            if (rule.exclusiveMinimum !== undefined && number <= rule.exclusiveMinimum)
                fail(`must be greater than ${rule.exclusiveMinimum}.`);
            return number;
        }
        if (rule.type === 'boolean') {
            if (typeof input !== 'boolean')
                fail('must be a boolean.');
            return input;
        }
        if (rule.type === 'array') {
            if (!Array.isArray(input))
                fail('must be an array.');
            const array = input;
            if (rule.minItems !== undefined && array.length < rule.minItems)
                fail(`must contain at least ${rule.minItems} entries.`);
            if (rule.maxItems !== undefined && array.length > rule.maxItems)
                fail(`must contain at most ${rule.maxItems} entries.`);
            return array.map((entry, index) => parse(rule.items ?? {}, entry, `${path}[${index}]`, depth + 1));
        }
        if (rule.type === 'object') {
            if (!input || typeof input !== 'object' || Array.isArray(input))
                fail('must be an object.');
            const source = input;
            const result = {};
            for (const key of rule.required ?? [])
                if (!Object.hasOwn(source, key) || source[key] === undefined)
                    fail(`missing required property ${key}.`);
            const keys = new Set([...Object.keys(rule.properties ?? {}), ...Object.keys(source)]);
            for (const key of keys) {
                const known = rule.properties && Object.hasOwn(rule.properties, key) ? rule.properties[key] : undefined;
                if (rule.propertyNames && !new RegExp(rule.propertyNames.pattern).test(key))
                    fail('contains an invalid property name.');
                if (!known && rule.additionalProperties === false)
                    fail(`unknown property ${key}.`);
                const childRule = known ?? (typeof rule.additionalProperties === 'object' ? rule.additionalProperties : {});
                const entry = Object.hasOwn(source, key) ? source[key] : undefined;
                if (entry === undefined && childRule.default === undefined)
                    continue;
                Object.defineProperty(result, key, { value: parse(childRule, entry, `${path}.${key}`, depth + 1), enumerable: true, writable: true, configurable: true });
            }
            return result;
        }
        return structuredClone(input);
    }
    return parse(schema, value, label, 0);
}
//# sourceMappingURL=schema.js.map