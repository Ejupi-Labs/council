---
name: council-advisor
description: Obtain a focused Council advisory opinion across models or harnesses when a specific uncertainty warrants it. Keep advice read-only and the worker responsible for the final result.
---

# Focused advisor

Choose an advisor for the uncertainty that matters, using configured capabilities, model preferences and current quota observations. Unknown billing data is not evidence of unlimited allowance. Use `council_advise` for independent read-only advice, or configure a worker's `advisors` mapping when it should yield and resume in the same workspace.

The worker yields only `{"council_advisor_request":{"question":"...","context":"..."}}` when that protocol is available. Include the complete relevant evidence. Council releases the worker's call slot, asks the advisor read-only and resumes the worker. Advisor requests and worker resumptions consume the configured call budget. Avoid repeated consultation without a new concrete uncertainty.

<!-- council-agent-instructions -->
If your configured advisor protocol is available, ask one focused question with all relevant evidence when it can resolve an actual uncertainty. Respect the remaining request quota. Otherwise complete your assigned task without inventing delegation tools. As an advisor, give read-only evidence, uncertainty and actionable guidance; do not implement changes or request another advisor. As the worker, assess the advice and retain responsibility for implementation and verification.
<!-- /council-agent-instructions -->
