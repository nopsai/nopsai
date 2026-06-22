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
    />
  );
}

export default BrandIdentity;
