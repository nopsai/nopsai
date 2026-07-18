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
    if (scrollDocumentationContainerToTop()) return;
    scrollWindowToTop();
  });
}

function scrollDocumentationContainerToTop() {
  const wrapper = document.getElementById('page-content-wrapper') as HTMLElement | null;
  if (!wrapper) return false;
  if (typeof wrapper.scrollTo === 'function') {
    try {
      wrapper.scrollTo({ top: 0, left: 0, behavior: 'auto' });
      return true;
    } catch {
      // Fall back to direct offsets for test environments or older browsers.
    }
  }
  wrapper.scrollTop = 0;
  wrapper.scrollLeft = 0;
  return true;
}

function scrollWindowToTop() {
  if (typeof window.scrollTo !== 'function') return;
  try {
    window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
  } catch {
    // jsdom exposes scrollTo but does not implement it.
  }
}

function decodeHashTarget(hash: string) {
  try {
    return decodeURIComponent(hash.replace(/^#/, ''));
  } catch {
    return hash.replace(/^#/, '');
  }
}
