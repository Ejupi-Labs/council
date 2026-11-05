# Reference preservation

The legacy functional baseline is commit27822baa3aee663366a870bde7fb1190649d3418.
The replacement is independently authored. Its implementation uses cmd/, internal/ and ui/.

Automatic review rejected the planned bulk removal command with the generic reason “blocked by
policy” before it executed. The safer replacement plan retains explicitly tracked legacy files
under reference/legacy/ using reversible Git moves. No file-content deletion is needed. All ignored
state, credentials, original release archives and project Git metadata remain in place.

Relocated roots: src, dist, tests, schemas, scripts, .github, .claude-plugin, .codex-plugin,
commands, skills, examples; obsolete root manifests (.mcp.json, mcp.json, plugin.json,
council.config.json, tsconfig.json, CHANGELOG.md); docs outside docs/refactor.
Root package manifests and README are retained for replacement, not relocated.

reference/legacy is not an application dependency, build input or release-package member.
Its historical documentation describes the baseline only. Existing source licenses remain.
