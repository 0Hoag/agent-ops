import { useHealth } from './services/health';

export function HealthPage() {
  const health = useHealth();
  return <section><h2 className="mb-2 text-2xl font-semibold">API health</h2><p className="mb-6 text-slate-600">Service connectivity status</p><div className="max-w-xl rounded-lg border border-slate-200 bg-white p-6 shadow-sm">{health.isPending ? <p role="status">Loading API status…</p> : health.isError ? <p role="alert">API status unavailable</p> : health.data ? <p>API status: <strong>{health.data.status}</strong></p> : <p>No health data available.</p>}</div></section>;
}
