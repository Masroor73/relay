import { describe, it, expect, beforeAll } from 'vitest';
import { createTestClient } from '../testHelpers.js';
import type { Client } from '@modelcontextprotocol/sdk/client/index.js';

// These are integration tests against a real cmd/server instance and a
// real Postgres connection, same as every *_test.go file in this project
// — skipped entirely if the environment isn't set up, not mocked.
const canRun = Boolean(process.env.DATABASE_URL && process.env.API_BASE_URL);

describe.skipIf(!canRun)('MCP DLQ tools', () => {
  let client: Client;

  beforeAll(async () => {
    client = await createTestClient();
  });

  it('list_dlq_events returns a valid tool response', async () => {
    const result = await client.callTool({ name: 'list_dlq_events', arguments: {} });
    expect(result.isError).toBeFalsy();
    expect(result.content).toBeInstanceOf(Array);
  });

  it('inspect_dlq_event rejects a malformed dlq_id via schema validation', async () => {
    const result = await client.callTool({
      name: 'inspect_dlq_event',
      arguments: { dlq_id: 'not-a-uuid' },
    });
    expect(result.isError).toBe(true);
  
    const content = result.content as Array<{ type: string; text: string }>;
    expect(content[0].text).toContain('Invalid UUID');
  });
    

  it('inspect_dlq_event returns isError for a nonexistent dlq_id', async () => {
    const result = await client.callTool({
      name: 'inspect_dlq_event',
      arguments: { dlq_id: '00000000-0000-0000-0000-000000000000' },
    });
    expect(result.isError).toBe(true);
  });

  it('replay_dlq_event returns isError for a nonexistent dlq_id', async () => {
    const result = await client.callTool({
      name: 'replay_dlq_event',
      arguments: { dlq_id: '00000000-0000-0000-0000-000000000000' },
    });
    expect(result.isError).toBe(true);
  });
});