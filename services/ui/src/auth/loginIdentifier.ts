export function isEmailLikeIdentifier(identifier: string): boolean {
  const trimmed = identifier.trim();
  const at = trimmed.indexOf("@");
  return at > 0 && at < trimmed.length - 1;
}

export function shouldUseLocalPasswordForIdentifier({
  identifier,
  localEnabled,
  ssoEnabled,
}: {
  identifier: string;
  localEnabled: boolean;
  ssoEnabled: boolean;
}): boolean {
  return localEnabled && ssoEnabled && identifier.trim() !== "" && !isEmailLikeIdentifier(identifier);
}
