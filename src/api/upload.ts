/** POST /api/upload/pdf (spec 00-03 Layer 3.3) — plain fetch, not the
 * json-only `http` helper, since this is a multipart upload with a
 * differently-shaped success body. */
export async function uploadPdf(file: File): Promise<{ path: string }> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await fetch('/api/upload/pdf', { method: 'POST', body: formData });
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error ?? response.statusText);
  }
  return (await response.json()) as { path: string };
}
