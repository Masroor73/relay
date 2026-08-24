import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { loadConfig } from './config.js';
import { ApiClient } from './apiClient.js';
import { registerListDlqEventsTool } from './tools/listDlqEvents.js';

const config = loadConfig();
const apiClient = new ApiClient(config);

const server = new McpServer({
  name: 'relay-mcp-server',
  version: '0.1.0',
});

registerListDlqEventsTool(server, apiClient);

// inspect_dlq_event (ENG-45) and replay_dlq_event (ENG-46) registered here.

const transport = new StdioServerTransport();
await server.connect(transport);