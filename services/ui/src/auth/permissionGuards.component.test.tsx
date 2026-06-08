import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { expect, test } from 'vitest';
import { PermissionGuard } from './permissionGuards';

function renderGuard({ allowed, loading }: { allowed: boolean; loading: boolean }) {
  return render(
    <MemoryRouter initialEntries={['/protected']}>
      <Routes>
        <Route
          path="/protected"
          element={(
            <PermissionGuard allowed={allowed} loading={loading} fallbackPath="/fallback">
              <div>Protected content</div>
            </PermissionGuard>
          )}
        />
        <Route path="/fallback" element={<div>Permission fallback</div>} />
      </Routes>
    </MemoryRouter>
  );
}

test('keeps protected content hidden while permissions load', () => {
  renderGuard({ allowed: false, loading: true });
  expect(screen.getByText('Loading access...')).toBeVisible();
  expect(screen.queryByText('Permission fallback')).not.toBeInTheDocument();
});

test('renders allowed content and redirects denied access', () => {
  const allowed = renderGuard({ allowed: true, loading: false });
  expect(screen.getByText('Protected content')).toBeVisible();
  allowed.unmount();

  renderGuard({ allowed: false, loading: false });
  expect(screen.getByText('Permission fallback')).toBeVisible();
  expect(screen.queryByText('Protected content')).not.toBeInTheDocument();
});
