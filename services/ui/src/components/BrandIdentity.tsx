type BrandIdentityProps = {
  className?: string;
  variant?: 'mark' | 'wordmark';
};

export function BrandIdentity({ className = '', variant = 'wordmark' }: BrandIdentityProps) {
  return (
    <span
      className={`brand-identity brand-identity--${variant} ${className}`.trim()}
      role="img"
      aria-label="NopsAI"
    >
      <span className="brand-identity__mark" aria-hidden="true" />
      {variant === 'wordmark' ? <span className="brand-identity__word" aria-hidden="true">nopsai</span> : null}
    </span>
  );
}

export default BrandIdentity;
