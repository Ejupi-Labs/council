-- Frozen accepted005 DDL at 794f08032b96434d1217f8bb3797c6803263f39c.
-- Test-only historical schema; never derive it from current migration constants.
-- Frozen accepted feature004 3f4a18e11de495f7b3138d61e030525a018868a5.
-- Source DDL only; local fixtures seed explicit historical records without current defaults.

CREATE TABLE policy (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),
 data TEXT NOT NULL
);
CREATE TABLE projects (
 sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),
 id TEXT NOT NULL UNIQUE,
 physical_identity TEXT NOT NULL UNIQUE,
 resolved_path TEXT NOT NULL,
 data TEXT NOT NULL
);
CREATE INDEX projects_resolved_path ON projects(resolved_path);
CREATE TABLE drafts (
 sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),
 id TEXT NOT NULL UNIQUE,
 project_id TEXT NOT NULL REFERENCES projects(id),
 revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1)
);
CREATE TABLE draft_versions (
 draft_id TEXT NOT NULL REFERENCES drafts(id),
 revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),
 data TEXT NOT NULL,
 PRIMARY KEY(draft_id,revision)
);
CREATE TABLE receipts (
 request_id TEXT PRIMARY KEY,
 operation TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 status INTEGER NOT NULL,
 envelope TEXT NOT NULL
);
PRAGMA application_id=1129202226;
PRAGMA user_version=1;


CREATE TABLE tasks(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,project_id TEXT NOT NULL REFERENCES projects(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),state_revision INTEGER NOT NULL CHECK(typeof(state_revision)='integer' AND state_revision>=1),current_version INTEGER NOT NULL CHECK(typeof(current_version)='integer' AND current_version>=1),data TEXT NOT NULL);
CREATE TABLE task_versions(task_id TEXT NOT NULL REFERENCES tasks(id),version INTEGER NOT NULL CHECK(typeof(version)='integer' AND version>=1),data TEXT NOT NULL,PRIMARY KEY(task_id,version));
CREATE TABLE attempts(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,task_id TEXT NOT NULL REFERENCES tasks(id),task_version INTEGER NOT NULL,revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),phase TEXT NOT NULL CHECK(phase IN ('queued','preparing','running','cancelling','settled')),data TEXT NOT NULL,FOREIGN KEY(task_id,task_version) REFERENCES task_versions(task_id,version));
CREATE INDEX attempts_task ON attempts(task_id,sequence);
CREATE TABLE attempt_terminals(attempt_id TEXT PRIMARY KEY REFERENCES attempts(id),data TEXT NOT NULL);
CREATE TABLE attempt_events(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),attempt_id TEXT NOT NULL REFERENCES attempts(id),data TEXT NOT NULL);
CREATE INDEX events_attempt ON attempt_events(attempt_id,sequence);
CREATE TABLE artifacts(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,attempt_id TEXT NOT NULL REFERENCES attempts(id),private_ref TEXT NOT NULL,physical_identity TEXT NOT NULL,byte_length INTEGER NOT NULL CHECK(typeof(byte_length)='integer' AND byte_length>=0),data TEXT NOT NULL);
CREATE INDEX artifacts_attempt ON artifacts(attempt_id,sequence);
CREATE TABLE source_snapshots(id TEXT PRIMARY KEY,attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id),manifest_artifact_id TEXT NOT NULL REFERENCES artifacts(id),index_artifact_id TEXT NOT NULL REFERENCES artifacts(id),data TEXT NOT NULL);
CREATE TABLE candidates(id TEXT PRIMARY KEY,attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id),source_snapshot_id TEXT NOT NULL REFERENCES source_snapshots(id),manifest_artifact_id TEXT NOT NULL REFERENCES artifacts(id),delta_artifact_id TEXT NOT NULL REFERENCES artifacts(id),workspace_ref TEXT NOT NULL,data TEXT NOT NULL);
CREATE TABLE response_acceptances(id TEXT PRIMARY KEY,task_id TEXT NOT NULL REFERENCES tasks(id),task_version INTEGER NOT NULL,attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id),response_artifact_id TEXT NOT NULL REFERENCES artifacts(id),data TEXT NOT NULL,FOREIGN KEY(task_id,task_version) REFERENCES task_versions(task_id,version));
CREATE TABLE execution_incidents(id TEXT PRIMARY KEY,attempt_id TEXT REFERENCES attempts(id),data TEXT NOT NULL);
CREATE TABLE execution_control(singleton INTEGER PRIMARY KEY CHECK(singleton=1),state TEXT NOT NULL CHECK(state IN ('open','closing','fenced')),owner_instance_id TEXT NOT NULL,incident_id TEXT REFERENCES execution_incidents(id));
INSERT INTO execution_control VALUES(1,'open','',NULL);
CREATE TABLE capacity_claims(resource_id TEXT PRIMARY KEY CHECK(resource_id='managed-native'),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id));
PRAGMA user_version=3;


CREATE TABLE routing_policy(singleton INTEGER PRIMARY KEY CHECK(singleton=1),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),data TEXT NOT NULL);
CREATE TABLE routing_contexts(id TEXT PRIMARY KEY CHECK(id IN ('codex-native','claude-native')),harness_id TEXT UNIQUE NOT NULL,revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),data TEXT NOT NULL);
CREATE TABLE routing_profiles(id TEXT PRIMARY KEY CHECK(id IN ('codex-native-default','codex-luna-economy','codex-sol-general','claude-native-default','claude-haiku-economy','claude-sonnet-general')),context_id TEXT NOT NULL REFERENCES routing_contexts(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),data TEXT NOT NULL);
CREATE TABLE routing_accounts(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),data TEXT NOT NULL);
CREATE TABLE routing_account_links(context_id TEXT PRIMARY KEY REFERENCES routing_contexts(id),account_id TEXT REFERENCES routing_accounts(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),data TEXT NOT NULL);
CREATE TABLE routing_capacity_state(scope_key TEXT PRIMARY KEY,context_id TEXT UNIQUE REFERENCES routing_contexts(id),account_id TEXT UNIQUE REFERENCES routing_accounts(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),data TEXT NOT NULL,CHECK((context_id IS NOT NULL AND account_id IS NULL AND scope_key='context:'||context_id) OR (account_id IS NOT NULL AND context_id IS NULL AND scope_key='account:'||account_id)));
CREATE TABLE routing_capacity_reports(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,scope_key TEXT NOT NULL REFERENCES routing_capacity_state(scope_key),dimension TEXT NOT NULL CHECK(dimension IN ('occupancy','allowance','service')),data TEXT NOT NULL);
CREATE INDEX routing_reports_scope ON routing_capacity_reports(scope_key,sequence);
CREATE TABLE routing_report_bindings(report_id TEXT NOT NULL REFERENCES routing_capacity_reports(id),scope_key TEXT NOT NULL REFERENCES routing_capacity_state(scope_key),PRIMARY KEY(report_id,scope_key));
CREATE TABLE task_routing_versions(id TEXT UNIQUE NOT NULL,task_id TEXT NOT NULL,task_version INTEGER NOT NULL CHECK(typeof(task_version)='integer' AND task_version>=1),data TEXT NOT NULL,PRIMARY KEY(task_id,task_version),FOREIGN KEY(task_id,task_version) REFERENCES task_versions(task_id,version));
CREATE TABLE task_budgets(task_id TEXT PRIMARY KEY REFERENCES tasks(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),granted INTEGER NOT NULL CHECK(typeof(granted)='integer' AND granted>=0),consumed INTEGER NOT NULL CHECK(typeof(consumed)='integer' AND consumed>=0 AND consumed<=granted),automatic INTEGER NOT NULL CHECK(typeof(automatic)='integer' AND automatic>=0 AND automatic<=consumed),historical INTEGER NOT NULL CHECK(typeof(historical)='integer' AND historical>=0 AND historical<=consumed),window_number INTEGER NOT NULL CHECK(typeof(window_number)='integer' AND window_number>=1),first_admitted_at TEXT,deadline_at TEXT,data TEXT NOT NULL,CHECK((first_admitted_at IS NULL AND deadline_at IS NULL) OR (first_admitted_at IS NOT NULL AND deadline_at IS NOT NULL)));
CREATE TABLE routing_decisions(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,task_id TEXT NOT NULL,task_version INTEGER NOT NULL,attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id),previous_attempt_id TEXT REFERENCES attempts(id),kind TEXT NOT NULL CHECK(kind IN ('manual','automatic')),data TEXT NOT NULL,FOREIGN KEY(task_id,task_version) REFERENCES task_versions(task_id,version));
CREATE INDEX routing_decisions_task ON routing_decisions(task_id,sequence);
CREATE UNIQUE INDEX routing_auto_predecessor ON routing_decisions(previous_attempt_id) WHERE kind='automatic';
CREATE TABLE routing_continuation_proofs(attempt_id TEXT PRIMARY KEY REFERENCES attempts(id),owner_instance_id TEXT NOT NULL,category TEXT NOT NULL CHECK(category IN ('pre_launch_ineligible','terminal_transient')),data TEXT NOT NULL);
CREATE TABLE budget_amendments(id TEXT PRIMARY KEY,task_id TEXT NOT NULL REFERENCES tasks(id),budget_revision INTEGER NOT NULL CHECK(typeof(budget_revision)='integer' AND budget_revision>=1),data TEXT NOT NULL,UNIQUE(task_id,budget_revision));
CREATE TABLE budget_ledger(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,task_id TEXT NOT NULL REFERENCES tasks(id),budget_revision INTEGER NOT NULL CHECK(typeof(budget_revision)='integer' AND budget_revision>=1),kind TEXT NOT NULL CHECK(kind IN ('legacy_baseline','grant','adoption','admission','window_open','window_expired','launch_intent','launch_started')),attempt_id TEXT REFERENCES attempts(id),data TEXT NOT NULL);
CREATE INDEX budget_ledger_task ON budget_ledger(task_id,sequence);
CREATE UNIQUE INDEX budget_admission_attempt ON budget_ledger(attempt_id) WHERE kind='admission';
CREATE UNIQUE INDEX budget_launch_intent_attempt ON budget_ledger(attempt_id) WHERE kind='launch_intent';
CREATE UNIQUE INDEX budget_launch_started_attempt ON budget_ledger(attempt_id) WHERE kind='launch_started';

PRAGMA user_version=4;

-- Accepted005 workflow schema addition
CREATE TABLE delivery_preferences(singleton INTEGER PRIMARY KEY CHECK(singleton=1),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),data TEXT NOT NULL);
CREATE TABLE workflows(id TEXT PRIMARY KEY,task_id TEXT NOT NULL REFERENCES tasks(id),task_version INTEGER NOT NULL CHECK(typeof(task_version)='integer' AND task_version>=1),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),state_revision INTEGER NOT NULL CHECK(typeof(state_revision)='integer' AND state_revision>=1),state TEXT NOT NULL CHECK(state IN ('queued','running','blocked','cancelling','cancelled','failed','interrupted','completed')),authority_data TEXT NOT NULL,data TEXT NOT NULL,UNIQUE(task_id,task_version),FOREIGN KEY(task_id,task_version) REFERENCES task_versions(task_id,version));
CREATE TABLE workflow_steps(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,workflow_id TEXT NOT NULL REFERENCES workflows(id),ordinal INTEGER NOT NULL CHECK(typeof(ordinal)='integer' AND ordinal>=1),execution_ordinal INTEGER NOT NULL CHECK(typeof(execution_ordinal)='integer' AND execution_ordinal>=1),logical_key TEXT NOT NULL,role TEXT NOT NULL CHECK(role IN ('author','review','correction','check','local_delivery','external_delivery')),executor_kind TEXT NOT NULL CHECK(executor_kind IN ('native','local_check','local_application','local_external')),cycle INTEGER NOT NULL CHECK(cycle IN (0,1)),status TEXT NOT NULL CHECK(status IN ('pending','ready','active','succeeded','failed','cancelled','blocked','interrupted','not_required')),attempt_id TEXT UNIQUE REFERENCES attempts(id),input_candidate_id TEXT REFERENCES candidates(id),input_response_artifact_id TEXT REFERENCES artifacts(id),predecessor_step_id TEXT REFERENCES workflow_steps(id),replacement_of_step_id TEXT UNIQUE REFERENCES workflow_steps(id),consumption_key TEXT UNIQUE,data TEXT NOT NULL,UNIQUE(workflow_id,ordinal),UNIQUE(workflow_id,logical_key,execution_ordinal),CHECK(input_candidate_id IS NULL OR input_response_artifact_id IS NULL));
CREATE INDEX workflow_steps_order ON workflow_steps(workflow_id,sequence);
CREATE TABLE workflow_reviews(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,workflow_id TEXT NOT NULL REFERENCES workflows(id),step_id TEXT UNIQUE NOT NULL REFERENCES workflow_steps(id),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id),candidate_id TEXT REFERENCES candidates(id),response_artifact_id TEXT REFERENCES artifacts(id),result_artifact_id TEXT NOT NULL REFERENCES artifacts(id),data TEXT NOT NULL,CHECK((candidate_id IS NOT NULL AND response_artifact_id IS NULL) OR (candidate_id IS NULL AND response_artifact_id IS NOT NULL)));
CREATE INDEX workflow_reviews_order ON workflow_reviews(workflow_id,sequence);
CREATE TABLE workflow_checks(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,workflow_id TEXT NOT NULL REFERENCES workflows(id),step_id TEXT UNIQUE NOT NULL REFERENCES workflow_steps(id),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id),definition_id TEXT NOT NULL,candidate_id TEXT REFERENCES candidates(id),response_artifact_id TEXT REFERENCES artifacts(id),data TEXT NOT NULL,CHECK((candidate_id IS NOT NULL AND response_artifact_id IS NULL) OR (candidate_id IS NULL AND response_artifact_id IS NOT NULL)));
CREATE INDEX workflow_checks_order ON workflow_checks(workflow_id,sequence);
CREATE TABLE workflow_resolutions(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,workflow_id TEXT NOT NULL REFERENCES workflows(id),finding_id TEXT NOT NULL,origin_review_id TEXT REFERENCES workflow_reviews(id),origin_check_id TEXT REFERENCES workflow_checks(id),corrected_candidate_id TEXT REFERENCES candidates(id),corrected_response_artifact_id TEXT REFERENCES artifacts(id),corrected_subject_sha256 TEXT NOT NULL,data TEXT NOT NULL,UNIQUE(workflow_id,finding_id,corrected_subject_sha256),CHECK((origin_review_id IS NOT NULL AND origin_check_id IS NULL) OR (origin_review_id IS NULL AND origin_check_id IS NOT NULL)),CHECK((corrected_candidate_id IS NOT NULL AND corrected_response_artifact_id IS NULL) OR (corrected_candidate_id IS NULL AND corrected_response_artifact_id IS NOT NULL)));
CREATE INDEX workflow_resolutions_order ON workflow_resolutions(workflow_id,sequence);
CREATE TABLE workflow_acceptances(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,workflow_id TEXT NOT NULL REFERENCES workflows(id),task_id TEXT NOT NULL REFERENCES tasks(id),task_version INTEGER NOT NULL CHECK(typeof(task_version)='integer' AND task_version>=1),candidate_id TEXT NOT NULL REFERENCES candidates(id),data TEXT NOT NULL,UNIQUE(workflow_id,candidate_id),FOREIGN KEY(task_id,task_version) REFERENCES task_versions(task_id,version));
CREATE INDEX workflow_acceptances_order ON workflow_acceptances(workflow_id,sequence);
CREATE TABLE delivery_operations(sequence INTEGER PRIMARY KEY CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT UNIQUE NOT NULL,workflow_id TEXT NOT NULL REFERENCES workflows(id),step_id TEXT UNIQUE NOT NULL REFERENCES workflow_steps(id),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id),candidate_id TEXT NOT NULL REFERENCES candidates(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),status TEXT NOT NULL CHECK(status IN ('preparing','ready','applying','applied','conflict','partial','cancelled','interrupted','uncertain')),active_effect_id TEXT,active_action TEXT,data TEXT NOT NULL,CHECK((active_effect_id IS NULL AND active_action IS NULL) OR (active_effect_id IS NOT NULL AND active_action IS NOT NULL)));
CREATE INDEX delivery_operations_order ON delivery_operations(workflow_id,sequence);
CREATE TABLE delivery_effects(operation_id TEXT NOT NULL REFERENCES delivery_operations(id),sequence INTEGER NOT NULL CHECK(typeof(sequence)='integer' AND sequence>=1),effect_id TEXT NOT NULL,kind TEXT NOT NULL CHECK(kind IN ('add','replace','delete')),action TEXT NOT NULL CHECK(action IN ('quarantine_before','publish_after')),state TEXT NOT NULL CHECK(state IN ('intent','applied','conflict','failed','uncertain')),data TEXT NOT NULL,PRIMARY KEY(operation_id,sequence));
CREATE INDEX delivery_effects_identity ON delivery_effects(operation_id,effect_id,action,sequence);
CREATE UNIQUE INDEX delivery_effects_intent ON delivery_effects(operation_id,effect_id,action) WHERE state='intent';
PRAGMA user_version=5;
