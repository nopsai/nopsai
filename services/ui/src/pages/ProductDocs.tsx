import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { findWikiArticleByPath, wikiHomePath } from '../features/product-docs/content';
import { isIndexArticle } from '../features/product-docs/indexes';
import { scrollDocumentationViewport } from '../features/product-docs/scroll';
import { searchWiki, type WikiSearchResult } from '../features/product-docs/search';
import { visibleWikiBlocks, wikiBlockTitle } from '../features/product-docs/blocks';
import { WikiArticleBody, WikiArticleHeader } from '../features/product-docs/WikiArticle';
import { WikiHome } from '../features/product-docs/WikiHome';
import { WikiIndex } from '../features/product-docs/WikiIndexes';
import { WikiOnThisPage, WikiPager, WikiSidebar } from '../features/product-docs/WikiNavigation';
import { copyTextToClipboard } from '../lib/clipboard';

export default function ProductDocsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const [copiedKey, setCopiedKey] = useState('');

  const query = new URLSearchParams(location.search).get('q') || '';
  const selection = useMemo(() => findWikiArticleByPath(location.pathname), [location.pathname]);
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

  const blocks = selection ? visibleWikiBlocks(selection.article) : [];
  const tocItems = blocks.map(block => ({ id: block, label: wikiBlockTitle(block, selection?.article) }));

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto grid w-full max-w-[1400px] gap-6 px-4 py-4 sm:px-5 lg:grid-cols-[15rem_minmax(0,1fr)] lg:px-6 xl:grid-cols-[15rem_minmax(0,1fr)_10rem]">
        <WikiSidebar
          activeArticleID={selection?.article.id || ''}
          query={query}
          results={results}
          onQueryChange={handleQueryChange}
          onSelectResult={handleSelectResult}
          showSearch={Boolean(selection)}
        />
        <main className="min-w-0 pb-12" aria-labelledby={selection ? 'wiki-article-title' : undefined}>
          {selection ? (
            <article className="mx-auto max-w-3xl">
              <WikiArticleHeader section={selection.section} article={selection.article} />
              {isIndexArticle(selection.article.id) ? (
                <div className="mt-6">
                  <WikiIndex indexID={selection.article.id} />
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
            <WikiHome
              query={query}
              results={results}
              onQueryChange={handleQueryChange}
              onSelectResult={handleSelectResult}
            />
          )}
        </main>
        {selection ? <WikiOnThisPage items={tocItems} /> : <span />}
      </div>
    </div>
  );
}
