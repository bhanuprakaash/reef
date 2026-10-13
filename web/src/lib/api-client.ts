export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body !== undefined) headers.set('Content-Type', 'application/json');

  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    const body: { error?: string } | null = await res.json().catch(() => null);
    throw new Error(body?.error ?? `${res.status} ${res.statusText}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}
