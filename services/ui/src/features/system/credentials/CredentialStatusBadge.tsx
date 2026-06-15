function statusClass(status: string) {
  if (status === 'active') return 'runner-pill runner-pill--ok';
  if (status === 'disabled') return 'runner-pill runner-pill--error';
  return 'runner-pill runner-pill--muted';
}

export function CredentialStatusBadge({ status }: { status: string }) {
  return <span className={statusClass(status)}>{status}</span>;
}
