import { z } from 'zod';
import type { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import type { ApiClient } from '../apiClient.js';

interface DlqEntry {
  dlq_id: string;
  event_id: string;
  final_error: string;
  moved_at: string;
}

export function registerListDlqEventsTool(server: McpServer, apiClient: ApiClient) {
  server.registerTool(
    'list_dlq_events',
    {
      title: 'List dead-letter events',
      description: 'Lists webhook events that exhausted all retry attempts and were moved to the dead-letter queue.',
      inputSchema: {},
    },
    async () => {
      const entries = await apiClient.get<DlqEntry[]>('/api/dlq');

      return {
        content: [
          {
            type: 'text',
            text: JSON.stringify(entries, null, 2),
          },
        ],
      };
    },
  );
}