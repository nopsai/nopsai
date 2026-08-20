import { useEffect, useMemo, useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { usePlatformVersionInfo } from '../app/usePlatformVersion';
import type { Theme } from '../app/types';
import { findWikiArticleByPath, findWikiArticleRedirect, wikiHomePath } from '../features/product-docs/content';
import { isIndexArticle } from '../features/product-docs/indexes';
import { scrollDocumentationViewport } from '../features/product-docs/scroll';
import { searchWiki, type WikiSearchResult } from '../features/product-docs/search';
import { visibleWikiBlocks, wikiBlockTitle } from '../features/product-docs/blocks';
import { DocsShell } from '../features/product-docs/DocsShell';
import { WikiArticleBody, WikiArticleHeader } from '../features/product-docs/WikiArticle';
import { WikiHome } from '../features/product-docs/WikiHome';
import { WikiIndex } from '../features/product-docs/WikiIndexes';
import { WikiPager } from '../features/product-docs/WikiNavigation';
import { copyTextToClipboard } from '../lib/clipboard';

export default function ProductDocsPage({ theme, onToggleTheme }: { theme?: Theme; onToggleTheme?: () => void } = {}) {
  const location = useLocation();
  const navigate = useNavigate();
  const [copiedKey, setCopiedKey] = useState('');
  const versionInfo = usePlatformVersionInfo();

  const query = new URLSearchParams(location.search).get('q') || '';
  const selection = useMemo(() => findWikiArticleByPath(location.pathname), [location.pathname]);
  // Pages moved between sections when the wiki was regrouped; article IDs did not.
  const redirect = useMemo(
    () => (selection ? undefined : findWikiArticleRedirect(location.pathname)),
    [location.pathname, selection],
  );
  const results = useMemo(() => searchWiki(query), [query]);
  const targetAnchor = location.hash.replace(/^#/, '') || undefined;

  useEffect(() => {
    scrollDocumentationViewport(location.hash);
  }, [location.pathname, location.hash]);

  const handleQueryChange = (nextQuery: string) => {
    const params = new URLSearchParams(location.search);
    if (nextQuery.trim()) params.set('q', nextQuery);
    else params.delete('q');
    navigate(
      { pathname: location.pathname, search: params.toString() ? `?${params.toString()}` : '', hash: location.hash },
      { replace: true },
    );
  };

  const handleSelectResult = (result: WikiSearchResult) => {
    const [pathname = wikiHomePath(), hash = ''] = result.href.split('#');
    navigate({ pathname, search: location.search, hash: hash ? `#${hash}` : '' });
  };

  const handleCopy = async (key: string, code: string) => {
    try {
      await copyTextToClipboard(code);
      setCopiedKey(key);
      window.setTimeout(() => setCopiedKey(current => (current === key ? '' : current)), 1600);
    } catch {
      setCopiedKey('');
    }
  };

  if (redirect) {
    return <Navigate to={{ pathname: redirect, search: location.search, hash: location.hash }} replace />;
  }

  const blocks = selection ? visibleWikiBlocks(selection.article) : [];
  const tocItems = blocks.map(block => ({ id: block, label: wikiBlockTitle(block, selection?.article) }));

  return (
    <DocsShell
      activeArticleID={selection?.article.id || ''}
      query={query}
      results={results}
      onQueryChange={handleQueryChange}
      onSelectResult={handleSelectResult}
      productVersion={versionInfo?.productVersion}
      theme={theme}
      onToggleTheme={onToggleTheme}
      toc={tocItems}
    >
      {selection ? (
        <article aria-labelledby="wiki-article-title">
          <WikiArticleHeader section={selection.section} article={selection.article} />
          {isIndexArticle(selection.article.id) ? (
            <div className="mt-6">
              <WikiIndex key={selection.article.id} indexID={selection.article.id} />
            </div>
          ) : null}
          <WikiArticleBody
            article={selection.article}
            blocks={blocks}
            targetAnchor={targetAnchor}
            copiedKey={copiedKey}
            onCopy={handleCopy}
          />
          <WikiPager articleID={selection.article.id} />
        </article>
      ) : (
        <WikiHome />
      )}
    </DocsShell>
  );
}
