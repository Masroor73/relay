import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { InMemoryTransport } from '@modelcontextprotocol/sdk/inMemory.js';
import { ApiClient } from './apiClient.js';
import { registerListDlqEventsTool } from './tools/listDlqEvents.js';
import { registerInspectDlqEventTool } from './tools/inspectDlqEvent.js';
import { registerReplayDlqEventTool } from './tools/replayDlqEvent.js';

// Builds a real, fully-wired MCP server + client pair connected via an
// in-memory transport - no subprocess, no stdio, but every tool call
// still goes through the real registerTool handlers and the real
// ApiClient against a live cmd/server instance (DATABASE_URL/API_BASE_URL
// still required, same as every other integration test in this project).
export async function createTestClient() {
  const apiClient = new ApiClient({
    apiBaseUrl: process.env.API_BASE_URL ?? 'http://localhost:8080',
    dashboardPassword: requireEnv('DASHBOARD_PASSWORD'),
  });

  const server = new McpServer({ name: 'relay-mcp-server-test', version: '0.1.0' });
  registerListDlqEventsTool(server, apiClient);
  registerInspectDlqEventTool(server, apiClient);
  registerReplayDlqEventTool(server, apiClient);

  const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
  const client = new Client({ name: 'test-client', version: '0.1.0' });

  await Promise.all([server.connect(serverTransport), client.connect(clientTransport)]);

  return client;
}

function requireEnv(key: string): string {
  const value = process.env[key];
  if (!value) throw new Error(`${key} is required for tests`);
  return value;
}