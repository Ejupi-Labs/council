export declare const BUILT_IN_SKILL_IDS: readonly ['dont-kill-an-ant-with-a-bazooka', 'council-implementation-review', 'council-independent-perspectives', 'council-advisor'];
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
})[];
export declare function validateSkills(value: unknown): CouncilSkillId[];
/** Only bundled worker guidance is injected; host orchestration instructions stay outside worker prompts. */
export declare function skillInstructions(value: unknown): string;
