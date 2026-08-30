// Stable short IDs for internal entity references.
// IDs survive renames — YAML export resolves back to names.

let counter = 0;

/** Generate a short unique ID (8 hex chars) */
export function generateEntityId(): string {
  const time = Date.now().toString(36);
  const rand = Math.random().toString(36).slice(2, 6);
  counter++;
  return `${time.slice(-4)}${rand}${(counter % 16).toString(16)}`;
}

/** Resolve a provider reference (by _providerId or name fallback) to the current name */
export function resolveProviderName(
  providerId: string | undefined,
  providerName: string,
  providers: Record<string, { _id?: string }>,
): string {
  if (!providerId) return providerName;
  // Find provider by _id
  for (const [name, def] of Object.entries(providers)) {
    if (def._id === providerId) return name;
  }
  // ID not found — fall back to name (provider may have been deleted)
  return providerName;
}

/** Find provider ID by name */
export function findProviderId(
  providerName: string,
  providers: Record<string, { _id?: string }>,
): string | undefined {
  return providers[providerName]?._id;
}

/** Strip internal fields (_id, _providerId) from an object for YAML export */
export function stripInternalFields<T extends Record<string, unknown>>(obj: T): T {
  const clean = { ...obj };
  for (const key of Object.keys(clean)) {
    if (key.startsWith('_')) delete clean[key];
  }
  return clean;
}
