import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { loadConfig } from './config.js';
import { ApiClient } from './apiClient.js';

const config = loadConfig();
const apiClient = new ApiClient(config);

const server = new McpServer({
  name: 'relay-mcp-server',
  version: '0.1.0',
});

// Tools registered here in ENG-44 (list_dlq_events), ENG-45
// (inspect_dlq_event), ENG-46 (replay_dlq_event).

const transport = new StdioServerTransport();
await server.connect(transport);