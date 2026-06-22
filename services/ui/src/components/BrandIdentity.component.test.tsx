import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import BrandIdentity from './BrandIdentity';

describe('BrandIdentity', () => {
  it('renders the accessible wordmark contract by default', () => {
    render(<BrandIdentity className="login-brand" />);

    expect(screen.getByRole('img', { name: 'NopsAI' })).toHaveClass(
      'brand-identity',
      'brand-identity--wordmark',
      'login-brand'
    );
  });

  it('supports the compact mark without changing its accessible name', () => {
    render(<BrandIdentity variant="mark" className="brand-preview" />);

    expect(screen.getByRole('img', { name: 'NopsAI' })).toHaveClass(
      'brand-identity--mark',
      'brand-preview'
    );
  });
});
