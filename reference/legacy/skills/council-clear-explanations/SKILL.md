---
name: council-clear-explanations
description: Explain a Council decision, technical concept, status or result in language its human audience can understand. Use when jargon or missing connections obscure the answer.
---

# Clear explanations

Start with the answer or usable result. Put any blocking limitation beside the claim it qualifies. Keep the user's language and requested depth. This skill also works for direct host replies; it does not require a Council run or an extra rewriting call.

For a concept, introduce its central idea before implementation details. Use an early concrete example and reuse it. For example: a person sends a message; the server keeps it; the recipient's app displays it. Explain each component's job through that same example before discussing where it runs.

For a decision, explain the proposed action, its reason, the important tradeoff and unresolved uncertainty. Add detail when it changes understanding. Use tables for useful comparisons, rather than as a fixed output template.

<!-- council-agent-instructions -->
For human-facing explanations, keep the user's language and requested depth. Lead with the answer or usable result, with any blocking limitation beside it. Use familiar words, direct verbs and one main idea per sentence. Explain necessary terms at first use and keep their meaning consistent. Develop one topic per paragraph. For concepts, introduce an early concrete example and reuse it to connect actors, actions and results. Explain responsibilities before implementation or deployment details.

Preserve exact commands, identifiers, UI labels, quoted errors, numbers and necessary conditions. Distinguish implemented, tested, running, published and merged; name the component and environment being described. Put prerequisites before instructions. Claim only checks actually performed. Keep material caveats, dissent and failed or cancelled work visible.

When the task requires JSON, code or another exact format, preserve that contract. Apply plain language only inside permitted prose fields; never add an explanatory wrapper to a room ballot, advisor request or private-chat envelope. Use no rigid word count, evidence clipping or extra model call merely to shorten text.
<!-- /council-agent-instructions -->

These original instructions primarily adapt the user's supplied clarity rules. Related references are Matt Pocock's [wait-what](https://github.com/mattpocock/skills/blob/main/skills/productivity/wait-what/SKILL.md), [Anthropic internal communications](https://github.com/anthropics/skills/blob/main/skills/internal-comms/examples/general-comms.md) and [Humanizer](https://github.com/blader/humanizer/blob/main/SKILL.md). They are selective clarity principles, not a claim of full ASD-STE100 compliance. See the [research record](../../docs/research-clear-writing-2026-11-05.md).
