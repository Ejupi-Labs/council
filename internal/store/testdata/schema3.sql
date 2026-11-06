-- Frozen accepted feature003 ce8aaa7. Metadata-only local fixture; never user state.

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
