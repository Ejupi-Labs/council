# Lazy Genius: simplicity review research

Retrieved **5 November 2026**. The user's requirement is the foundation: deliver the complete expected result and constraints, starting with the simplest suitable design. This review examined primary instructions and engineering guidance. No external skill was installed, downloaded code executed, vendor agent invoked or credentials inspected.

## Relevant source patterns

### Anthropic: simplify without changing the contract

The official [code-simplifier agent](https://github.com/anthropics/claude-plugins-official/blob/main/plugins/code-simplifier/agents/code-simplifier.md) favors understandable code, preserved behavior and focused changes. It cautions against dense expressions, collapsed responsibilities and removal of useful abstractions merely to reduce size. Its [plugin license](https://github.com/anthropics/claude-plugins-official/blob/main/plugins/code-simplifier/LICENSE) is Apache-2.0.

For Council, borrow the review criteria. The upstream agent is an autonomous post-edit rewriter and specifies Opus. A Lazy Genius reviewer should also assess plans before implementation, preserve chosen models and avoid a mandatory extra rewriting call. Simpler code still needs to satisfy its actual contract, including exact outputs.

### Karpathy-inspired guidance: stop speculative scope

The community [CLAUDE.md](https://github.com/multica-ai/andrej-karpathy-skills/blob/main/CLAUDE.md) targets speculative features, unnecessary generality, unrelated edits and weak verification. The [skill file](https://github.com/multica-ai/andrej-karpathy-skills/blob/main/skills/karpathy-guidelines/SKILL.md) declares MIT, but a repository-root `LICENSE` was not found at the inspected URL. This is community guidance inspired by Karpathy, not his endorsement.

Council should keep the task connection and explicit success criteria. Do not adopt fixed line-count comparisons, a universal ban on single-use abstractions, or automatic clarification stops. An abstraction can clarify one important boundary; fewer lines can conceal more complexity. Use original wording rather than copying the standalone instructions.

### Fowler: defer future capability, preserve maintainability

[YAGNI](https://martinfowler.com/bliki/Yagni.html) argues against paying today for capabilities justified only by presumed future needs. It explicitly preserves refactoring, testing and other practices that keep software changeable. This provides the central distinction: postpone speculative machinery, not work needed for a reliable current result.

For example, a requested CSV export does not automatically need a format registry and extension lifecycle. Direct export code may be sufficient. Existing output guarantees, validation and failure reporting still belong to the present task. State the real condition that would make a registry useful later, rather than building it immediately.

[Beck Design Rules](https://martinfowler.com/bliki/BeckDesignRules.html) places working behavior and understandable intent ahead of minimizing elements. This supports evaluating design simplicity after confirming correctness. Fowler's pages retain their copyright notices; this report paraphrases concepts, not a reusable prompt license.

### Google: review necessary complexity and avoid review perfectionism

Google's [review guidance](https://google.github.io/eng-practices/review/reviewer/looking-for.html) challenges unnecessary generality while still examining behavior, concurrency, user impact and useful tests. Its [review standard](https://google.github.io/eng-practices/review/reviewer/standard.html) separates consequential findings from optional polish and accepts justified alternatives. A reviewer can overengineer the review itself by demanding perfection or unrelated cleanup. The repository [license](https://github.com/google/eng-practices/blob/master/LICENSE) is CC BY 3.0; attribution is retained here.

## Council adaptation

The Lazy Genius lens should ask what each proposed component, dependency, setting, fallback or special case accomplishes for a current requirement. Compare the simplest complete option with the proposed design. Consider understanding, operations, coupling and future repair work, not only implementation size.

For a material finding, identify the unnecessary mechanism, its current cost, the simpler complete alternative and the verification that protects the contract. Name an observable future trigger for reconsidering deferred complexity. If the evidence supports the design, approve it without inventing simplifications. If an important fact is missing, identify that fact rather than guessing.

Rare failures need consequence-aware judgment. An uncommon authentication bypass, destructive write or loss of committed data can justify protection. A hypothetical use case with no credible path or current obligation may not. Unknown frequency is not proof of irrelevance. Preserve required security, accessibility, compatibility, correctness and proportionate verification.

Keep the lenses distinct: **Bazooka** selects economical models and collaboration; **Constraint reviewer** checks stated limits; **Constructive skeptic** challenges claims; **Lazy Genius** asks whether the machinery is justified. They can complement each other without requiring a committee.

Review findings can withhold a recommendation for an overbuilt choice within the assigned mandate. A skill is not a new runtime veto, permission grant or automatic cancellation mechanism. Use existing author/reviewer/host responsibilities, preserve required JSON formats and avoid recursive delegation. Do not impose fixed participant counts, line limits, abstraction thresholds or an automatic rewrite after every task.
