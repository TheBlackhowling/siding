import type { BoardStack } from "./types";

export type StackGroup = {
  stack: string;
  checkout: string;
  rows: BoardStack[];
};

export type ProjectGroup = {
  project: string;
  up: number;
  allocated: number;
  stacks: StackGroup[];
};

function sortRows(rows: BoardStack[]): BoardStack[] {
  return [...rows].sort((a, b) => {
    if (a.running !== b.running) {
      return a.running ? -1 : 1;
    }
    return a.mode.localeCompare(b.mode);
  });
}

export function groupByProject(rows: BoardStack[]): ProjectGroup[] {
  const byProject = new Map<string, BoardStack[]>();
  for (const row of rows) {
    const key = row.project || row.stack || row.checkout;
    const list = byProject.get(key) ?? [];
    list.push(row);
    byProject.set(key, list);
  }
  const projects = [...byProject.entries()].map(([project, items]) => {
    const byStack = new Map<string, BoardStack[]>();
    for (const row of items) {
      const key = row.stack || row.checkout;
      const list = byStack.get(key) ?? [];
      list.push(row);
      byStack.set(key, list);
    }
    const stacks = [...byStack.entries()]
      .map(([stack, stackRows]) => ({
        stack,
        checkout: stackRows[0]?.checkout ?? "",
        rows: sortRows(stackRows),
      }))
      .sort((a, b) => a.stack.localeCompare(b.stack));
    return {
      project,
      up: items.filter((r) => r.running).length,
      allocated: items.filter((r) => !r.running).length,
      stacks,
    };
  });
  return projects.sort((a, b) => {
    if ((a.up > 0) !== (b.up > 0)) {
      return a.up > 0 ? -1 : 1;
    }
    return a.project.localeCompare(b.project);
  });
}
