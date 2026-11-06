# Requirements review: economical routing and account capacity

**Status**: Closed; requirements ready for planning. This is not design, implementation or
runtime acceptance. Review date: 2026-11-06.

Root authored the specification using the resolved Specify template. Three independent readers
reviewed budget/capacity/control boundaries, native capability honesty and holistic requirements
quality against the baseline and constitution. No runtime source, provider call or test execution
was used to claim specification readiness.

| Finding | Priority | Correction and independent recheck |
| --- | --- | --- |
| S004-01: unknown task suitability had no observable fallback distinct from unknown availability. | P2/Medium | US1.7, RTE-004 and SC-002 require a constraint-compatible conservative general/native default or a named blocker. No routine/small suitability or proven economy is inferred. Holistic reviewer closed the finding. |
| S004-02: freshness wording could turn expired exhaustion into default-eligible unknown. | P2 | RTE-005/008 retain denying exhaustion, with age visible, until attributable correction/recovery. Temporary cooldown expiry remains separate and never implies replenishment. Native reviewer closed the finding. |
| S004-03: two admissions/30 minutes was merely recommended and its time anchor was unclear. | P2 | RTE-012, SC-006 and assumptions make the new-task default normative, anchored at first budgeted admission with queue/preparation/settlement included. All attempts share it. Native and budget reviewers closed the finding. |
| S004-04: future execution of unadopted legacy tasks was ambiguous. | P2 | RTE-024, US4.6 and SC-009 preserve old inspection/control/exact replay but require explicit adoption before new work. Adoption captures additional authority and retains known prior use; migration grants none. Native and budget reviewers closed the finding. |
| S004-05: a policy-named reason could be mistaken for proof authorizing continuation. | P2 | RTE-015/017/018 and the closed category A/B table require concrete qualified proof. Category A requires durable blocked/no-preparation/no-creation/not-launched evidence; intentionally absent worker output is the sole no-output exception. Category B requires structured terminal transient rejection and complete expected evidence/root/cleanup. Prose, generic numbers, retry progress, authentication/model-access failures and uncertainty do not supply authority. Budget and native reviewers closed the finding. |
| S004-06: queued reselection could be implemented as a free mutation of an admitted target. | P2 | RTE-016 and SC-006 make the admitted target immutable. Ineligibility before dispatch settles the old attempt; an authorized replacement receives a new linked decision/attempt and spends another unit. Default continuation OFF causes no automatic replacement; a labelled pre-admission proposal is not an attempt. Budget reviewer closed the finding. |

No Critical, High or remaining Medium/P2 requirement finding remained after independent recheck.
The final draft contains 28 RTE requirements, ten success criteria and four prioritized stories.
Optional report validity is concrete at 15 minutes. Source installation, model/access/profile,
declared account association, actual account, allowance and financial uncertainty remain distinct.

Root also adopted the budget reviewer's nonblocking wording precision that ineligibility applies
before dispatch rather than rewriting already-running work. Deadline wording preserves the
accepted owned-cleanup behavior: expiry requests controlled Stop and prevents new work, while
bounded cleanup can finish later without extending task authority or promising OS hard real time.
Council-managed continuation limits do not claim control of native internal requests/fallback.

Planning must now resolve the deterministic suitability rubric, capability/reference provenance,
data/control contracts, migrations, exact accounting and bounded UI/CLI operations. The selected
stack remains a design decision inherited from feature 001; the specification prescribes no new
runtime dependency, database schema or endpoint. A separate design/task consistency review and
commit are required before application implementation.
