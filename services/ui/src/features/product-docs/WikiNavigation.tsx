import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import { getWikiNeighbors, wikiArticlePath } from './content/index.js';

/** Previous and next page across section boundaries, so the wiki reads as one book. */
export function WikiPager({ articleID }: { articleID: string }) {
  const { previous, next } = getWikiNeighbors(articleID);
  if (!previous && !next) return null;
  return (
    <nav aria-label="Wiki pagination" className="docs-pager">
      {previous ? (
        <Link to={wikiArticlePath(previous.section.id, previous.article.id)} className="docs-pager-link">
          <span className="docs-pager-label">
            <ChevronLeft className="h-3 w-3" aria-hidden="true" />
            Previous
          </span>
          <span className="docs-pager-title">{previous.article.title}</span>
        </Link>
      ) : (
        <span />
      )}
      {next ? (
        <Link to={wikiArticlePath(next.section.id, next.article.id)} className="docs-pager-link docs-pager-link--next">
          <span className="docs-pager-label">
            Next
            <ChevronRight className="h-3 w-3" aria-hidden="true" />
          </span>
          <span className="docs-pager-title">{next.article.title}</span>
        </Link>
      ) : null}
    </nav>
  );
}
