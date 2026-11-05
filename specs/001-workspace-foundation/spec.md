# Feature Specification: Workspace onboarding and branded task drafts

**Feature Branch**: `refactor`

**Feature Directory**: `specs/001-workspace-foundation`

**Created**: 2026-11-05

**Status**: Implemented and reviewed; Windows ARM64 foundation accepted in [acceptance.md](acceptance.md)

**Input**: Build the first usable Council workspace: open a local project, discover existing
harness installations without model calls, persist the owner's autonomy preference and a task
draft, and expose the same preparation flow through an English Ejupi Labs UI and CLI.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Open a project and retain the operating preference (Priority: P1)

A programmer opens an existing project without preparing transport configuration. Council shows
the actual selected folder and repository context, offers the autonomous full-access preference
as a primary choice, and retains that choice and the project for the next visit.

**Why this priority**: Every later task needs a reliable project identity and an operating choice
that the programmer does not have to repeat.

**Independent Test**: Open a fixture project through the CLI and UI, select the policy, restart
Council and retrieve the same project and policy while the fixture's files and Git state remain
unchanged.

**Acceptance Scenarios**:

1. **Given** an existing project whose path contains spaces and Unicode, **When** it is opened,
   **Then** its selected folder, repository context and setup status are visible without editing
   a configuration file or starting an agent.
2. **Given** two ordinary path aliases for the same physical folder, **When** both are opened,
   **Then** Council resolves one project identity; distinct physical folders remain distinct.
3. **Given** the owner selected autonomous full access once, **When** Council is restarted,
   **Then** the saved preference is shown without another mandatory permission questionnaire.
4. **Given** an invalid or unavailable project path, **When** opening it fails, **Then** Council
   explains the prerequisite and creates no project record or agent attempt for that path.
5. **Given** a project with existing staged and unstaged edits, **When** it is opened or inspected,
   **Then** those contents, staging state and selected branch remain unchanged.

### User Story 2 - Understand existing agent setup (Priority: P2)

A programmer sees the installation status of the five intended harnesses and the next supported
setup action where necessary. Council distinguishes finding an executable from authenticating an
account, verifying a model or having available allowance.

**Why this priority**: The motivating use case needs the programmer to see the available connection
choices without a transport form or an invented readiness claim.

**Independent Test**: Use controlled installation fixtures for present, missing and unsupported
launchers and verify that UI and CLI report the same states with zero model calls and no vendor
credential or global configuration changes.

**Acceptance Scenarios**:

1. **Given** a detected harness installation with no authenticated connection evidence, **When**
   discovery is displayed, **Then** installation is reported and authentication/model access stays
   unverified or unknown; the card does not claim a verified live connection.
2. **Given** one unavailable installation, **When** discovery runs, **Then** other connections
   remain inspectable and the missing installation has a specific user-facing next action.
3. **Given** an unresolved native model or externally unknown account activity, **When** its
   connection is inspected, **Then** Council shows unknown values rather than a guessed model,
   idle account or remaining allowance.
4. **Given** a project or policy draft, **When** discovery refreshes, **Then** that refresh does
   not erase the draft or change the selected project.
5. **Given** a harness that cannot yet execute through this foundation, **When** its setup is shown,
   **Then** the limitation is explicit; a decorative installation card is not an implemented adapter.

### User Story 3 - Prepare a task in the Ejupi Labs workspace (Priority: P3)

A programmer enters an original task with the project already selected, saves the draft, and can
return to it through the UI or CLI. Auto is the visible ordinary choice. The interface uses the
verified Ejupi Labs identity, readable controls and keyboard-accessible feedback.

**Why this priority**: This creates a useful preparation journey for the managed-execution feature
without pretending that a draft is an executed or accepted task.

**Independent Test**: Save a multiline Unicode draft in one interface, retrieve it unchanged in
the other after restart, and exercise the primary UI flow at ordinary and narrow viewport sizes.

**Acceptance Scenarios**:

1. **Given** a selected project, **When** the programmer saves a task using only the task text,
   **Then** Council retains its full original content and a stable linked draft identity with Auto
   defaults; no voting, model-ranking or transport form is required.
2. **Given** a saved draft, **When** Council restarts and the draft is reopened through either
   interface, **Then** the project, original content and saved policy are consistent.
3. **Given** a saved draft in this feature, **When** its status is displayed, **Then** it says
   Draft / Not run and clearly explains that execution is unavailable in this preview.
4. **Given** the task composer and connection views, **When** they are operated by keyboard,
   **Then** controls have names, focus is visible and pending/error feedback is reachable.
5. **Given** a slow or obsolete response, **When** the user selects another project or edits a draft,
   **Then** that response does not overwrite the newer selection or unsaved input.

### Edge Cases

- A selected project is a subdirectory of a repository: retain the chosen working folder and
  distinguish it from the repository root rather than silently moving the user elsewhere.
- A path is missing, inaccessible, nonrepresentable or not a supported project; report the reason
  without modifying that path or creating an apparently ready project.
- Case-distinct physical sibling directories must not be collapsed by a Windows case-folding guess.
- A detected launcher requires an unsupported shell wrapper: report that limitation rather than
  invoking an arbitrary shell or falsely labelling the connection ready.
- Authentication, account identity, usage and effective model may all be unknown during discovery.
- Two clients inspect or edit the same draft: preserve a newer version or report a conflict.
- A state write fails or a second owner is active: no false saved acknowledgement or competing
  writer may be reported as a successful update.
- A draft contains multiline text, leading/interior Unicode markers or a literal replacement
  character: preserve valid original text, reject malformed supplied encoding rather than
  silently replacing bytes, and do not log its body as diagnostics.
- The UI is opened without valid local access or a different web origin tries an action:
  access is refused without exposing local task/project data.
- Brand artwork or an installation is unavailable: use an honest empty/error state, not an
  invented logo, generic replacement icon family or simulated completed agent result.

## Requirements *(mandatory)*

### Functional Requirements

- **WS-001**: Council MUST open a supported existing project through UI and CLI, retain the selected
  working folder and repository context, and preserve original files, staging and branch state.
- **WS-002**: Physical aliases MUST resolve consistently, while distinct physical projects remain
  distinct; invalid or inaccessible paths MUST fail before project admission.
- **WS-003**: Council MUST persist and show the selected autonomy preference without requiring
  repeated per-tool or per-task configuration in this preparation journey.
- **WS-004**: Requested autonomy MUST remain distinct from effective harness capability; discovery
  MUST NOT claim that full access, authentication or model execution was verified.
- **WS-005**: Council MUST expose installation/setup status for Codex, Claude Code, DeepSeek Harness,
  Junie and Antigravity without making model calls, inspecting/copying vendor credentials,
  automatically installing agents or silently changing vendor-global settings.
- **WS-006**: Installation, supported launcher, authentication evidence, model identity, external
  occupancy and allowance MUST remain distinguishable; absent observations MUST remain unknown.
- **WS-007**: A programmer MUST be able to save and reopen a project-linked task draft with only
  task text required, preserving that original text and a stable draft identity.
- **WS-008**: Drafts MUST remain explicitly unexecuted. This feature MUST NOT fabricate agent
  attempts, passed checks, usage receipts or accepted/integrated changes.
- **WS-009**: The UI MUST follow the approved Ejupi Labs identity and icon conventions, with
  retained provenance/notices for shipped assets and English application copy.
- **WS-010**: Primary flows MUST support keyboard use, labelled controls, visible focus, readable
  contrast, non-colour status cues, narrow layouts and reduced-motion preferences.
- **WS-011**: UI and CLI MUST observe the same durable project, policy, connection and draft facts.
  Repeated updates and stale reads MUST NOT overwrite newer state or erase a draft silently.
- **WS-012**: Local collaboration data MUST stay under the documented private `.council/` scope,
  excluded from Git/release contents and separate from legacy collaboration data.
- **WS-013**: The local UI MUST prevent an unauthorized remote observer or unrelated web origin
  from reading private project/draft data or causing state changes.
- **WS-014**: Errors MUST identify the failed operation and a useful next action. A failed save,
  missing prerequisite or active owner conflict MUST NOT produce a false successful receipt.
- **WS-015**: Release users MUST be able to run the foundation without installing a compiler or
  development dependencies. Development retains working `npm test` and `npm run check` entrypoints.

Parent requirements covered in part or in full: FR-001–FR-010, FR-043–FR-048. Coverage of a parent
spanning later features is not declared complete merely because this foundation exists.

### Key Entities *(include if feature involves data)*

- **Project**: Stable identity, original selected location, resolved working location, repository
  context and setup observations. It is not a snapshot of vendor credentials.
- **Connection observation**: Harness identity, installation/launcher evidence and separately
  known or unknown capabilities/readiness facts. It is not a provider account entitlement.
- **Autonomy preference**: Saved operating choice and scope; effective permissions remain a later
  supported-adapter observation.
- **Task draft**: Stable project link, complete original text, revision, saved policy context and
  unexecuted status. Later execution must not retroactively claim this draft already ran.
- **Local owner**: The component authorized to change this foundation's local records. Other
  interfaces must use the same ownership model instead of becoming competing writers.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a prepared fresh installation, opening a supported project and saving the first
  draft requires no transport JSON, mandatory model choice, voting form or development toolchain.
- **SC-002**: UI/CLI acceptance preserves the exact project-linked multiline Unicode draft and
  autonomy preference across restart; pre-existing source/staging/branch identities are unchanged.
- **SC-003**: Every controlled installed/missing/unverified connection scenario reports its actual
  evidence and next action, with exactly zero model calls and zero vendor credential changes.
- **SC-004**: The primary opening/discovery/draft flows are keyboard-operable and usable at 360 CSS
  px and normal desktop widths; inspected renders use approved brand assets and named controls.
- **SC-005**: Duplicate project aliases, stale draft updates, failed writes and unauthorized UI
  actions are rejected or reconciled without duplicate identities or false saved acknowledgements.
- **SC-006**: An extracted release foundation starts and exposes its preparation journey with the
  documented platform prerequisites; the evidence identifies the tested OS/architecture and version.
- **SC-007**: Every feature requirement has an implementation task and acceptance evidence, with
  independent review and corrected blocking findings before feature 002 begins.

## Assumptions

- The first release targets local Git projects. Unsupported project types get a clear setup result.
- Vendor sign-in and installation are owned by the vendor; this feature's discovery is not a live
  account, quota or model-access probe.
- The UI and CLI preparation flows are part of this feature. Managed execution, full account-aware
  routing, review/integration, task graphs and MCP arrive in their ordered later features.
- The saved policy is requested intent. A future adapter must establish and report its supported
  effective policy; there is no universal full-access or OS-sandbox claim.
- Legacy 1.3 state remains separate. This feature does not migrate or reinterpret it.
- Development and acceptance use controlled installations and fake observations. No live provider
  task or measured subscription saving is claimed.
