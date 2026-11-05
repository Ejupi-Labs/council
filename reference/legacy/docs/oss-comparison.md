# Open-source orchestration references

Reviewed on **4 November 2026**. Star counts are approximate GitHub snapshots, not ratings of correctness, security or fit. The projects vary from coding harnesses to general agent frameworks; Council remains a local, dependency-free coordinator for installed harnesses.

| Project | GitHub stars observed | Relevant area |
| --- | ---: | --- |
| [OpenHands](https://github.com/OpenHands/OpenHands) | 92.6k | Coding-agent conversations, pause/resume and execution observability |
| [Ruflo](https://github.com/ruvnet/ruflo) | 80.8k | Cross-harness orchestration, roles and persistent collaboration |
| [AutoGen](https://github.com/microsoft/autogen) | 61.7k | Stateful team conversations and explicit stopping conditions |
| [CrewAI](https://github.com/crewAIInc/crewAI) | 60.5k | Task dependencies, output contracts and guardrails |
| [LangGraph](https://github.com/langchain-ai/langgraph) | 44.4k | Durable workflow state and external resume |
| [oh-my-claudecode](https://github.com/Yeachan-Heo/oh-my-claudecode) | 40.6k | Coding-team roles, verification evidence and model routing |

AutoGen's current README directs new projects toward Microsoft Agent Framework and describes its transition away from active development. Its documented team-control patterns are useful references independently of adopting that runtime.

## Applied in Council

[OpenHands pause/resume](https://docs.openhands.dev/sdk/guides/convo-pause-and-resume), [AutoGen external stopping and team resumption](https://microsoft.github.io/autogen/stable/user-guide/agentchat-user-guide/tutorial/teams.html), and [LangGraph interrupts](https://docs.langchain.com/oss/javascript/langgraph/interrupts) make yielding control an explicit part of orchestration. Council applies this idea at its own workflow admission boundary: a pause request blocks further workflows, drains already admitted work, persists a discussion-round or execution checkpoint, and releases its room job. Resuming requires an explicit action. It never suspends a vendor process or automatically replays completed work.

Manual discussion and execution are separate admission phases. A room configured with `autoExecute: false` reserves its discussion bound before voting; execution later validates the complete remaining task/advisor bound. Automatic rooms retain the combined preflight. These calculations are deterministic and invoke no classifier, advisor or worker.

[CrewAI's explicit task context](https://docs.crewai.com/en/concepts/tasks) also informed Council's existing complete prerequisite handoffs. Its [task guardrails](https://docs.crewai.com/en/concepts/tasks#task-guardrails) and OMC's [verifier role](https://github.com/Yeachan-Heo/oh-my-claudecode/blob/0564555b63dfedd158669c6799d53743abd7d6bf/agents/verifier.md) suggest a future distinction between reported task success and independently verified acceptance criteria. That distinction is not automatically established by a completed provider call or a majority vote.

[OpenHands observability](https://docs.openhands.dev/sdk/guides/observability) informed Council's chronological event view alongside the full workflow inspector. The viewer reads local stored events on demand, with exact-type filtering, complete paged payloads and explicit newer-event checks. Its sequence cursor preserves every unread matching event. No telemetry exporter or external collector is required.

No reference implementation code was copied and no framework runtime or npm dependency was added. Context remains complete unless the caller explicitly chooses a budget; failure and cancellation retain their own outcomes.
