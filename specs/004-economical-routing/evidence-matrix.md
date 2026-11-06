# Economical routing evidence matrix

Qualification checkpoint: 6 November 2026, API1 / execution004-v1 / schema4. This maps the
[28 requirements and ten success criteria](spec.md) to actual source and test identifiers.
It is an evidence index, not acceptance or independent approval by its domain/store author.
The design gate was `e9b37ad`; final implementation commit/native qualification remain pending.

## Observed checks and limits

Root ran the checks below; this matrix author inspected source/logs and ran no additional checks.
Private log names resolve under `.council/refactor/`, outside Git and release artifacts.

- **C1 — core:** final full store passed 4.146s (`004-test-final.log`), including the new
  authority regressions; earlier full store passed 3.225s (`004-store-all.log`). Full pure routing passed
  0.120s (`004-routing-final-authoring.log`); focused domain/adapter checks passed
  (`004-domain-adapters.log`). Explicit-empty requests passed domain/store 0.074/1.445s
  (`004-presence.log`); standalone reference-price correction passed domain/CLI 0.079/0.136s
  (`004-price-presence.log`) and received independent source closure.
- **C2 — authority/integration:** focused engine/HTTP passed 5.283/0.069s
  (`004-engine-http.log`); continuation passed 7.288s (`004-engine-continuation.log`);
  preparation/Stop ordering passed 2.628s (`004-preparation-ordering.log`). Source ordering
  passed 8.597s (`004-engine-source-ordering.log`); project-identity replacement red/green
  ended at 1.727s (`004-project-proof*.log`). Inherited-reference red/green ended at 0.118s
  (`004-inheritance*.log`). Final negative authority/price/rearm cases passed 0.445s
  (`004-routing-authority.log`), using DAO fixtures with no native-worker/provider launches.
- **C3 — interfaces:** final `npm test` passed every Go package and **39/39 browser cases**
  (`004-test-final.log`), including all 16 routing cases and shared lifecycle tests. The earlier
  38/39 browser run's stale-copy assertion passed its focused correction in 4.116s
  (`004-browser-version.log`). Synthetic schema1/2 CLI downgrade repair passed its focused
  27.323s run (`004-cli-migration.log`); production rejection was not weakened.
- **C4 — package:** `scripts/package.test.mjs` case
  `native preview has exact contents, complete notices and an independently verifiable checksum`
  passed **1/1 in 9.849s** in the final suite (`004-test-final.log`); the earlier standalone
  run passed 11.9167s (`004-package.log`). It exercises the extracted executable, restricted
  execution PATH, actual owner/owned fake workers, economical Claude route, receipts,
  ledger/usage and exact Stop.
- **C5 — visual/accessibility:** Root's actual QA run produced **28 captures, 15 keyboard checks
  and ten approved asset comparisons** (`004-render-qa.log` and private
  `screenshots/004-economical-routing/capture-evidence.json`). It records 1440px/360px states,
  no horizontal overflow, visible focus, reduced-motion checks and matching source/dist/served
  SHA256 values. Root reports opening **eight** captures; this author did not view the images
  and does not claim all 28 received visual inspection.
- **HOSTED GATES PENDING:** final `npm test` rebuilt the embedded UI/native binary and passed;
  final `npm run check` also exited 0 (`004-check-final.log`). Independent source/correction
  review is closed in review.md. Immutable implementation commit, six native platform jobs and
  Linux race qualification remain required. No hosted004 qualification or feature acceptance
  is claimed.

Browser keys below name exact cases, not inferred coverage:

- **B1**, [routing-start.test.mjs](../../ui/tests/routing-start.test.mjs):
  `economical Auto needs only the project, task and saved autonomy, with truthful finite defaults`.
- **B2**, [routing-capacity.test.mjs](../../ui/tests/routing-capacity.test.mjs):
  `optional reported busy Codex and available Claude route useful fake work without claiming a login or free allowance`.
- **B3**, [routing-capacity.test.mjs](../../ui/tests/routing-capacity.test.mjs):
  `declared account association carries an inherited exhaustion through unlink and exact origin correction`.
- **B4**, [routing-budget.test.mjs](../../ui/tests/routing-budget.test.mjs):
  `the third explicit attempt blocks at two units and guidance cannot replenish the shared window`.
- **B5**, [routing-budget.test.mjs](../../ui/tests/routing-budget.test.mjs):
  `actual schema3 adoption and its dependent Start retain separate identities through two lost acknowledgements`.
- **B6**, [routing-budget.test.mjs](../../ui/tests/routing-budget.test.mjs):
  `a lost grant reply spends no worker and retries its exact amendment without overwriting newer grant input`.
- **B7**, [routing-history.test.mjs](../../ui/tests/routing-history.test.mjs):
  `a late usage page cannot replace another selected task or its current budget`.
- **B8**, [routing-history.test.mjs](../../ui/tests/routing-history.test.mjs):
  `controlled usage wire data preserves huge exact counters and estimates without claiming full-task totals or charges`.
- **B9**, [routing-start.test.mjs](../../ui/tests/routing-start.test.mjs):
  `a delayed routed admission preserves a newer retained-task selection and literal incompatible pins block`.
- **B10**, [routing-start.test.mjs](../../ui/tests/routing-start.test.mjs):
  `a delayed configuration refresh preserves an explicit unsaved Auto profile for the next guidance version`.
- **B11**, [routing-budget.test.mjs](../../ui/tests/routing-budget.test.mjs):
  `continuation defaults OFF and the unsupported terminal category is disclosed rather than enabled`.
- **B12**, [routing-start.test.mjs](../../ui/tests/routing-start.test.mjs):
  `lost economical Start freezes optional profile and budget while newer composer input survives retry`.

## Requirement mapping

Tests linked here are present in the actual files. C1–C5 identify observed local execution;
the final complete-source gates above remain pending. Shared003 lifecycle tests below also ran
in C3's Go stage; local fake execution is never live provider qualification.

| Requirement | Implementation and concrete evidence |
| --- | --- |
| RTE-001 — distinct identities | [DTOs](../../internal/domain/routing.go), [native observation handling](../../internal/engine/results.go); [TestRoutingProfileLaunchesOwnedClaudeFixtureWithoutChangingIntent](../../internal/engine/routing_admission_test.go), [TestNativeObservationBindingsBoundsAndFakeSeparation](../../internal/store/observations_test.go), B2/B8. C1–C3. |
| RTE-002 — minimal useful defaults | [Start](../../internal/engine/engine.go), [catalog/default policy](../../internal/routing/catalog.go), [composer](../../ui/src/tasks.ts); B1 and [TestRoutingCLIActualOwnerSelectsReportedContextAndDebitsOnce](../../internal/cli/routing_integration_test.go). C3/C4. |
| RTE-003 — deterministic, zero-model routing | [selection](../../internal/routing/select.go), [assessment](../../internal/routing/assess.go), [metadata-only facts](../../internal/engine/routing_facts.go); [TestSelectionRolesPinsAndManagedQueue](../../internal/routing/select_test.go), [TestAssessmentRequiresWholeBoundedOperationAndRetainsSourceProof](../../internal/routing/assess_test.go). C1/C3; no model classifier exists. |
| RTE-004 — suitability before economy, honest unknowns | [selection/prices](../../internal/routing/select.go); [TestSelectionCapacityAndVersionNegativesPrecedeEconomy](../../internal/routing/select_test.go), [TestPriceFrontierHandlesIncomparabilityWithoutComparatorCycles](../../internal/routing/select_test.go), [TestAssessmentConservativeUnknownAndDeclaredAttribution](../../internal/routing/assess_test.go). C1/C3. |
| RTE-005 — unknown eligible, negatives win | [capacity](../../internal/routing/capacity.go); [TestCapacityUnknownAndManagedBusyAreQueueEligible](../../internal/routing/capacity_test.go), [TestCapacityRequirePositiveNeedsBothCoveredFreshDimensions](../../internal/routing/capacity_test.go), [TestCapacityStickyExhaustionSurvivesAgeRelinkAndNewHead](../../internal/routing/capacity_test.go). C1/C3. |
| RTE-006 — declared pools without account attestation | [accounts/links](../../internal/store/routing_accounts.go); [TestContextPositiveCannotTransferThroughDeclaredAccount](../../internal/routing/capacity_test.go), [TestRoutingConcurrentIndependentAdmissionsShareDeclaredPoolAndOneWorker](../../internal/cli/routing_capacity_integration_test.go), B3. C1/C3. |
| RTE-007 — separate sourced capacity dimensions | [report DTOs](../../internal/domain/routing.go), [reports](../../internal/store/routing_capacity.go); [TestRoutingCapacityReportFaultReplayAndStaleRevision](../../internal/store/routing_capacity_test.go), [TestElapsedCooldownDoesNotInventAllowance](../../internal/routing/capacity_test.go), B2. C1/C3. |
| RTE-008 — expiry, sticky exhaustion and exact recovery | [current report state](../../internal/store/routing_capacity.go); [TestRoutingCapacityRetainedOldCorrectionAndLinkInheritance](../../internal/store/routing_capacity_test.go), [TestRoutingCLIAccountCorrectionRetainsImmutableReportAndNewerDenial](../../internal/cli/routing_integration_test.go), B3. C1–C3, including while-linked provenance red/green. |
| RTE-009 — reported Codex→Claude journey | [selection](../../internal/routing/select.go), [facts](../../internal/engine/routing_facts.go); [TestRoutingCLIActualOwnerSelectsReportedContextAndDebitsOnce](../../internal/cli/routing_integration_test.go), B2, extracted-package C4. These use user reports/fakes, not inspected personal conversations or credentials. |
| RTE-010 — pins and literal selectors | [validation](../../internal/domain/execution_validation.go), [resolved builder](../../internal/adapters/routing.go); [TestRoutingLaunchRejectsConflictsBeforeResolvingAnExecutable](../../internal/adapters/routing_test.go), [TestRoutingConflictsHaveNoReceiptTaskOrWorker](../../internal/engine/routing_admission_test.go), B9. C1–C3. |
| RTE-011 — no unauthorized paid/strong guarantee path | [eligibility](../../internal/routing/select.go), [strict presence](../../internal/domain/routing_presence.go); [TestRoutingConflictsHaveNoReceiptTaskOrWorker](../../internal/engine/routing_admission_test.go), [TestRoutingCLIReferencePriceFileRejectsEmptyOptionalMembersBeforeMarshal](../../internal/cli/routing_price_presence_test.go). C1–C3. Paid executor/verified monetary cap are unsupported, not positively qualified. |
| RTE-012 — common finite budget/window | [admission](../../internal/store/routing_admission.go), [budget](../../internal/store/routing_budget.go), [launch ledger](../../internal/store/attempts.go); [TestRoutingBudgetCountsReplayAndExplicitAdditionalAuthority](../../internal/store/routing_budget_test.go), [TestRoutingManagedOccupancyQueuesWithoutInventingIneligibility](../../internal/engine/routing_admission_test.go), B4. C1–C4. |
| RTE-013 — exact durable admission accounting | [transactional admission](../../internal/store/routing_admission.go); [TestRoutingBudgetAdmissionLostAcknowledgementIsOneUnit](../../internal/store/routing_budget_test.go), [TestRoutingBudgetPersistsLargeCountsAndRejectsCumulativeOverflow](../../internal/store/routing_budget_test.go), [TestRoutingContinuationPreCommitFaultDoesNotSpendUnit](../../internal/store/routing_continuation_test.go). C1/C2; concurrency in C3's independent-client test. |
| RTE-014 — no implicit refill/rearm | [amendment](../../internal/store/routing_budget.go), [continuation revisions](../../internal/store/routing_continuation.go); [TestRoutingGuidanceContinuationUsesExistingGrantWithoutRefill](../../internal/store/routing_budget_test.go), [TestRoutingGrantAndGuidanceCannotRearmOldProofButExplicitStartWorks](../../internal/store/routing_authority_test.go), B4/B6. C1–C3; authority additions observed C2, 0.445s. |
| RTE-015 — bounded opt-in A, no invented B/escalation | [continuation](../../internal/store/routing_continuation.go), [category validation](../../internal/domain/routing_validation.go); [TestRoutingQualifiedFreshBlockCreatesOneOwnedSuccessorOnlyWhenEnabled](../../internal/engine/routing_continuation_test.go), [TestRoutingContinuationRequiresCapturedReplacementAndEscalationAuthority](../../internal/store/routing_authority_test.go), [TestRoutingCurrentPriceCannotRewriteCapturedEscalationBasis](../../internal/store/routing_authority_test.go), B11. C1–C3. |
| RTE-016 — current guards, immutable queued target | [dispatch guard](../../internal/store/routing_dispatch.go), [Prepare ordering](../../internal/engine/results.go); [TestRoutingEligibilityMutationCannotCommitBetweenGuardAndPreparation](../../internal/engine/routing_preparation_test.go), [TestRoutingUnknownCapacityCoverageCannotAuthorizeReplacement](../../internal/store/routing_continuation_test.go), independent-client queued-target assertions. C1–C3. |
| RTE-017 — Stop/owner/evidence/persistence dominate fallback | [continuation/Stop](../../internal/store/routing_stop.go), [engine lifecycle](../../internal/engine/results.go); [TestRoutingStopBetweenSettlementAndContinuationPreservesTerminalAndPreventsSuccessor](../../internal/engine/routing_continuation_test.go), [TestRoutingNewOwnerCannotRearmRetainedPrelaunchProof](../../internal/engine/routing_continuation_test.go), [TestInteropClaudeOwnedDescendantAndUnknownCleanupKeepHonestCapacity](../../internal/engine/interop_control_test.go). C2/C3. |
| RTE-018 — settlement/proof, never prose authority | [proof producer](../../internal/store/routing_dispatch.go), [proof consumer](../../internal/store/routing_continuation.go); [TestRoutingContinuationOneLinkedUnitAndImmutablePredecessor](../../internal/store/routing_continuation_test.go), [TestRoutingOptionsCategoryAndBudgetBoundaries](../../internal/domain/routing_validation_test.go), B11. C1–C3. No terminal-transient success qualification exists. |
| RTE-019 — immutable attributable decisions | [decision DTOs](../../internal/domain/routing.go), [same-commit snapshots](../../internal/store/routing_admission.go); [TestRoutingReadsKeepCapturedAuthorityAndDoNotSpendBudget](../../internal/engine/routing_reads_test.go), [TestRoutingCurrentPriceCannotRewriteCapturedEscalationBasis](../../internal/store/routing_authority_test.go). C1–C3. |
| RTE-020 — replay before prerequisites | [fingerprints](../../internal/store/fingerprints.go), [receipt transaction](../../internal/store/store.go); [TestRoutingBudgetAdmissionLostAcknowledgementIsOneUnit](../../internal/store/routing_budget_test.go), [TestRoutingConfigurationChangesDoNotRefillCapturedTask](../../internal/store/routing_capacity_test.go), B12/B6. C1/C3/C4. |
| RTE-021 — bounded exact scoped usage history | [projection](../../internal/routing/usage.go), [all-outcome query/paging](../../internal/store/routing_reads.go); [TestUsageSumsExactScopesOnceAndKeepsMissingFakeAndEstimatesSeparate](../../internal/routing/usage_test.go), [TestRoutingUsageWholeItemBudgetStableCursorAndExactPageSubtotal](../../internal/store/routing_usage_test.go), B7/B8. C1/C3. Subtotals cover returned attempts only. |
| RTE-022 — estimates/prices/charges/unknown distinct | [usage](../../internal/routing/usage.go), [reference-price validation](../../internal/domain/routing_presence.go); [TestUsageUnknownNeverBecomesZeroAndAllHundredMaximaFit](../../internal/routing/usage_test.go), [TestRoutingCLIReferencePriceFilePreservesOmissionAndExtremeLexeme](../../internal/cli/routing_price_presence_test.go), B8. C1/C3; fake usage excluded, charges unknown. |
| RTE-023 — shared interface/control semantics, newer edits survive | [HTTP](../../internal/httpapi/routing.go), [CLI](../../internal/cli/routing.go), [UI](../../ui/src/routing.ts); [TestRoutingHTTPDispatchesNestedHistoryAndSeparateQueryScopes](../../internal/httpapi/routing_test.go), [TestRoutingCLIContractOperationsAndCurrentOwnerGate](../../internal/cli/routing_test.go), B7/B9/B10/B12. C3. |
| RTE-024 — preserved upgrades and explicit legacy adoption | [additive migration](../../internal/store/routing_migration.go), [adoption](../../internal/store/routing_budget.go); [TestRoutingMigrationPreservesAcceptedSchema3WithoutGrantingAuthority](../../internal/store/routing_migration_test.go), [TestPopulatedPreviousSchemasUpgradeThroughCLIWithoutReplayingHistory](../../internal/cli/interop_integration_test.go), [TestRoutingLegacyAdoptionAppendsVersionAndDoesNotStart](../../internal/store/routing_budget_test.go), B5. C1/C3; schema1/2 fixture is synthetic, schema3 fixture uses frozen accepted SQL. |
| RTE-025 — shared source/isolation/lifecycle, no inferred acceptance | [run lifecycle](../../internal/engine/results.go), [workspace capture](../../internal/workspace/capture.go); [TestInteropClaudeCapturedInstructionsAndCandidatePreserveOriginalEditsIndexBranch](../../internal/engine/interop_workspace_test.go), [TestRoutingProjectReplacementAfterInitialIdentityCheckBlocksBeforePreparation](../../internal/engine/routing_admission_test.go), [TestInteropClaudeResponsePublicationFaultCannotBecomeCandidateSuccess](../../internal/engine/interop_results_test.go). C2/C3. |
| RTE-026 — identity, keyboard, compact 360px flow | [markup](../../ui/src/index.html), [styles](../../ui/src/style.css), [routing UI](../../ui/src/routing.ts); B1/B2 plus C5's actual renders, focus/reduced-motion and ten asset hashes. Root opened eight images; no blanket visual approval by this author. |
| RTE-027 — bounded zero-model observation and named limits | [bounded reads](../../internal/store/routing_reads.go), [source assessor](../../internal/routing/assess.go); [TestRoutingReadsDefaultThirtyAndKeepCursorHighWater](../../internal/engine/routing_reads_test.go), [TestRoutingReadRejectsMalformedIdentitiesFiltersAndLimits](../../internal/engine/routing_reads_test.go), [TestRoutingUsageWholeItemBudgetStableCursorAndExactPageSubtotal](../../internal/store/routing_usage_test.go). C1/C3. |
| RTE-028 — fake development, independent review and gates | [review record](review.md), [package test](../../scripts/package.test.mjs), C1–C5. Shared actual owners/contained fake workers and labelled synthetic usage exercised; no vendor authentication/usage measured. Final developer gates passed; commit/native/race gates and acceptance **PENDING**. |

## Success-criterion mapping

| Criterion | Concrete evidence and remaining boundary |
| --- | --- |
| SC-001 | B1, [TestRoutingCLIActualOwnerSelectsReportedContextAndDebitsOnce](../../internal/cli/routing_integration_test.go), extracted executable C4. No compulsory account/quota/transport form. |
| SC-002 | [TestSelectionRolesPinsAndManagedQueue](../../internal/routing/select_test.go), [TestPriceFrontierHandlesIncomparabilityWithoutComparatorCycles](../../internal/routing/select_test.go), [TestAssessmentConservativeUnknownAndDeclaredAttribution](../../internal/routing/assess_test.go); C1/C3. Pure policy introduces no model consultation; no measured quality claim. |
| SC-003 | B2/B3 and [TestRoutingConcurrentIndependentAdmissionsShareDeclaredPoolAndOneWorker](../../internal/cli/routing_capacity_integration_test.go); C3. Distinct clients, one owner/global worker, unknown actual login. |
| SC-004 | [TestCapacityRequirePositiveNeedsBothCoveredFreshDimensions](../../internal/routing/capacity_test.go), [TestElapsedCooldownDoesNotInventAllowance](../../internal/routing/capacity_test.go), [TestRoutingCapacityRetainedOldCorrectionAndLinkInheritance](../../internal/store/routing_capacity_test.go); C1/C3. |
| SC-005 | [TestRoutingConflictsHaveNoReceiptTaskOrWorker](../../internal/engine/routing_admission_test.go), [TestRoutingLaunchRejectsConflictsBeforeResolvingAnExecutable](../../internal/adapters/routing_test.go), B9; C1–C3. Unsupported actual-account/payment/stronger guarantees are blockers, not qualified executors. |
| SC-006 | [TestRoutingBudgetCountsReplayAndExplicitAdditionalAuthority](../../internal/store/routing_budget_test.go), [TestRoutingContinuationRequiresCapturedReplacementAndEscalationAuthority](../../internal/store/routing_authority_test.go), [TestRoutingKnownNonEscalatingReplacementRemainsUsable](../../internal/store/routing_authority_test.go), [TestRoutingCurrentPriceCannotRewriteCapturedEscalationBasis](../../internal/store/routing_authority_test.go), B4/B11. C1–C3; final authority subcases passed 0.445s. |
| SC-007 | [TestRoutingStopBetweenSettlementAndContinuationPreservesTerminalAndPreventsSuccessor](../../internal/engine/routing_continuation_test.go), [TestRoutingNewOwnerCannotRearmRetainedPrelaunchProof](../../internal/engine/routing_continuation_test.go), [TestInteropClaudeRunningDeadlinePreservesFailureAndVerifiedCleanup](../../internal/engine/interop_control_test.go), [TestInteropClaudeResponsePublicationFaultCannotBecomeCandidateSuccess](../../internal/engine/interop_results_test.go). C2/C3. |
| SC-008 | [TestUsageSumsExactScopesOnceAndKeepsMissingFakeAndEstimatesSeparate](../../internal/routing/usage_test.go), [TestRoutingUsageWholeItemBudgetStableCursorAndExactPageSubtotal](../../internal/store/routing_usage_test.go), B7/B8; C1/C3. All-outcome query is retained; controlled counts/estimates are not real provider telemetry. |
| SC-009 | Frozen003 migration/adoption tests and schema1/2 CLI check in RTE-024; B5/B6/B7/B9/B10/B12; [TestKnown003MaintenanceCannotGrant004ExecutionOrRouting](../../internal/cli/routing_test.go); C3/C5. Old history remains old history; final full local suite passed, native/commit gates remain pending. |
| SC-010 | This 38-row index and [review.md](review.md) identify source, tests, corrections and qualification limits. C1–C5 and final build/test/check are observed local evidence; independent source review is closed. The immutable checked commit and all six native plus Linux race jobs are **PENDING**; the criterion is not yet fully satisfied. |

No live provider task, account-switching qualification, financial-cap enforcement, direct paid
executor, model-quality benchmark, automatic review/integration or feature005 implementation is
established by this matrix. Root owns final review and acceptance after the remaining gates.
