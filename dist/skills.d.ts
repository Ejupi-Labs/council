export declare const BUILT_IN_SKILL_IDS: readonly ['dont-kill-an-ant-with-a-bazooka', 'council-implementation-review', 'council-independent-perspectives', 'council-advisor', 'council-evidence-researcher', 'council-proposal-advocate', 'council-constructive-skeptic', 'council-constraint-reviewer', 'council-user-advocate', 'council-clear-explanations', 'council-lazy-genius'];
export type CouncilSkillId = typeof BUILT_IN_SKILL_IDS[number];
export declare function collaborationSkills(): ({
    id: "dont-kill-an-ant-with-a-bazooka";
    name: "Don't kill an ant with a bazooka";
    description: 'Right-size models, effort and collaboration to the task while preserving verification and explicit model choices.';
} | {
    id: "council-implementation-review";
    name: 'Implementation and review';
    description: 'Separate implementation, independent review and revision for work that benefits from a second perspective.';
} | {
    id: "council-independent-perspectives";
    name: 'Independent perspectives';
    description: 'Keep parallel analyses independent and make disagreements and evidence explicit.';
} | {
    id: "council-advisor";
    name: 'Focused advisor';
    description: 'Use a focused read-only consultation when a specific uncertainty warrants another model.';
} | {
    id: "council-evidence-researcher";
    name: 'Evidence researcher';
    description: 'Investigate relevant sources, distinguish facts from assumptions and identify missing evidence.';
} | {
    id: "council-proposal-advocate";
    name: 'Proposal advocate';
    description: 'Develop a defensible proposal, explain its tradeoffs and revise it when evidence changes.';
} | {
    id: "council-constructive-skeptic";
    name: 'Constructive skeptic';
    description: 'Test consequential claims and assumptions without manufacturing opposition or blocking supported conclusions.';
} | {
    id: "council-constraint-reviewer";
    name: 'Constraint reviewer';
    description: 'Check a proposal against the task’s explicit requirements, permissions and practical constraints.';
} | {
    id: "council-user-advocate";
    name: 'User advocate';
    description: 'Examine a proposal from the intended user’s needs, comprehension and actual workflow.';
} | {
    id: "council-clear-explanations";
    name: 'Clear explanations';
    description: 'Explain outcomes and decisions in the reader’s language while preserving exact technical evidence and required reply formats.';
} | {
    id: "council-lazy-genius";
    name: 'Lazy Genius';
    description: 'Review unnecessary complexity and propose the simplest complete solution while preserving requirements, safeguards and verification.';
})[];
export declare function validateSkills(value: unknown): CouncilSkillId[];
/** Only bundled worker guidance is injected; host orchestration instructions stay outside worker prompts. */
export declare function skillInstructions(value: unknown): string;
