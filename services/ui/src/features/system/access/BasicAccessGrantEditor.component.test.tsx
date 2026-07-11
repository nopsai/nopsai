import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test } from 'vitest';
import { BasicAccessGrantEditor } from './BasicAccessGrantEditor';
import {
  buildBasicGrantChangeSet,
  stageBasicGrant,
  type BasicGrantDraft,
} from './basicGrantModel';
import type { AccessGrantRecord, EditableAccessGrant } from './model';

const originalViewerGrant: AccessGrantRecord = {
  id: 'grant-viewer',
  subjectType: 'user',
  subjectID: 'user-1',
  role: 'viewer',
  resourceType: 'team',
  resourceID: 'root',
  inherit: true,
  grantedBy: 'platform-admin',
};

function BasicGrantHarness() {
  const [entries, setEntries] = useState<EditableAccessGrant[]>([
    {
      localID: originalViewerGrant.id,
      id: originalViewerGrant.id,
      role: originalViewerGrant.role,
      resourceType: originalViewerGrant.resourceType,
      resourceID: originalViewerGrant.resourceID,
      inherit: originalViewerGrant.inherit,
      grantedBy: originalViewerGrant.grantedBy,
    },
  ]);
  const [draft, setDraft] = useState<BasicGrantDraft>({ role: '', scope: 'root' });
  const [error, setError] = useState<string | null>(null);

  return (
    <>
      <BasicAccessGrantEditor
        entries={entries}
        draft={draft}
        options={[
          { value: 'root', label: 'Root' },
          { value: 'engineering', label: 'Engineering' },
        ]}
        error={error}
        showGrantedBy
        toneClassForRole={() => 'access-chip--muted'}
        onDraftChange={setDraft}
        onAdd={() => {
          const result = stageBasicGrant(entries, draft, `draft-${entries.length}`);
          setEntries(result.entries);
          setError(result.error);
          if (!result.error) setDraft(previous => ({ ...previous, role: '' }));
        }}
        onRemove={localID => setEntries(previous => previous.filter(entry => entry.localID !== localID))}
      />
      <output data-testid="grant-state">{JSON.stringify(entries)}</output>
    </>
  );
}

test('replaces a role on the same target and adds scoped access', async () => {
  const user = userEvent.setup();
  render(<BasicGrantHarness />);

  expect(screen.getByText(/This viewer basic role/)).toHaveTextContent('Granted by platform-admin.');
  await user.selectOptions(screen.getByLabelText('Access level'), 'developer');
  await user.click(screen.getByRole('button', { name: 'Add' }));

  expect(screen.queryByText('viewer')).not.toBeInTheDocument();
  expect(screen.getByText('developer')).toBeVisible();
  expect(screen.getByTestId('grant-state')).toHaveTextContent('"localID":"grant-viewer"');

  await user.selectOptions(screen.getByLabelText('Access level'), 'owner');
  await user.selectOptions(screen.getByLabelText('Team target'), 'engineering');
  await user.click(screen.getByRole('button', { name: 'Add' }));

  expect(screen.getByText('2 listed')).toBeVisible();
  expect(screen.getByText('Includes children')).toBeVisible();
  expect(screen.getByText('/engineering')).toBeVisible();

  await user.click(screen.getAllByRole('button', { name: 'Remove' })[0]);
  expect(screen.getByText('1 listed')).toBeVisible();
});

test('stages platform admin grants and reports duplicate entries', () => {
  const first = stageBasicGrant([], { role: 'admin', scope: 'engineering' }, 'draft-admin');
  expect(first.error).toBeNull();
  expect(first.entries).toEqual([
    expect.objectContaining({
      resourceType: 'platform',
      resourceID: 'platform',
      inherit: false,
    }),
  ]);

  const duplicate = stageBasicGrant(first.entries, { role: 'admin', scope: 'root' }, 'ignored');
  expect(duplicate.error).toBe('This basic role is already listed.');
  expect(duplicate.entries).toBe(first.entries);
});

test('builds the delete and create operations for changed grant targets', () => {
  const originalDeveloperGrant: AccessGrantRecord = {
    ...originalViewerGrant,
    id: 'grant-developer',
    role: 'developer',
    resourceID: 'engineering',
  };
  const changes = buildBasicGrantChangeSet(
    [originalViewerGrant, originalDeveloperGrant],
    [
      {
        localID: originalViewerGrant.id,
        id: originalViewerGrant.id,
        role: 'owner',
        resourceType: 'team',
        resourceID: 'root',
        inherit: true,
      },
      {
        localID: 'draft-platform',
        role: 'admin',
        resourceType: 'platform',
        resourceID: 'platform',
        inherit: false,
      },
    ]
  );

  expect(changes.grantsToDelete.map(grant => grant.id)).toEqual([
    'grant-viewer',
    'grant-developer',
  ]);
  expect(changes.grantsToAdd.map(grant => grant.role)).toEqual(['owner', 'admin']);
});
