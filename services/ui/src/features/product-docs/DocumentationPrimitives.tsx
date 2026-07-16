export function SectionFrame({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <section id={id} className="scroll-mt-8 border-t border-[var(--border-primary)] pt-8">
      <h2 className="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">{title}</h2>
      <div className="mt-4">{children}</div>
    </section>
  );
}

export function MetadataItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{label}</dt>
      <dd className="mt-1 text-[var(--text-secondary)]">{value}</dd>
    </div>
  );
}

export function CompactList({ title, items, code = false }: { title: string; items: string[]; code?: boolean }) {
  if (!items.length) return null;
  return (
    <div className="mt-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{title}</p>
      <ul className="mt-2 space-y-1 pl-5 text-sm leading-6 text-[var(--text-secondary)]">
        {items.map((item, index) => <li key={`${title}-${index}`} className="list-disc">{code ? <code className="text-xs">{item}</code> : item}</li>)}
      </ul>
    </div>
  );
}

export function Notice({
  title,
  children,
  tone = 'neutral',
}: {
  title: string;
  children: React.ReactNode;
  tone?: 'neutral' | 'warning';
}) {
  return (
    <div className={`mt-4 rounded border px-4 py-3 ${tone === 'warning' ? 'border-amber-500/50' : 'border-[var(--border-primary)]'}`}>
      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{title}</p>
      <div className="mt-1 text-sm leading-6 text-[var(--text-secondary)]">{children}</div>
    </div>
  );
}
