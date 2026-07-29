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
  const selectedID = gitWebhookSourceIDFromPath(location.pathname);
  const onSelect = useCallback((sourceID: string) => {
    navigate(sourceID ? `/git-webhook-sources/${encodeRouteIdentifier(sourceID)}` : '/git-webhook-sources');
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

function gitWebhookSourceIDFromPath(pathname: string) {
  const parts = pathname.split('/').filter(Boolean);
  if (parts[0] !== 'git-webhook-sources' || parts.length < 2) return '';
  return parts.slice(1).map(part => {
    try {
      return decodeURIComponent(part);
    } catch {
      return part;
    }
  }).join('/');
}

function encodeRouteIdentifier(identifier: string) {
  return identifier.split('/').filter(Boolean).map(encodeURIComponent).join('/');
}
