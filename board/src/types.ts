export type BoardLink = {
  label: string;
  url: string;
};

export type BoardStack = {
  project: string;
  stack: string;
  mode: string;
  checkout: string;
  host: string;
  compose_project: string;
  running: boolean;
  branch: string;
  sha: string;
  last_up_at: string;
  links?: BoardLink[];
};

export type BoardStatus = {
  generated_at: string;
  listen: number;
  stacks: BoardStack[];
};
