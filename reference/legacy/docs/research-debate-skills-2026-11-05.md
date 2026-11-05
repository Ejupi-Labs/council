# Debate and evidence skills: research snapshot

Retrieved 5 November 2026. This review reads actual public instructions, not just repository descriptions. Stars describe repository popularity. They do not prove that an individual skill works well. Directory install counts are neither unique-user counts nor verified active usage. No external skill was installed, no downloaded code was run, and no local vendor agent was invoked for the research.

## Adoption and licensing

The exact GitHub counts below came from each repository's public API. Linked commits identify the source snapshot. The former Personal AI Infrastructure and Everything Claude Code names now redirect to LifeOS and ECC.

| Repository | Stars / forks | Repository license | Inspected commit |
| --- | ---: | --- | --- |
| [sickn33/agentic-awesome-skills](https://api.github.com/repos/sickn33/agentic-awesome-skills) | 49,184 / 7,143 | MIT | [be0a15a](https://github.com/sickn33/agentic-awesome-skills/commit/be0a15aa71de8027efb8eb69aa6b58c392c55fe4) |
| [danielmiessler/LifeOS](https://api.github.com/repos/danielmiessler/LifeOS) | 19,708 / 2,513 | MIT | [b7efec9](https://github.com/danielmiessler/LifeOS/commit/b7efec97d8c3885feee285bee468c35602fb0606) |
| [affaan-m/ECC](https://api.github.com/repos/affaan-m/ECC) | 297,481 / 43,984 | MIT | [b38be6a](https://github.com/affaan-m/ECC/commit/b38be6a7f6350964d59328b52f0624e1d1d552d1) |
| [obra/superpowers](https://api.github.com/repos/obra/superpowers) | 312,662 / 27,813 | MIT | [22f0b46](https://github.com/obra/superpowers/commit/22f0b463e7a5e579904dd6211a520321035526f7) |
| [wshobson/agents](https://api.github.com/repos/wshobson/agents) | 42,224 / 4,460 | MIT | [7e2fb55](https://github.com/wshobson/agents/commit/7e2fb559cf4b824b27f1fec49fe88f7511066d26) |
| [msitarzewski/agency-agents](https://api.github.com/repos/msitarzewski/agency-agents) | 166,642 / 26,992 | MIT | [b37b5bb](https://github.com/msitarzewski/agency-agents/commit/b37b5bb33b78f2c1d6567d98e25ca818e3259219) |

The served skills.sh pages displayed **1.4K installs** for [multi-agent-brainstorming](https://www.skills.sh/sickn33/agentic-awesome-skills/multi-agent-brainstorming), **418.0K** for [Superpowers brainstorming](https://www.skills.sh/obra/superpowers/brainstorming), and **227.4K** for [dispatching-parallel-agents](https://www.skills.sh/obra/superpowers/dispatching-parallel-agents). [ecc-council](https://www.skills.sh/affaan-m/ecc/ecc-council) displayed only **seven**, with older cached metadata. This does not establish broad adoption of ECC's debate skill. Individual install counts for LifeOS and the selected Agency/wshobson files were not verified.

## Most useful patterns

### Structured design review: the closest match

[multi-agent-brainstorming/SKILL.md](https://github.com/sickn33/agentic-awesome-skills/blob/be0a15aa71de8027efb8eb69aa6b58c392c55fe4/skills/multi-agent-brainstorming/SKILL.md) separates designer, challenger, constraint reviewer, user advocate and arbiter. Reviews happen in sequence. Objections refer to decisions, and the final record explains their disposition. It explicitly distinguishes independently invoked reviewers from one model playing several roles.

**Council adaptation:** separate proposal, criticism and final decision; retain unresolved objections. Select roles that matter to the task. Do not import mandatory role counts, fixed locks, or a process that continues until everyone approves. A failed or missing reviewer must remain visible.

### LifeOS Council and RedTeam: useful disagreement

[Council/SKILL.md](https://github.com/danielmiessler/LifeOS/blob/b7efec97d8c3885feee285bee468c35602fb0606/LifeOS/install/skills/Council/SKILL.md) creates topic-specific perspectives and preserves a visible discussion. [CouncilMembers.md](https://github.com/danielmiessler/LifeOS/blob/b7efec97d8c3885feee285bee468c35602fb0606/LifeOS/install/skills/Council/CouncilMembers.md) defines each member's expertise, position and challenge. Later contributions must address actual earlier arguments. Research supplies context before deliberation; a separate adversarial review can test the conclusion.

[RedTeam ParallelAnalysis](https://github.com/danielmiessler/LifeOS/blob/b7efec97d8c3885feee285bee468c35602fb0606/LifeOS/install/skills/RedTeam/Intents/ParallelAnalysis.md) attacks the strongest fair interpretation, separates supported claims from assumptions, and rejects objections invented merely to disagree.

**Council adaptation:** give each participant a meaningful question and state what evidence could change its position. Preserve dissent. Do not copy fixed rounds, word limits, provider/model preferences or large default fan-outs. Cross-provider agreement is useful evidence, not proof.

### ECC Council: reduce early influence

[skills/council/SKILL.md](https://github.com/affaan-m/ECC/blob/b38be6a7f6350964d59328b52f0624e1d1d552d1/skills/council/SKILL.md) obtains fresh skeptical, practical and risk-focused positions before synthesis. It keeps raw positions and the strongest dissent visible. Straight facts and obvious execution tasks bypass the ceremony.

[council-multi-model](https://github.com/affaan-m/ECC/blob/b38be6a7f6350964d59328b52f0624e1d1d552d1/skills/council-multi-model/SKILL.md) labels same-provider and cross-provider reviews honestly and reports missing reviews. Its exact CLI-version pin and tool restrictions describe that adapter, not Council's existing compatibility.

**Council adaptation:** collect independent initial positions before sharing peer conclusions. Supply complete relevant evidence and user constraints; avoid unrelated conversation history. Do not import its fixed participant count, word cap, external service adapter or automatic issue updates.

### Superpowers: focused delegation and a fresh reader

[dispatching-parallel-agents](https://github.com/obra/superpowers/blob/22f0b463e7a5e579904dd6211a520321035526f7/skills/dispatching-parallel-agents/SKILL.md) assigns independent investigations with clear goals and boundaries. This is parallel research, not opposing debate.

The [brainstorming builder check](https://github.com/obra/superpowers/blob/22f0b463e7a5e579904dd6211a520321035526f7/skills/brainstorming/builder-check-prompt.md) asks a fresh reader what consequential information is missing before planning. It excludes details a competent implementer can decide.

**Council adaptation:** use separate researchers for independent evidence, then a focused reviewer for unresolved questions. Avoid repeated approval ceremonies when existing user instructions already authorize the work.

### wshobson: competing explanations tested against evidence

[parallel-debugging](https://github.com/wshobson/agents/blob/7e2fb559cf4b824b27f1fec49fe88f7511066d26/plugins/agent-teams/skills/parallel-debugging/SKILL.md) investigates competing causes. Its [hypothesis template](https://github.com/wshobson/agents/blob/7e2fb559cf4b824b27f1fec49fe88f7511066d26/plugins/agent-teams/skills/parallel-debugging/references/hypothesis-testing.md) records evidence that would support or disprove each cause. Outcomes distinguish confirmed, disproved and inconclusive explanations.

**Council adaptation:** ask researchers to test rival explanations instead of defending an assigned answer. Require a clear link from evidence to conclusion. Do not turn illustrative confidence percentages into calibrated probabilities or rank a claim by how many agents repeat it.

## Calibration lesson from Agency Agents

[Evidence Collector](https://github.com/msitarzewski/agency-agents/blob/b37b5bb33b78f2c1d6567d98e25ca818e3259219/testing/testing-evidence-collector.md) accepts zero reproducible defects and separates missing coverage from product failures. It distinguishes screenshots of appearance from evidence of successful behavior. The separate [Reality Checker](https://github.com/msitarzewski/agency-agents/blob/b37b5bb33b78f2c1d6567d98e25ca818e3259219/testing/testing-reality-checker.md) includes automatic negative expectations and stack-specific commands. Prefer calibrated evidence; do not import assumed defects, artificial ratings or unrelated QA scripts.

## Proposed Council approach

State the decision, success criteria and fixed user constraints first. Gather missing facts only where useful. Collect independent positions, then ask a challenger to address their strongest claims. Let a reviewer explain which objections remain and why the chosen path is preferable. Report the recommendation, practical effect, strongest dissent and the next useful check in plain language.

Scale participants and rounds to the uncertainty and configured call budget. Preserve relevant evidence without invented context caps. Use Council's server-side waiting rather than generating commentary while workers run. Distinguish genuine independent calls, provider diversity and role simulation. A discussion result does not grant execution permission or establish completed work.

The external projects were not installed. Council's original implementation of these ideas is described in [Collaboration playbooks](collaboration-playbooks.md). Repository-level licenses do not settle every aggregated file's provenance. This report paraphrases ideas; direct copying requires retaining the applicable notices and checking the exact source license.
