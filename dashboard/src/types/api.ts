export interface Pipeline {
  name: string;
  namespace: string;
  triggerType: string;
  triggerInfo: string;
  triggerJql?: string;
  pollInterval: string;
  activeRuns: number;
  totalRuns: number;
}

export interface SelectedRepo {
  owner: string;
  name: string;
  forkOwner?: string;
}

export interface TriageResult {
  repo: string;
  confidence: string;
  reasoning: string;
}

export interface PipelineRun {
  name: string;
  namespace: string;
  pipeline: string;
  issueNumber: number;
  issueKey: string;
  issueTitle: string;
  phase: string;
  currentStep: string;
  branch: string;
  waitingFor?: string;
  diffJobName?: string;
  chatPodName?: string;
  resolvedRepo?: SelectedRepo;
  triageResult?: TriageResult;
  startedAt: string | null;
  finishedAt: string | null;
  durationSeconds?: number;
  steps: StepStatus[];
}

export interface IssueHistoryRecord {
  pipelineNamespace: string;
  pipelineName: string;
  issueKey: string;
  phase: string;
  runName: string;
  completedAt: string;
}

export interface PendingIssue {
  key: string;
  title: string;
}

export interface StepStatus {
  name: string;
  type: string;
  phase: string;
  startedAt: string | null;
  finishedAt: string | null;
  durationSeconds?: number;
  jobName: string;
  attempt: number;
  message: string;
}
