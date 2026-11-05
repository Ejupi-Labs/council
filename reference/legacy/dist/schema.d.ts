/** The bounded JSON Schema subset used by Council's public input contracts. */
export type JsonSchemaType = 'object' | 'array' | 'string' | 'integer' | 'number' | 'boolean' | 'null';
export interface JsonSchema {
    type?: JsonSchemaType | JsonSchemaType[];
    properties?: Record<string, JsonSchema>;
    required?: string[];
    additionalProperties?: boolean | JsonSchema;
    propertyNames?: {
        pattern: string;
    };
    items?: JsonSchema;
    enum?: readonly (string | number | boolean)[];
    const?: string | number | boolean;
    default?: unknown;
    minLength?: number;
    maxLength?: number;
    pattern?: string;
    minItems?: number;
    maxItems?: number;
    minimum?: number;
    maximum?: number;
    exclusiveMinimum?: number;
    description?: string;
}
export declare class ValidationError extends Error {
    constructor(message: string);
}
export declare function objectSchema(properties: Record<string, JsonSchema>, required?: string[]): JsonSchema;
export declare function parseSchema<T = Record<string, unknown>>(schema: JsonSchema, value: unknown, label?: string): T;
