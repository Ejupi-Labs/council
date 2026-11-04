export class RoutingError extends Error {
    code;
    alternatives;
    escalation;
    constructor(code, message, alternatives = [], escalation) {
        super(message);
        this.code = code;
        this.alternatives = alternatives;
        this.escalation = escalation;
        this.name = 'RoutingError';
    }
}
//# sourceMappingURL=routing-types.js.map