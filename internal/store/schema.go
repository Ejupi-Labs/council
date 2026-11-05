package store

const schemaVersion = 1
const applicationID = 1129202226
const schema = `
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
`
