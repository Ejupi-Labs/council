# Clear explanations: skill research

Retrieved **5 November 2026**. This review examines actual instructions, repository adoption and reuse terms. Stars measure repository interest. Skills-directory installs measure reported installations, not active users, teaching quality or verified workflow outcomes. No external skills were installed or executed. Council's original adaptations and their usage are described in [Collaboration playbooks](collaboration-playbooks.md).

## Strongest matches

GitHub counts below were read from the public API at approximately **09:48 UTC**. Installation counts are the rounded values displayed by skills.sh in the same research session. These are separate measurements and can change independently.

| Repository and inspected skill | GitHub stars | Reported skill installs | Reuse evidence |
| --- | ---: | ---: | --- |
| [mattpocock/skills](https://github.com/mattpocock/skills): [wait-what](https://github.com/mattpocock/skills/blob/main/skills/productivity/wait-what/SKILL.md), [teach](https://github.com/mattpocock/skills/blob/main/skills/productivity/teach/SKILL.md) | 313,959 | [wait-what: 496.4K](https://www.skills.sh/mattpocock/skills/wait-what); [teach: 914.5K](https://www.skills.sh/mattpocock/skills/teach) | [Repository MIT license](https://github.com/mattpocock/skills/blob/main/LICENSE) |
| [anthropics/skills](https://github.com/anthropics/skills): [internal-comms](https://github.com/anthropics/skills/blob/main/skills/internal-comms/SKILL.md), [doc-coauthoring](https://github.com/anthropics/skills/blob/main/skills/doc-coauthoring/SKILL.md) | 187,555 | [internal-comms: 83.6K](https://www.skills.sh/anthropics/skills/internal-comms); [doc-coauthoring: 98.0K](https://www.skills.sh/anthropics/skills/doc-coauthoring) | [Internal-comms Apache-2.0 file](https://github.com/anthropics/skills/blob/main/skills/internal-comms/LICENSE.txt); a specific coauthoring license was not verified |
| [blader/humanizer](https://github.com/blader/humanizer): [humanizer](https://github.com/blader/humanizer/blob/main/SKILL.md) | 65,020 | [12.0K](https://www.skills.sh/blader/humanizer/humanizer) | [MIT](https://github.com/blader/humanizer/blob/main/LICENSE) |
| [multica-ai/andrej-karpathy-skills](https://github.com/multica-ai/andrej-karpathy-skills): [CLAUDE.md](https://github.com/multica-ai/andrej-karpathy-skills/blob/main/CLAUDE.md), [karpathy-guidelines](https://github.com/multica-ai/andrej-karpathy-skills/blob/main/skills/karpathy-guidelines/SKILL.md) | 229,657 | [44.7K](https://www.skills.sh/multica-ai/andrej-karpathy-skills/karpathy-guidelines) | Skill frontmatter declares MIT; the repository API reports no detected license |

Count sources: public GitHub repository metadata for [Matt Pocock](https://api.github.com/repos/mattpocock/skills), [Anthropic](https://api.github.com/repos/anthropics/skills), [Humanizer](https://api.github.com/repos/blader/humanizer), and [Karpathy-inspired guidelines](https://api.github.com/repos/multica-ai/andrej-karpathy-skills). The original `forrestchang/andrej-karpathy-skills` URL redirects to `multica-ai`. This is community-authored guidance inspired by Karpathy, not evidence of his endorsement.

## What is useful for Council

**wait-what is the closest match to the user's example.** Its short, explicitly invoked instruction asks the agent to reconstruct a confusing answer with missing context, simplified technical language and established project terms. This is a repair action for an explanation that failed. Council can provide the same capability while preserving the user's language, rather than forcing English. A glossary helps only when its terms are explained to the reader. [Inspected instruction](https://github.com/mattpocock/skills/blob/main/skills/productivity/wait-what/SKILL.md).

The file invokes no separate STE or glossary skill. It needs a compatible host and manual invocation; its frontmatter disables automatic invocation. The author's [usage page](https://github.com/mattpocock/skills/blob/main/docs/productivity/wait-what.md) says project `GLOSSARY.md` / `GLOSSARY-MAP.md` files are optional. Without them, it loses project terminology support. It reuses existing global writing instructions and does not bundle the STE standard or establish full compliance.

**teach is useful for a requested learning programme.** It organizes lessons around the learner's purpose, prior learning, trusted resources and practice. Its persistent teaching workspace and interactive lessons are much heavier than an ordinary status update. Council should borrow the attention to the learner's starting point, not create a course or learning records for every explanation. [Inspected instruction](https://github.com/mattpocock/skills/blob/main/skills/productivity/teach/SKILL.md).

**internal-comms provides a good reporting model.** Its general communication reference starts with audience, purpose and tone, then emphasizes direct language and the main information first. Council usually knows the audience: the human managing the agents. Use that context directly; ask only when missing information affects the result. Different report types deserve different detail. [General communication reference](https://github.com/anthropics/skills/blob/main/skills/internal-comms/examples/general-comms.md).

**doc-coauthoring contributes reader testing.** It separates context collection, drafting and testing with a fresh reader. The useful check is whether someone without the authoring conversation understands the document. That can justify an independent pass on an important architecture or decision explanation. Running this process for every short reply would add cost and delay. Its inspected folder had no verified `LICENSE.txt`; use this workflow as conceptual inspiration, not copied prompt text. [Inspected instruction](https://github.com/anthropics/skills/blob/main/skills/doc-coauthoring/SKILL.md), [repository licensing explanation](https://github.com/anthropics/skills#about-this-repository).

**Humanizer is an editing aid, not proof of comprehension.** Version 3.1.0 preserves supported claims and exact technical material while removing inflated, repetitive or generic presentation. It distinguishes factual prose from personal voice. Embedded use returns the final text; the default pasted-text workflow also exposes a draft and critique. Council should use a short editing pass, retain uncertainty and avoid inventing personal experience or missing facts. Detector evasion is not the goal. [Inspected skill](https://github.com/blader/humanizer/blob/main/SKILL.md).

**The Karpathy-inspired rules help explain implementation decisions.** They surface assumptions, simpler alternatives and verifiable outcomes. They are primarily coding guidance, not a teaching method. Council should retain the clear tradeoffs without adopting a blanket requirement to stop for every uncertainty. The license discrepancy also favors original wording rather than copying the standalone `CLAUDE.md`. [Inspected instructions](https://github.com/multica-ai/andrej-karpathy-skills/blob/main/CLAUDE.md).

## Proposed Council guidance

Use the user's supplied clarity rules as the foundation. These are proposed adaptations, not claims that the external skills already implement Council integration:

- Keep the chosen language, audience and requested detail. Preserve commands, identifiers, UI labels, quoted errors and factual qualifications.
- Start with the answer or usable result. Put a blocking limitation beside the claim it limits.
- For a new concept, establish its purpose before naming its implementation. Introduce one concrete input → actor → action → result example and reuse it.
- Name each actor and explain its responsibility. Separate that responsibility from the program or machine that performs it.
- Explain essential terminology when it first appears. Use the same term consistently; avoid replacing technical words with unexplained metaphors.
- For a decision, state the recommendation, why it fits, the strongest unresolved objection and what the human must decide. Do not reproduce the entire debate by default.
- For status, identify the component and environment. Distinguish proposed, implemented, tested, running, merged and published work. Include relevant prerequisites in final instructions.
- If the user did not understand, rebuild the explanation around a better example. Do not simply shorten the same jargon or blame the reader.

Keep these rules available to the host and to workers that produce human-facing explanations. Preserve complete supporting evidence separately. Do not impose hard sentence-length checks, fixed paragraph counts or automatic extra model calls.

The official [ASD-STE100 Issue 9 PDF](https://www.asd-ste100.org/assets/files/ASD-STE100_ISSUE9.pdf) returned HTTP 403 during this research. Its full contents were not independently checked. The user supplied selective adaptations; Council should not claim full STE compliance, especially for Italian.
