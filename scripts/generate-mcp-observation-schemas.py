#!/usr/bin/env python3
"""Emit the fixed, closed observation schemas for MCP feature 008.

The owner remains authoritative for bounded decimal values, UTF-8 byte limits,
opaque cursors, and state-dependent validation, which JSON Schema cannot express
exactly. This generator does not generate or approve mutation DTO schemas.
"""

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "internal/mcphost/schemas/v1"
DRAFT = "https://json-schema.org/draft/2020-12/schema"


def positive_int64_pattern():
    maximum = "9223372036854775807"
    choices = ["[1-9][0-9]{0,17}"]
    for index, digit in enumerate(maximum):
        low = 1 if index == 0 else 0
        high = int(digit) - 1
        if high < low:
            continue
        selected = str(low) if high == low else "[" + str(low) + "-" + str(high) + "]"
        remaining = len(maximum) - index - 1
        choices.append(maximum[:index] + selected + ("[0-9]{" + str(remaining) + "}" if remaining else ""))
    choices.append(maximum)
    return "(" + "|".join(choices) + ")"


INT64 = positive_int64_pattern()
ID = {"type": "string", "minLength": 32, "maxLength": 32, "pattern": "^[0-9a-f]{32}$"}
REV = {"type": "string", "pattern": "^" + INT64 + "$", "not": {"pattern": "[^0-9]"}}
QUERY = {"type": "string", "minLength": 1}
CONTEXT = {"type": "string", "enum": ["codex-native", "claude-native"]}
COUNT = {"type": "string", "pattern": "^(0|" + INT64 + ")$", "not": {"pattern": "[^0-9]"}}
PAGE_LIMIT = {"type": "string", "pattern": "^([1-9]|[1-9][0-9]|100)$",
              "not": {"pattern": "[^0-9]"}}
BYTE_LIMIT = {"type": "string", "pattern": "^([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-6])$",
              "not": {"pattern": "[^0-9]"}}
WAIT = {"type": "string", "pattern": "^(0|[1-9][0-9]{0,3}|[12][0-9]{4}|30000)$",
        "not": {"pattern": "[^0-9]"}}
CURSOR = {"type": "string", "minLength": 1, "maxLength": 2048,
          "pattern": "^[A-Za-z0-9_-]+$", "not": {"pattern": "[^A-Za-z0-9_-]"},
          "description": "Opaque cursor from the same list and filter; its contents are validated by the owner."}


def schema(required=(), optional=(), exclusive=()):
    properties = {}
    for key in (*required, *optional):
        if key in {"id", "taskId", "projectId", "accountId", "workflowId"}:
            value = ID
        elif key == "contextId":
            value = CONTEXT
        elif key in {"version", "taskVersion", "revision"}:
            value = REV
        elif key in {"offset", "through", "afterRevision"}:
            value = COUNT
        elif key == "timeoutMs":
            value = WAIT
        elif key == "limit":
            value = PAGE_LIMIT
        elif key == "cursor":
            value = CURSOR
        else:
            value = QUERY
        properties[key] = value
    out = {"$schema": DRAFT, "type": "object", "properties": properties,
           "additionalProperties": False}
    if required:
        out["required"] = list(required)
    if exclusive:
        out["not"] = {"required": list(exclusive)}
    return out


def write(family, rows):
    path = ROOT / (family + ".json")
    path.write_text(json.dumps(rows, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def obj(properties, required=()):
    out = {"type": "object", "properties": properties, "additionalProperties": False}
    if required:
        out["required"] = list(required)
    return out


def mutation(body, required=()):
    out = schema(required)
    out["properties"]["input"] = body
    out.setdefault("required", []).append("input")
    return out


def enum(*values):
    return {"type": "string", "enum": list(values)}


STRING = {"type": "string"}
NONEMPTY = {"type": "string", "minLength": 1}
LABEL = {"type": "string", "minLength": 1, "maxLength": 128}
OPTIONAL_LABEL = {"type": "string", "maxLength": 128}
PRICE_DECIMAL = {"type": "string", "minLength": 1, "maxLength": 64,
                 "pattern": r"^(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$",
                 "not": {"pattern": "[^0-9eE+.-]"}}
WORKFLOW_KEY = {"type": "string", "minLength": 1, "maxLength": 64,
                "pattern": "^[a-z0-9_-]+$", "not": {"pattern": "[^a-z0-9_-]"}}
CHECK_KEY = {**WORKFLOW_KEY, "allOf": [{"not": {"pattern": "^(task-|required-|workflow-|delivery-|independent-review$|local-integration$|external-delivery$)"}}]}
TEXT = {"type": "string", "maxLength": 4194304}
NONEMPTY_TEXT = {"type": "string", "minLength": 1, "maxLength": 4194304}
BOOL = {"type": "boolean"}
TIMEOUT = {"type": "integer", "minimum": 1, "maximum": 86400}
SMALL_TIMEOUT = {"type": "integer", "minimum": 1, "maximum": 1800}
COUNT16 = {"type": "string", "pattern": "^([1-9]|1[0-6])$"}
SCENARIO = enum("graph-branch-a", "graph-branch-b", "graph-join", "graph-join-no-edit",
                "graph-fail", "graph-wait", "graph-response", "graph-document", "graph-diverge-a",
                "graph-diverge-b", "workflow-document", "workflow-defect", "response", "candidate",
                "partial_failure", "slow", "late_success", "owned_descendant", "cleanup_unknown",
                "trailing_observation", "malformed", "missing_result", "repeated_result",
                "trailing_error", "success_error", "model_usage", "empty_response", "empty_candidate")
PROFILE = enum("codex-native-default", "codex-luna-economy", "codex-sol-general",
               "claude-native-default", "claude-haiku-economy", "claude-sonnet-general")
BUDGET = obj({"admissionLimit": COUNT16, "windowSeconds": TIMEOUT})
CONTINUATION = obj({
    "maxAutomaticAdmissions": {"type": "string", "pattern": "^(0|[1-9]|1[0-5])$"},
    "categories": {"type": "array", "uniqueItems": True,
                   "items": enum("pre_launch_ineligible", "terminal_transient")},
    "allowReplacement": BOOL, "allowEscalation": BOOL,
})
ROUTING = obj({"suitability": enum("unknown", "routine", "general"),
               "targetProfileId": PROFILE, "expectedPolicyRevision": REV,
               "budget": BUDGET, "continuation": CONTINUATION, "useCurrentPolicy": BOOL})
ROUTING_START = {**ROUTING, "not": {"required": ["useCurrentPolicy"]}}
ROUTING_GUIDANCE = {**ROUTING, "allOf": [
    {"not": {"required": ["budget"]}},
    {"if": {"required": ["expectedPolicyRevision"]},
     "then": {"required": ["useCurrentPolicy"], "properties": {"useCurrentPolicy": {"const": True}}}},
]}
ROUTING_ADOPT = {**ROUTING, "not": {"anyOf": [{"required": ["budget"]}, {"required": ["useCurrentPolicy"]}]}}
PINS = obj({"harnessId": enum("codex", "claude-code", "deepseek-harness", "junie", "antigravity"),
            "model": {"type": "string", "minLength": 1, "maxLength": 256}, "accountId": ID, "paymentPath": enum("native", "paid_api")})
ACCEPTANCE = obj({"deliverable": enum("candidate", "response_only"), "conditionsText": TEXT,
                  "requiredEvidence": {"type": "array", "uniqueItems": True,
                                       "items": enum("check", "independent_review", "local_integration", "external_delivery")},
                  "allowCandidatePreparation": BOOL})
COMMAND = obj({"executable": {"type": "string", "minLength": 1, "maxLength": 4096}, "arguments": {"type": "array", "maxItems": 64, "items": {"type": "string", "maxLength": 4096}},
               "timeoutSeconds": SMALL_TIMEOUT}, ("executable", "arguments"))
ACTION = obj({"action": {"type": "string", "minLength": 1, "maxLength": 1024}, "destination": {"type": "string", "minLength": 1, "maxLength": 1024},
              "acceptanceBasis": enum("command_completion"), "command": COMMAND},
             ("action", "destination", "acceptanceBasis", "command"))
SCOPE = obj({"kind": enum("whole_candidate", "declared_subset"),
             "paths": {"type": "array", "maxItems": 256, "uniqueItems": True, "items": {"type": "string", "minLength": 1, "maxLength": 4096}}},
            ("kind", "paths"))
SCOPE["oneOf"] = [
    {"properties": {"kind": {"const": "whole_candidate"}, "paths": {"maxItems": 0}}},
    {"properties": {"kind": {"const": "declared_subset"}, "paths": {"minItems": 1}}},
]
CHECK = obj({"key": CHECK_KEY, "executable": {"type": "string", "minLength": 1, "maxLength": 4096},
             "arguments": {"type": "array", "maxItems": 64, "items": {"type": "string", "maxLength": 4096}},
             "relativeWorkingDirectory": {"type": "string", "minLength": 1, "maxLength": 4096}, "scope": SCOPE,
             "coveredConditionKeys": {"type": "array", "maxItems": 16, "uniqueItems": True, "items": WORKFLOW_KEY},
             "timeoutSeconds": SMALL_TIMEOUT,
             "successExitCodes": {"type": "array", "minItems": 1, "maxItems": 16, "uniqueItems": True,
                                  "items": {"type": "integer", "minimum": 0, "maximum": 255}},
             "required": BOOL}, ("key", "executable", "arguments"))
WORKFLOW = obj({"mode": enum("auto", "author_only"),
                "delivery": obj({"localMode": enum("review_only", "apply_verified_local"), "externalAction": ACTION}),
                "checks": {"type": "array", "maxItems": 16, "items": CHECK},
                "review": enum("auto", "required"),
                "maxCorrectionCycles": {"type": "integer", "minimum": 0, "maximum": 1}})
WORKFLOW["allOf"] = [{"if": {"properties": {"mode": {"const": "author_only"}}, "required": ["mode"]},
                       "then": {"not": {"anyOf": [{"required": [name]} for name in
                                                   ("delivery", "checks", "review", "maxCorrectionCycles")]}}}]


page = ("limit", "cursor")
execution = {
    "council_tasks_list": schema(optional=("projectId", *page)),
    "council_tasks_get": schema(("taskId",)),
    "council_tasks_version_get": schema(("taskId", "version")),
    "council_tasks_attempts_list": schema(("taskId",), page),
    "council_attempts_get": schema(("id",)),
    "council_attempts_wait": schema(("id",), ("afterRevision", "timeoutMs")),
    "council_attempts_events": schema(("id",), page),
    "council_attempts_artifacts": schema(("id",), page),
    "council_artifacts_get": schema(("id",)),
    "council_artifacts_bytes": schema(("id",), ("offset", "through", "limit")),
}
execution["council_artifacts_bytes"]["properties"]["limit"] = BYTE_LIMIT
common = {"workflow": WORKFLOW, "routing": ROUTING, "pins": PINS, "acceptance": ACCEPTANCE,
          "policyOverride": enum("supervised"), "timeoutSeconds": TIMEOUT}
start = obj({**common, "routing": ROUTING_START, "requestId": ID, "projectId": ID, "originalText": NONEMPTY_TEXT,
             "sourceDraftRef": obj({"draftId": ID, "revision": REV}, ("draftId", "revision")),
             "expectedPolicyRevision": REV, "executionMode": enum("native", "fake"),
             "fakeScenario": SCENARIO}, ("requestId", "projectId", "originalText"))
start["not"] = {"required": ["sourceDraftRef", "expectedPolicyRevision"]}
guidance = obj({**common, "routing": ROUTING_GUIDANCE, "requestId": ID, "expectedRevision": REV, "guidanceText": TEXT,
                "useCurrentPolicy": BOOL, "clearPolicyOverride": BOOL},
               ("requestId", "expectedRevision", "guidanceText"))
attempt = obj({"requestId": ID, "expectedRevision": REV, "taskVersion": REV,
               "executionMode": enum("native", "fake"), "fakeScenario": SCENARIO},
              ("requestId", "expectedRevision", "taskVersion"))
for body in (start, attempt):
    body["allOf"] = [{"if": {"required": ["fakeScenario"]},
                      "then": {"properties": {"executionMode": {"const": "fake"}},
                               "required": ["executionMode"]}}]
guidance["allOf"] = [{"if": {"properties": {"clearPolicyOverride": {"const": True}},
                              "required": ["clearPolicyOverride"]},
                       "then": {"not": {"required": ["policyOverride"]}}}]
response = obj({"requestId": ID, "expectedRevision": REV, "taskVersion": REV, "attemptId": ID,
                "responseArtifactId": ID, "responseSha256": {"type": "string", "pattern": "^[0-9a-f]{64}$"},
                "confirmation": enum("reviewed_response_and_conditions")},
               ("requestId", "expectedRevision", "taskVersion", "attemptId", "responseArtifactId", "responseSha256", "confirmation"))
execution.update({
    "council_tasks_start": mutation(start),
    "council_tasks_guidance": mutation(guidance, ("taskId",)),
    "council_tasks_attempts_start": mutation(attempt, ("taskId",)),
    "council_tasks_response_accept": mutation(response, ("taskId",)),
    "council_attempts_stop": mutation(obj({"requestId": ID}, ("requestId",)), ("id",)),
})

routing = {
    "council_routing_profiles_list": schema(optional=("contextId",)),
    "council_routing_accounts_list": schema(optional=page),
    "council_routing_capacity_get": schema(optional=("contextId", "accountId")),
    "council_routing_reports_list": schema(optional=("contextId", "accountId", *page)),
    "council_routing_reports_get": schema(("id",)),
    "council_tasks_routing_get": schema(("taskId",)),
    "council_tasks_routing_decisions": schema(("taskId",), page),
    "council_tasks_budget_get": schema(("taskId",)),
    "council_tasks_budget_ledger": schema(("taskId",), page),
    "council_tasks_usage": schema(("taskId",), page),
}
for name in ("council_routing_capacity_get", "council_routing_reports_list"):
    routing[name]["oneOf"] = [{"required": ["contextId"]}, {"required": ["accountId"]}]
identity = {"requestId": ID, "expectedRevision": REV}
policy = obj({**identity, "economyOrder": {"type": "array", "maxItems": 6, "uniqueItems": True, "items": PROFILE},
              "unknownAvailability": enum("allow", "require_positive"), "defaultBudget": BUDGET,
              "continuation": CONTINUATION}, identity)
price = obj({"currency": {"type": "string", "minLength": 3, "maxLength": 3, "pattern": "^[A-Z]{3}$"},
             "unit": enum("per_million_tokens"), "basis": enum("api_list", "declared_marginal"),
             "inputPerMillion": PRICE_DECIMAL, "outputPerMillion": PRICE_DECIMAL, "sourceLabel": LABEL,
             "sourceUrl": {"type": "string", "minLength": 1, "maxLength": 1024}, "observedAt": {"type": "string", "minLength": 1, "maxLength": 64}, "validUntil": {"type": "string", "minLength": 1, "maxLength": 64}},
            ("currency", "sourceLabel", "observedAt"))
context = obj({**identity, "label": LABEL, "enabled": BOOL}, identity)
profile = obj({**identity, "enabled": BOOL, "referencePrice": price, "clearReferencePrice": {"const": True}}, identity)
profile["not"] = {"required": ["referencePrice", "clearReferencePrice"]}
account_create = obj({"requestId": ID, "label": LABEL, "providerLabel": OPTIONAL_LABEL,
                      "entitlement": enum("unknown", "subscription", "local")}, ("requestId", "label"))
account_update = obj({**identity, "label": LABEL, "providerLabel": OPTIONAL_LABEL,
                      "entitlement": enum("unknown", "subscription", "local")}, identity)
account_link = obj({**identity, "declaredAccountId": ID, "unlink": {"const": True}}, identity)
account_link["oneOf"] = [{"required": ["declaredAccountId"]}, {"required": ["unlink"]}]
subject = obj({"kind": enum("context", "account"), "id": NONEMPTY}, ("kind", "id"))
subject["oneOf"] = [
    {"properties": {"kind": {"const": "context"}, "id": CONTEXT}},
    {"properties": {"kind": {"const": "account"}, "id": ID}},
]
report = obj({**identity, "subject": subject, "dimension": enum("occupancy", "allowance", "service"),
              "state": NONEMPTY, "observedAt": {"type": "string", "minLength": 1, "maxLength": 64}, "validForSeconds": TIMEOUT,
              "retryAfter": {"type": "string", "minLength": 1, "maxLength": 64}, "note": {"type": "string", "maxLength": 512}, "correctionOf": ID,
              "correctionAction": enum("recover", "withdraw")},
             ("requestId", "expectedRevision", "subject", "dimension", "state"))
report["oneOf"] = [
    {"properties": {"dimension": {"const": "occupancy"}, "state": enum("busy", "available", "unknown")}},
    {"properties": {"dimension": {"const": "allowance"}, "state": enum("available", "exhausted", "unknown")}},
    {"properties": {"dimension": {"const": "service"}, "state": enum("temporary_throttle", "temporary_overload", "authentication_denied", "model_denied", "permission_denied", "unknown")}},
]
report["allOf"] = [
    {"if": {"required": ["correctionOf"]}, "then": {"required": ["correctionAction"]}},
    {"if": {"required": ["correctionAction"]}, "then": {"required": ["correctionOf"]}},
    {"if": {"properties": {"correctionAction": {"const": "withdraw"}}, "required": ["correctionAction"]},
     "then": {"properties": {"state": {"const": "unknown"}}}},
    {"if": {"required": ["retryAfter"]},
     "then": {"properties": {"dimension": {"const": "service"},
                             "state": enum("temporary_throttle", "temporary_overload")}}},
]
amendment = obj({**identity, "action": enum("grant", "adopt"), "expectedBudgetRevision": REV,
                 "additionalAdmissions": COUNT16, "newWindowSeconds": TIMEOUT, "routing": ROUTING_ADOPT},
                ("requestId", "expectedRevision", "action", "additionalAdmissions"))
amendment["oneOf"] = [
    {"properties": {"action": {"const": "grant"}}, "required": ["expectedBudgetRevision"],
     "not": {"required": ["routing"]}},
    {"properties": {"action": {"const": "adopt"}}, "required": ["newWindowSeconds"],
     "not": {"required": ["expectedBudgetRevision"]}},
]
routing.update({
    "council_routing_policy_update": mutation(policy),
    "council_routing_context_update": mutation(context, ("id",)),
    "council_routing_profile_update": mutation(profile, ("id",)),
    "council_routing_accounts_create": mutation(account_create),
    "council_routing_accounts_update": mutation(account_update, ("id",)),
    "council_routing_links_update": mutation(account_link, ("id",)),
    "council_routing_reports_create": mutation(report),
    "council_tasks_budget_amend": mutation(amendment, ("taskId",)),
})
routing["council_routing_context_update"]["properties"]["id"] = CONTEXT
routing["council_routing_links_update"]["properties"]["id"] = CONTEXT
routing["council_routing_profile_update"]["properties"]["id"] = PROFILE

workflow = {"council_candidates_changes": schema(("id",), page)}
for suffix in ("get", "steps", "reviews", "checks", "findings", "deliveries", "candidates"):
    optional = ("workflowId", "taskVersion")
    if suffix != "get":
        optional += page
    workflow["council_tasks_workflow_" + suffix] = schema(
        ("taskId",), optional, ("workflowId", "taskVersion"))
advance_fields = {**identity, "workflowRevision": REV, "taskVersion": REV}
workflow.update({
    "council_workflow_preferences_update": mutation(obj({**identity, "localMode": enum("review_only", "apply_verified_local")},
                                                         ("requestId", "expectedRevision", "localMode"))),
    "council_tasks_workflow_adopt": mutation(obj({**identity, "taskVersion": REV, "workflow": WORKFLOW},
                                                 ("requestId", "expectedRevision", "taskVersion", "workflow")), ("taskId",)),
    "council_tasks_workflow_advance": mutation(obj(advance_fields, advance_fields), ("taskId",)),
    "council_tasks_workflow_check": mutation(obj({**advance_fields, "checkId": ID},
                                                 (*advance_fields, "checkId")), ("taskId",)),
})
workflow["council_tasks_workflow_adopt"]["properties"]["input"]["properties"]["workflow"] = {
    **WORKFLOW, "not": {"properties": {"mode": {"const": "author_only"}}, "required": ["mode"]}}

write("execution", execution)
write("routing", routing)
write("workflow", workflow)
