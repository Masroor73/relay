import { z } from 'zod';
import type { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import type { ApiClient, ApiError } from '../apiClient.js';

export function registerReplayDlqEventTool(server: McpServer, apiClient: ApiClient) {
  server.registerTool(
    'replay_dlq_event',
    {
      title: 'Replay a dead-letter event',
      description: 'Moves a dead-lettered event back into the active processing queue for reprocessing. This is a state-changing action — the event will be retried by the processor.',
      inputSchema: {
        dlq_id: z.string().uuid().describe('The dlq_id of the dead-letter event to replay'),
      },
      annotations: {
        destructiveHint: true,
        idempotentHint: false,
      },
    },
    async ({ dlq_id }) => {
      try {
        await apiClient.post(`/api/dlq/${dlq_id}/replay`);
        return {
          content: [{ type: 'text', text: `Event ${dlq_id} replayed successfully — moved back into the outbox for reprocessing.` }],
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