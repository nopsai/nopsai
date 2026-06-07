import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from '../lib/api';
import ResourceAccessCard from './ResourceAccessCard';

describe('ResourceAccessCard', () => {
  beforeEach(() => {
    vi.spyOn(apiClient, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === '/v1/groups' || path === '/v1/admin/service-accounts') {
        return Response.json([]);
      }
      if (path.endsWith('/grants') && init?.method === 'POST') {
        return Response.json({}, { status: 201 });
      }
      return Response.json({
        resource: 'pipeline:platform/deploy',
        resource_type: 'pipeline',
        resource_id: 'platform/deploy',
        visibility: 'restricted',
        use_access: { grants: [] },
        manage_access: { mode: 'owners' },
      });
    });
  });

  it('loads access settings and submits an explicit repository grant', async () => {
    render(<ResourceAccessCard resourceType="pipeline" resourceID="platform/deploy" label="pipeline" />);
    await userEvent.click(screen.getByRole('button', { name: 'Access' }));

    expect(await screen.findByRole('heading', { name: 'Who can use this pipeline?' })).toBeInTheDocument();
    const repositoryInput = screen.getByPlaceholderText('owner/repo');
    await userEvent.type(repositoryInput, 'acme/payments');
    await userEvent.click(screen.getByRole('button', { name: 'Add' }));

    await waitFor(() => {
      expect(apiClient.fetch).toHaveBeenCalledWith(
        '/v1/resources/pipeline/platform/deploy/grants',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            subject_type: 'repository',
            subject_id: 'acme/payments',
            actions: ['pipeline.use'],
            conditions: { branches: [], events: [] },
          }),
        })
      );
    });
  });
});
