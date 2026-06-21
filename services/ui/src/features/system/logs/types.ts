export type SystemLogStream = 'stdout' | 'stderr';

export type SystemLogEntry = {
  id: string;
  source_id: string;
  container_name: string;
  container_instance: string;
  emitted_at: string;
  observed_at: string;
  stream: SystemLogStream;
  line: string;
};

export type SystemLogSource = {
  id: string;
  display_name: string;
  container_name: string;
  container_instance?: string;
  available: boolean;
  state: string;
  health?: string;
  status?: string;
};

export type SystemLogSourcesResponse = {
  sources: SystemLogSource[];
  redaction_warning: string;
  max_tail_lines: number;
};

export type SystemLogSSEEvent =
  | { event: 'status'; data: { state: string; source_id: string } }
  | { event: 'reset'; data: { reason: string } }
  | { event: 'log'; id: string; data: SystemLogEntry };

export type SystemLogConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error';
