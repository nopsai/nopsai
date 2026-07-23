export type PlatformVersionInfo = {
  productVersion: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function readString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

export function normalizePlatformVersionInfo(payload: unknown): PlatformVersionInfo | null {
  if (!isRecord(payload)) return null;
  const productVersion = readString(payload.productVersion) || readString(payload.version);
  return productVersion ? { productVersion } : null;
}

export function appVersionFooterText(info: PlatformVersionInfo | null): string {
  return info?.productVersion ? `Version ${info.productVersion}` : '';
}
