# NopsAI Brand Assets

Runtime brand assets live in `services/ui/public/brand` so the UI, browser
metadata, repository documentation, and notification templates use one owned
source. Keep the semantic filenames stable when artwork is refreshed.

| Asset | Intended use |
| --- | --- |
| `nopsai-logo-light.png` / `nopsai-logo-dark.png` | Theme-aware horizontal wordmark |
| `nopsai-mark-light.png` / `nopsai-mark-dark.png` | Theme-aware standalone mark |
| `nopsai-logo-vertical-light.png` | Light-background vertical composition |
| `nopsai-app-icon.png` | Favicon, touch icon, and compact notification mark |
| `nopsai-banner-light.png` / `nopsai-banner-dark.png` | Repository and social banner pair |

This directory retains the supplied combined logo reference and alternate
banner compositions. They are documentation assets and are intentionally not
copied into the production UI image.

UI code must render product identity through `src/components/BrandIdentity.tsx`
rather than embedding asset paths in route or shell components. This keeps
light/dark behavior and accessible naming consistent.
