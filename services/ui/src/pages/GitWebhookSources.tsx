import { useCallback } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { GitWebhookSourcesView } from '../features/git-webhook-sources/GitWebhookSourcesView';
import { useGitWebhookSources } from '../features/git-webhook-sources/useGitWebhookSources';

export default function GitWebhookSourcesPage({
  canWriteGitWebhookSources,
  canDeleteGitWebhookSources,
}: {
  canWriteGitWebhookSources: boolean;
  canDeleteGitWebhookSources: boolean;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const selectedID = decodeURIComponent(location.pathname.split('/').filter(Boolean)[1] || '');
  const onSelect = useCallback((sourceID: string) => {
    navigate(sourceID ? `/git-webhook-sources/${encodeURIComponent(sourceID)}` : '/git-webhook-sources');
  }, [navigate]);
  const controller = useGitWebhookSources({
    selectedID,
    canWrite: canWriteGitWebhookSources,
    canDelete: canDeleteGitWebhookSources,
    onSelect,
  });

  return (
    <GitWebhookSourcesView
      controller={controller}
      canWrite={canWriteGitWebhookSources}
      canDelete={canDeleteGitWebhookSources}
    />
  );
}
