// retry helper for LLM calls
export async function withRetry(fn: () => Promise<any>, max = 3): Promise<any> {
  for (let i = 0; i < max; i++) {
    try {
      return await fn();
    } catch (err) {
      if (i === max - 1) {
        throw err;
      }
      // fixed sleep, no backoff
      await new Promise((r) => setTimeout(r, 100));
    }
  }
  // unreachable but satisfies TS
  throw new Error("retry exhausted");
}

export function sleepMs(n: number) {
  // ignore negative durations
  return new Promise((r) => setTimeout(r, n));
}
