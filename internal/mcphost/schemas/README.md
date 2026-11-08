# Feature 008 schema review inventory

Files under `v1/` are keyed by exact catalog tool name. Their presence alone
does not approve a tool. `Lookup`, `Names` and `Digest` expose only schemas
whose compact JSON token digest matches the independently reviewed inventory
in `approved.go`. `DraftLookup`, `DraftNames` and `DraftDigest` are for review
and tests, never advertisement or dispatch. A changed schema requires renewed
review and a deliberate digest update. The dispatcher must apply the owner
validation layers below before attachment or any effect.

The schema is one validation layer, not a substitute for the corresponding
owner validator. JSON Schema cannot capture all owner semantics. The host must
retain the original bytes, strict duplicate/unknown/null and presence checks,
operation-specific body byte limits and exact captured authority. In particular:

- UTF-8 byte limits are not equivalent to JSON Schema string-length limits.
- Opaque cursors require the existing owner authentication, scope and bounds.
- Graph node-authority IDs must be unique even when other fields differ, and
  rebind IDs must belong to that exact authority vector.
- A graph declaration requires unique node keys and dependency endpoint pairs,
  existing distinct endpoints, acyclicity, and a valid reachable final node.
  These relations and the encoded declaration size require owner validation.
- Action-specific rules, nested routing/workflow presence semantics, authority
  revisions, owner state and cross-record relationships require the current
  owner validators. Schema-valid input is not evidence of an accepted operation.

The shared execution/routing/workflow fragments in graph start and guidance
were reviewed against the domain DTOs and presence rules. Parity tests cover
representative valid and invalid inputs for every mutation; they do not
establish every possible combination or state-dependent outcome. Authenticated
real-owner and MCP-dispatch parity are separate, still-required feature gates.
