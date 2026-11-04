# Collaboration reference notes

Reviewed official documentation on 4 November 2026. These are design references; Council does not import their runtime frameworks or copy their implementation code.

The [GitHub comparison](oss-comparison.md) records six widely starred projects, observed popularity, applied pause/resume patterns and ideas considered for later work.

[CrewAI tasks](https://docs.crewai.com/en/concepts/tasks) explicitly support passing other tasks' outputs as context. Council's dependency handoff keeps the complete recorded room evidence and attaches prerequisite workflow/artifact references. A retained worktree is separate from the follower's workspace, so its existence never implies that predecessor changes were integrated.

[LangGraph persistence](https://docs.langchain.com/oss/javascript/langgraph/persistence) and [interrupts](https://docs.langchain.com/oss/javascript/langgraph/interrupts) document saved execution state and suspended work awaiting external input. Council keeps its existing SQLite room state, server-side waits and approval revision checks. Completed writes are retained for review and are never automatically replayed or merged.

This pass adds a direct read-only workflow evidence view and exact JSON snapshot export so the project manager can review the records behind task previews. It also records configured model/effort settings at admission, separating those known choices from unrecorded native defaults. Inspecting these records and planning model selection require no classifier or worker model call.
