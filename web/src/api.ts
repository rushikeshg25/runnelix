export interface GraphNode {
  id: string;
  type: string;
  version: number;
  config: Record<string, unknown>;
}
export interface Endpoint {
  node: string;
  port: string;
}
export interface Graph {
  formatVersion: 1;
  nodes: GraphNode[];
  edges: { from: Endpoint; to: Endpoint }[];
}
export interface Field {
  type?: string;
  title?: string;
  enum?: string[];
  minimum?: number;
  maximum?: number;
  maxLength?: number;
}
export interface Descriptor {
  type: string;
  version: number;
  title: string;
  kind: string;
  inputs: string[];
  outputs: string[];
  retrySafety: string;
  implementationDigest: string;
  configSchema: { properties?: Record<string, Field>; required?: string[] };
}
export interface Workflow {
  id: string;
  name: string;
  revision: number;
  graph: Graph;
  activeVersionId: string | null;
}
export interface Run {
  runId: string;
  workflowId: string;
  workflowVersionId: string;
  state: string;
  executionMode: string;
  cancelRequested: boolean;
  checkpoint: number;
  dispatchState: string;
}
export interface RunEvent {
  sequence: number;
  state: string;
  nodeId?: string;
  nodeState?: string;
  invocationId?: string;
  outputRef?: string;
  errorCode?: string;
  recordedAt: string;
}
export interface Attempt {
  invocationId: string;
  attempt: number;
  status: string;
  outputRef: string | null;
  errorCode: string | null;
}
export interface Version {
  id: string;
  graph: Graph;
  digest: string;
  sequence: number;
  draftRevision: number;
}
export interface Credential {
  id: string;
  name: string;
  kind: string;
  destination: string;
  revoked: boolean;
}
export class APIError extends Error {
  readonly status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}
// Session CSRF state stays in memory. Provider tokens never enter this module.
let csrfToken = "";
export function setCSRFToken(value: string) {
  csrfToken = value;
}
export const sessionExpiredEvent = "runnelix-session-expired";
export interface BrowserSession {
  mode: "development" | "oidc";
  authenticated: boolean;
  loginEnabled?: boolean;
  csrfToken?: string;
  expiresAt?: string;
}
export async function request<T>(
  path: string,
  method = "GET",
  value?: unknown,
  headers: Record<string, string> = {},
): Promise<T> {
  return requestJSON<T>(
    path,
    method,
    value === undefined ? undefined : JSON.stringify(value),
    headers,
  );
}
// Use only for pre-encoded JSON whose original representation must reach the server.
export async function requestJSON<T>(
  path: string,
  method: string,
  body?: string,
  headers: Record<string, string> = {},
): Promise<T> {
  if (!path.startsWith("/v1/") || path.includes("\\"))
    throw new Error("API requests require a local /v1/ path");
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...headers,
      ...(csrfToken && method !== "GET" && method !== "HEAD"
        ? { "X-CSRF-Token": csrfToken }
        : {}),
    },
    ...(body !== undefined ? { body } : {}),
    signal: AbortSignal.timeout(12000),
  });
  if (response.status === 401 && path !== "/v1/auth/session") {
    csrfToken = "";
    window.dispatchEvent(new Event(sessionExpiredEvent));
  }
  const data = await response.json();
  if (!response.ok)
    throw new APIError(
      [
        data.message ?? data.code ?? "Request failed",
        ...(data.issues ?? []).map(
          (i: { path: string; message: string }) => `${i.path}: ${i.message}`,
        ),
        data.requestId ? `Reference: ${data.requestId}` : "",
      ]
        .filter(Boolean)
        .join("\n"),
      response.status,
    );
  return data as T;
}
export const workspace = "workspace_dev";
export const root = `/v1/workspaces/${workspace}`;
export const emptyGraph = (): Graph => ({
  formatVersion: 1,
  nodes: [
    { id: "start", type: "trigger.manual", version: 1, config: {} },
    { id: "echo", type: "mock.echo", version: 1, config: {} },
  ],
  edges: [
    { from: { node: "start", port: "out" }, to: { node: "echo", port: "in" } },
  ],
});
