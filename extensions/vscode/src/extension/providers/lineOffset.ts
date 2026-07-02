export class LineOffsetTracker {
  private records = new Map<string, Array<{ line: number; delta: number }>>();

  record(file: string, line: number, delta: number): void {
    const arr = this.records.get(file) ?? [];
    arr.push({ line, delta });
    this.records.set(file, arr);
  }

  adjusted(file: string, line: number): number {
    const arr = this.records.get(file) ?? [];
    let offset = 0;
    for (const r of arr) if (r.line < line) offset += r.delta;
    return Math.max(0, line + offset);
  }

  clear(): void {
    this.records.clear();
  }
}
