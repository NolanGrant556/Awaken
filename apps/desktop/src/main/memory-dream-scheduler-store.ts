import fs from "node:fs/promises";
import { dirname, join } from "node:path";

type SchedulerState = {
  completedRuns: number;
  memoryMutations: number;
  lastSuccessfulAt?: string;
  seenRunIds: string[];
  seenMutationIds: string[];
};

const emptyState = (): SchedulerState => ({ completedRuns: 0, memoryMutations: 0, seenRunIds: [], seenMutationIds: [] });

export class MemoryDreamSchedulerStore {
  private readonly path: string;
  private state = emptyState();
  private writeChain = Promise.resolve();

  constructor(configDir: string) {
    this.path = join(configDir, "memory-dream-scheduler.json");
  }

  async initialize(): Promise<void> {
    try {
      const parsed = JSON.parse(await fs.readFile(this.path, "utf8")) as Partial<SchedulerState>;
      this.state = {
        completedRuns: Math.max(0, Number(parsed.completedRuns) || 0),
        memoryMutations: Math.max(0, Number(parsed.memoryMutations) || 0),
        lastSuccessfulAt: typeof parsed.lastSuccessfulAt === "string" ? parsed.lastSuccessfulAt : undefined,
        seenRunIds: Array.isArray(parsed.seenRunIds) ? parsed.seenRunIds.filter((id): id is string => typeof id === "string").slice(-1000) : [],
        seenMutationIds: Array.isArray(parsed.seenMutationIds) ? parsed.seenMutationIds.filter((id): id is string => typeof id === "string").slice(-2000) : [],
      };
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    }
  }

  eligible(now = Date.now()): boolean {
    const last = this.state.lastSuccessfulAt ? Date.parse(this.state.lastSuccessfulAt) : 0;
    return (!last || now - last >= 24 * 60 * 60 * 1000) && (this.state.completedRuns >= 5 || this.state.memoryMutations >= 20);
  }

  recordCompletedRun(runId: string): void {
    if (!runId || this.state.seenRunIds.includes(runId)) return;
    this.state.seenRunIds = [...this.state.seenRunIds.slice(-999), runId];
    this.state.completedRuns++;
    this.persist();
  }

  recordMemoryMutation(recordId: string): void {
    if (!recordId || this.state.seenMutationIds.includes(recordId)) return;
    this.state.seenMutationIds = [...this.state.seenMutationIds.slice(-1999), recordId];
    this.state.memoryMutations++;
    this.persist();
  }

  recordSuccessfulDream(at = new Date()): void {
    this.state.completedRuns = 0;
    this.state.memoryMutations = 0;
    this.state.lastSuccessfulAt = at.toISOString();
    this.persist();
  }

  snapshot(): Readonly<SchedulerState> {
    return { ...this.state, seenRunIds: [...this.state.seenRunIds], seenMutationIds: [...this.state.seenMutationIds] };
  }

  private persist(): void {
    const data = JSON.stringify(this.state, null, 2) + "\n";
    this.writeChain = this.writeChain.then(async () => {
      await fs.mkdir(dirname(this.path), { recursive: true, mode: 0o700 });
      const temporary = `${this.path}.tmp`;
      await fs.writeFile(temporary, data, { mode: 0o600 });
      await fs.rename(temporary, this.path);
    }).catch((error) => console.error("memory_dream_scheduler_state_write_failed", error));
  }
}
