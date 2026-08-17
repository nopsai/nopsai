import type { GitHubAppRegistrationStart } from './model.js';

/**
 * GitHub only accepts an App manifest through a form submission from the
 * operator's own browser session, so the registration cannot be a fetch call:
 * the page has to post the manifest and follow the redirect to GitHub.
 */
export function submitGitHubAppManifest(start: GitHubAppRegistrationStart, target: Document = document): void {
  if (!start.post_url || !start.manifest) {
    throw new Error('GitHub App registration did not return a manifest.');
  }
  const form = target.createElement('form');
  form.method = 'POST';
  form.action = start.post_url;
  form.style.display = 'none';

  const field = target.createElement('input');
  field.type = 'hidden';
  field.name = 'manifest';
  field.value = start.manifest;
  form.appendChild(field);

  target.body.appendChild(form);
  form.submit();
}

/** Removes the callback markers GitHub appended, so a reload is not replayed. */
export function clearGitHubAppCallbackParams(): void {
  if (typeof window === 'undefined' || !window.history?.replaceState) return;
  const url = new URL(window.location.href);
  if (!url.searchParams.has('github_app') && !url.searchParams.has('github_app_error')) return;
  url.searchParams.delete('github_app');
  url.searchParams.delete('github_app_error');
  window.history.replaceState({}, '', url.pathname + (url.search ? url.search : '') + url.hash);
}
