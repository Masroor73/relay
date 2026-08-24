export interface Config {
    apiBaseUrl: string;
    dashboardPassword: string;
  }
  
  export function loadConfig(): Config {
    const apiBaseUrl = process.env.API_BASE_URL ?? 'http://localhost:8080';
    const dashboardPassword = process.env.DASHBOARD_PASSWORD;
  
    if (!dashboardPassword) {
      throw new Error('DASHBOARD_PASSWORD is required');
    }
  
    return { apiBaseUrl, dashboardPassword };
  }