export type PlatformVersionInfo = {
  productVersion: string;
  commit: string;
  buildDate: string;
  apiVersion: string;
  cliCompatibility: string;
  runnerCompatibility: string;
  runnerProtocolVersion: string;
  releaseManifestDigest: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function readString(value: unknown): string {
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return '';
}

/**
 * The whole of /version, not just the number.
 *
 * The endpoint answers with the server's public build info — commit, build date,
 * API version, and the CLI and runner compatibility ranges. The footer only ever
 * showed the product version; the About dialog shows the rest, because those are
 * the values an operator is asked for when reporting a problem.
 */
export function normalizePlatformVersionInfo(payload: unknown): PlatformVersionInfo | null {
  if (!isRecord(payload)) return null;
  const productVersion = readString(payload.productVersion) || readString(payload.version);
  if (!productVersion) return null;
  return {
    productVersion,
    commit: readString(payload.commit),
    buildDate: readString(payload.buildDate),
    apiVersion: readString(payload.apiVersion),
    cliCompatibility: readString(payload.cliCompatibility),
    runnerCompatibility: readString(payload.runnerCompatibility),
    runnerProtocolVersion: readString(payload.runnerProtocolVersion),
    releaseManifestDigest: readString(payload.releaseManifestDigest),
  };
}

export function appVersionFooterText(info: PlatformVersionInfo | null): string {
  return info?.productVersion ? `Version ${info.productVersion}` : '';
}
