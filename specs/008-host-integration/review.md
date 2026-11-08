# 008 Design Review

Independent pre-implementation review identified and corrected these design issues:

- A known older owner must retain its separately verified maintenance Stop without allowing new execution or replacing its lock; owner capability checks are per operation and tool discovery is available before attachment.
- Existing owner receipts intentionally normalize certain omitted/default inputs. Host retries preserve those documented semantic equivalences while effect-changing input or captured authority conflicts; an adapter-local raw-byte cache cannot redefine cross-interface behavior.
- The SDK alone does not enforce the initialized notification before dispatch. An ordered per-session validating gate and raw stdio tests are required.
- The original catalog duplicated agent list/refresh; one fresh observational list tool now represents the existing route. A mechanical audit must reject other missing, extra or duplicate mappings.
- Owner DTO references alone are not complete advertised JSON schemas. A dedicated versioned schema generation and independent review gate precedes all tool dispatch implementation.

Review confirms the documented boundaries are internally consistent; it does not claim that the SDK, tool catalog, protocol transport, owner client, tests or live provider behavior have been implemented or qualified. Further independent source review and exact-source acceptance remain required.
