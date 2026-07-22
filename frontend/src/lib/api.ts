const DEFAULT_API_URL = "http://127.0.0.1:8000";

export function apiUrl() {
  return (process.env.THIEF_API_URL ?? DEFAULT_API_URL).replace(/\/$/, "");
}
