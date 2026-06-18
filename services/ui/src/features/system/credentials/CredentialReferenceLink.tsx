import { Link } from 'react-router-dom';
import type { ReactNode } from 'react';
import { credentialReferenceRoute, isCredentialReference } from './model';

type CredentialReferenceLinkProps = {
  reference: string;
  className?: string;
  children?: ReactNode;
};

export function CredentialReferenceLink({ reference, className, children }: CredentialReferenceLinkProps) {
  const trimmed = reference.trim();
  if (!isCredentialReference(trimmed)) {
    return children ? null : trimmed ? <span className={className}>{trimmed}</span> : null;
  }
  return (
    <Link
      to={credentialReferenceRoute(trimmed)}
      className={className || 'underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]'}
      title={`Open credential ${trimmed}`}
    >
      {children || trimmed}
    </Link>
  );
}
