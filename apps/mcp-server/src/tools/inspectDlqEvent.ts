import { z } from 'zod';
import type { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import type { ApiClient, ApiError } from '../apiClient.js';

interface DlqEntry {
  dlq_id: string;
  event_id: string;
  final_error: string;
  moved_at: string;
}

export function registerInspectDlqEventTool(server: McpServer, apiClient: ApiClient) {
  server.registerTool(
    'inspect_dlq_event',
    {
      title: 'Inspect a dead-letter event',
      description: 'Fetches the full detail of a single dead-lettered event by its dlq_id, including the final error that caused it to be dead-lettered.',
      inputSchema: {
        dlq_id: z.string().uuid().describe('The dlq_id of the dead-letter event to inspect'),
      },
    },
    async ({ dlq_id }) => {
      try {
        const entry = await apiClient.get<DlqEntry>(`/api/dlq/${dlq_id}`);
        return {
          content: [{ type: 'text', text: JSON.stringify(entry, null, 2) }],
        };
      } catch (err) {
        const apiErr = err as ApiError;
        if (apiErr.status === 404) {
          return {
            content: [{ type: 'text', text: `No dead-letter event found with dlq_id ${dlq_id}` }],
            isError: true,
          };
        }
        throw err;
      }
    },
  );
}