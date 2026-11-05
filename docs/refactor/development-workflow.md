# Specify development workflow

## Installed workflow

This branch uses GitHub Spec Kit / Specify CLI **0.16.1**, initialized for the Codex skills
integration with PowerShell scripts. The generated `.specify/` infrastructure and
`.agents/skills/speckit-*/SKILL.md` files are development tooling. They do not implement runtime
agent interoperability and do not make provider calls during initialization.

Initialization used:

```powershell
specify init --here --integration codex --integration-options="--skills" --script ps --force
```

The repository was clean before initialization, the generated paths were checked for collisions,
and the existing application and `AGENTS.md` were unchanged by the initializer. The project then
authored [constitution 1.0.0](../../.specify/memory/constitution.md) and its product baseline.
Retained upstream license: [Spec Kit MIT notice](../../third-party/spec-kit/LICENSE).

The optional Git extension is not installed. The branch stays exactly `refactor`; feature context
is independent of Git branch naming. `.specify/feature.json` is machine-local and ignored through
the generated `.specify/.gitignore`. Numbered feature artifacts are committed under `specs/`.
The future contexts named in the roadmap are planned paths, not pre-generated feature work.

## One feature at a time

1. Check the current branch and select the next unaccepted roadmap feature.
2. Apply `$speckit-specify` with that feature's natural-language scope. Resolve its template with
   `specify preset resolve spec-template`, create its one feature directory and persist the
   active context. Fill `spec.md` and its separate requirements-quality checklist.
3. Resolve material ambiguity using the user instructions and documented defaults. Ask only for
   a missing consequential decision that has no reasonable authorized default; continue other
   useful work while awaiting an answer.
4. Apply `$speckit-plan`. Run the generated `setup-plan.ps1 -Json`, then prepare the technical
   plan, research decisions, data model, external contracts and runnable acceptance guide.
5. Apply `$speckit-tasks`. Run `setup-tasks.ps1 -Json`, then generate dependency-ordered checklist
   entries with task IDs, user-story labels and exact file paths.
6. Apply `$speckit-analyze` to review cross-artifact consistency. Resolve blocking gaps and commit
   the specification/design/task artifacts before application implementation for this feature.
7. Apply `$speckit-implement`. Complete the active feature with meaningful checks and explicit
   file ownership for parallel work. Do not start implementing the next feature in parallel.
8. Obtain independent review of the implementation and evidence. Record findings in `review.md`,
   correct blocking issues and reverify the affected acceptance conditions against the correction.
9. Use `$speckit-converge` when needed to find remaining gaps; appended tasks are still subject to
   implementation, review and actual verification. It is not a substitute for acceptance evidence.
10. Complete `acceptance.md`, update traceability and commit the reviewed feature before moving on.

These names invoke the installed agent skills; they are not executable shell commands. The
generated PowerShell helpers prepare and locate artifacts. Do not launch another live vendor agent
to simulate executing the skills. This project's independent review closes review gates without
an extra user approval at every phase; already authorized development continues autonomously.
The bundled `speckit` workflow YAML is an upstream example, not a requirement to introduce a second
interactive approval process or automatically start an external integration CLI.

## Context and branch discipline

Use one active feature context per checkout. The environment override `SPECIFY_FEATURE_DIRECTORY`
may select a feature explicitly; do not run competing feature-generation phases against the same
shared pointer. Check the branch before and after helper execution. Do not install a branch-changing
hook or overwrite another feature's context while its phase is active.

Technology is open. Feature 001 records the initial decision and its constraints; later deviations
need an updated design and impact review. Keep `npm test` and `npm run check` as working developer
entrypoints even if they delegate to a replacement toolchain. Preserve generated runtime/release
entrypoints as required by the accepted packaging plan.

Development uses local fake agents and explicitly labelled simulation. Live-provider qualification
is separate and requires a specific user-requested task. Record actual tool/runtime versions,
test outcomes, inspected UI artifacts and unperformed checks; never infer a passed test from a
queued job, terminated child, successful setup or completed response.

## Sources

- [Spec Kit 0.16.1 core reference](https://github.com/github/spec-kit/blob/v0.16.1/docs/reference/core.md).
- [Adopting Spec Kit in an existing project](https://github.github.io/spec-kit/how-to/existing-projects.html).
- [Codex integration reference](https://github.com/github/spec-kit/blob/v0.16.1/docs/reference/integrations.md).
- [Agentic specification-driven development](https://github.com/github/spec-kit/blob/v0.16.1/docs/reference/agentic-sdd.md).
