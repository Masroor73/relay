import type { Config } from './config.js';

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

// Shared client for calling cmd/server's dashboard API, authenticated the
// same way the dashboard itself is (Basic Auth, same DASHBOARD_PASSWORD
// credential) — not a new or separate auth mechanism for MCP specifically.
export class ApiClient {
  private authHeader: string;

  constructor(private config: Config) {
    this.authHeader = 'Basic ' + Buffer.from(`mcp-server:${config.dashboardPassword}`).toString('base64');
  }

  async get<T>(path: string): Promise<T> {
    const response = await fetch(`${this.config.apiBaseUrl}${path}`, {
      headers: { Authorization: this.authHeader },
    });
    return this.handleResponse<T>(response);
  }

  async post<T>(path: string): Promise<T> {
    const response = await fetch(`${this.config.apiBaseUrl}${path}`, {
      method: 'POST',
      headers: { Authorization: this.authHeader },
    });
    return this.handleResponse<T>(response);
  }

  private async handleResponse<T>(response: Response): Promise<T> {
    if (!response.ok) {
      const body = await response.text().catch(() => '');
      throw new ApiError(response.status, `Request failed: ${response.status} ${body}`);
    }
    // Replay's success response has no body (ENG-34's handler returns
    // 200 with an empty body) — guard against parsing empty JSON.
    const text = await response.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }
}