import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { WorkflowEngine } from './engine.js';
import { CouncilStore } from './store.js';
import { createRunner } from './runner.js';
import { parseWorkflow } from './config.js';
import type { AgentRunner, CouncilConfig, RunRecord, WorkflowInput } from './types.js';

export class CouncilService {
  readonly store: CouncilStore;
  readonly runner: AgentRunner;
  readonly engine: WorkflowEngine;
  private readonly jobs = new Map<string, { controller: AbortController; done: Promise<RunRecord> }>();
  private closing = false;

  constructor(readonly config: CouncilConfig, runner?: AgentRunner) {
    this.store = new CouncilStore(config.stateDir);
    this.runner = runner ?? createRunner(config);
    this.engine = new WorkflowEngine(config, this.runner, this.store);
  }

  start(value: unknown): RunRecord {
    if (this.closing) throw new Error('Council is shutting down.');
    if (Number(process.env.COUNCIL_DEPTH ?? 0) > 0) throw new Error('Nested Council delegation is disabled. Return your result to the parent agent.');
    if (this.jobs.size >= 20) throw new Error('This server already has 20 active runs. Wait for a result before starting more.');
    const input = parseWorkflow(value);
    this.engine.plan(input);
    const now = new Date().toISOString();
    const run: RunRecord = { id: randomUUID(), input, status: 'queued', createdAt: now, updatedAt: now, steps: [] };
    this.store.saveRun(run);
    this.store.appendEvent(run.id, 'queued', { mode: input.mode });
    const controller = new AbortController();
    const done = Promise.resolve().then(() => this.engine.run(input, { id: run.id, signal: controller.signal }))
      .catch((error: unknown) => {
        const failed: RunRecord = { ...(this.store.getRun(run.id) ?? run), status: controller.signal.aborted ? 'cancelled' : 'failed', error: error instanceof Error ? error.message : 'Workflow failed.', updatedAt: new Date().toISOString() };
        this.store.saveRun(failed);
        return failed;
      }).finally(() => { this.jobs.delete(run.id); });
    this.jobs.set(run.id, { controller, done });
    return run;
  }

  async run(input: WorkflowInput): Promise<RunRecord> {
    const run = this.start(input);
    return await this.jobs.get(run.id)!.done;
  }

  async wait(id: string, timeoutMs = 1000): Promise<RunRecord> {
    const until = Date.now() + Math.min(Math.max(timeoutMs, 0), 30_000);
    for (;;) {
      const run = this.store.getRun(id);
      if (!run) throw new Error(`Run not found: ${id}`);
      if (!['queued', 'running'].includes(run.status) || Date.now() >= until) return run;
      await delay(Math.min(100, until - Date.now()));
    }
  }

  cancel(id: string): RunRecord {
    const run = this.store.getRun(id);
    if (!run) throw new Error(`Run not found: ${id}`);
    if (!['queued', 'running'].includes(run.status)) return run;
    this.store.appendEvent(id, 'cancel_requested', {});
    this.jobs.get(id)?.controller.abort();
    return this.store.getRun(id)!;
  }

  async close(): Promise<void> {
    if (this.closing) return;
    this.closing = true;
    for (const job of this.jobs.values()) job.controller.abort();
    await Promise.allSettled([...this.jobs.values()].map(job => job.done));
    this.store.close();
  }
}
