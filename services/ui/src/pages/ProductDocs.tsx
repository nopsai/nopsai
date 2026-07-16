import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { wikiArticlePath } from '../features/product-docs/model';
import { ArticleHeader, Boundaries, Details, KeyFacts, Prerequisites, Procedure, Sources } from '../features/product-docs/DocumentationArticle';
import { Examples } from '../features/product-docs/DocumentationExamples';
import { ArticleNavigation, DocumentationSidebar, OnThisPage } from '../features/product-docs/DocumentationNavigation';
import { FieldReference, OperationalGuidance } from '../features/product-docs/DocumentationReference';
import {
  documentationSections,
  findDocumentationArticle,
  findDocumentationArticleByPath,
  findDocumentationSectionForArticle,
  getArticleSectionOrder,
  type DocumentationArticle,
} from '../features/product-docs/quality';
import { searchDocumentation, type DocumentationSearchResult } from '../features/product-docs/search';

type ArticleBlock = 'prerequisites' | 'procedure' | 'key-facts' | 'details' | 'configuration' | 'examples' | 'operations' | 'boundaries' | 'sources';

const blockLabels: Record<ArticleBlock, string> = {
  prerequisites: 'Prerequisites',
  procedure: 'Procedure',
  'key-facts': 'Key points',
  details: 'How it works',
  configuration: 'Field reference',
  examples: 'Examples',
  operations: 'Operational guidance',
  boundaries: 'Boundaries',
  sources: 'Sources',
};

function getVisibleBlocks(article: DocumentationArticle) {
  const visible: Record<ArticleBlock, boolean> = {
    prerequisites: article.prerequisites.length > 0,
    procedure: article.steps.length > 0,
    'key-facts': article.keyFacts.length > 0,
    details: article.details.length > 0,
    configuration: article.configRows.length > 0,
    examples: article.examples.length > 0,
    operations: article.runbookEntries.length > 0,
    boundaries: article.caveats.length > 0,
    sources: article.sourceLinks.length > 0,
  };
  return getArticleSectionOrder(article).filter(block => visible[block as ArticleBlock]) as ArticleBlock[];
}

export default function ProductDocsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const [copiedKey, setCopiedKey] = useState('');
  const query = new URLSearchParams(location.search).get('q') || '';
  const routeSelection = useMemo(() => findDocumentationArticleByPath(location.pathname), [location.pathname]);
  const activeArticle = routeSelection?.article || findDocumentationArticle(documentationSections[0]?.articles[0]?.id || '');
  const activeSection = activeArticle ? findDocumentationSectionForArticle(activeArticle.id) : undefined;
  const searchResults = useMemo(() => searchDocumentation(query), [query]);
  const visibleBlocks = useMemo(() => activeArticle ? getVisibleBlocks(activeArticle) : [], [activeArticle]);
  const tocItems = visibleBlocks.map(block => ({ id: block, label: blockLabels[block] }));

  useEffect(() => {
    scrollDocumentationViewport(location.hash);
  }, [location.pathname, location.hash]);

  if (!activeArticle || !activeSection) return null;

  const handleQueryChange = (nextQuery: string) => {
    const params = new URLSearchParams(location.search);
    if (nextQuery.trim()) params.set('q', nextQuery);
    else params.delete('q');
    navigate({ pathname: location.pathname, search: params.toString() ? `?${params.toString()}` : '', hash: location.hash }, { replace: true });
  };

  const handleSelectArticle = (articleID: string) => {
    const section = findDocumentationSectionForArticle(articleID);
    if (!section) return;
    const pathname = wikiArticlePath(section.id, articleID);
    navigate({ pathname, search: location.search });
    if (pathname === location.pathname && !location.hash) scrollDocumentationViewport('');
  };

  const handleSelectSearchResult = (result: DocumentationSearchResult) => {
    const [pathname = '/docs', hash = ''] = result.href.split('#');
    navigate({ pathname, search: location.search, hash: hash ? `#${hash}` : '' });
  };

  const handleCopy = async (key: string, code: string) => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) return;
    try {
      await navigator.clipboard.writeText(code);
      setCopiedKey(key);
      window.setTimeout(() => setCopiedKey(current => current === key ? '' : current), 1600);
    } catch {
      setCopiedKey('');
    }
  };

  const renderBlock = (block: ArticleBlock) => {
    switch (block) {
      case 'prerequisites': return <Prerequisites key={block} items={activeArticle.prerequisites} />;
      case 'procedure': return <Procedure key={block} article={activeArticle} copiedKey={copiedKey} onCopy={handleCopy} />;
      case 'key-facts': return <KeyFacts key={block} article={activeArticle} />;
      case 'details': return <Details key={block} article={activeArticle} />;
      case 'configuration': return <FieldReference key={block} article={activeArticle} />;
      case 'examples': return <Examples key={block} article={activeArticle} copiedKey={copiedKey} onCopy={handleCopy} />;
      case 'operations': return <OperationalGuidance key={block} article={activeArticle} />;
      case 'boundaries': return <Boundaries key={block} article={activeArticle} />;
      case 'sources': return <Sources key={block} article={activeArticle} />;
    }
  };

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto grid w-full max-w-[1440px] gap-8 px-4 py-6 sm:px-6 lg:grid-cols-[17rem_minmax(0,1fr)] xl:grid-cols-[17rem_minmax(0,1fr)_11rem] lg:px-8">
        <DocumentationSidebar
          activeArticleID={activeArticle.id}
          query={query}
          searchResults={searchResults}
          onQueryChange={handleQueryChange}
          onSelectArticle={handleSelectArticle}
          onSelectSearchResult={handleSelectSearchResult}
        />
        <main className="min-w-0 pb-16" aria-labelledby="documentation-article-title">
          <article className="mx-auto max-w-3xl">
            <ArticleHeader section={activeSection} article={activeArticle} />
            <div className="mt-9">{visibleBlocks.map(renderBlock)}</div>
            <ArticleNavigation articleID={activeArticle.id} onSelect={handleSelectArticle} />
          </article>
        </main>
        <OnThisPage items={tocItems} />
      </div>
    </div>
  );
}

export function scrollDocumentationViewport(hash: string) {
  if (typeof window === 'undefined') return;
  const schedule = window.requestAnimationFrame || ((callback: FrameRequestCallback) => window.setTimeout(callback, 0));
  schedule(() => {
    if (hash) {
      const targetID = decodeHashTarget(hash);
      const target = targetID ? document.getElementById(targetID) : null;
      if (target && typeof target.scrollIntoView === 'function') {
        try {
          target.scrollIntoView({ block: 'start' });
          return;
        } catch {
          // Fall back to top-level scroll when a test or browser environment lacks smooth element scrolling.
        }
      }
    }
    if (typeof window.scrollTo === 'function') {
      try {
        window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
      } catch {
        // jsdom exposes scrollTo but does not implement it.
      }
    }
  });
}

function decodeHashTarget(hash: string) {
  try {
    return decodeURIComponent(hash.replace(/^#/, ''));
  } catch {
    return hash.replace(/^#/, '');
  }
}
