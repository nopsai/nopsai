import { useEffect, useState } from 'react';
import { apiClient } from '../lib/api';
import {
  appVersionFooterText,
  normalizePlatformVersionInfo,
  type PlatformVersionInfo,
} from './platformVersion';

function AppFooter() {
  const [versionInfo, setVersionInfo] = useState<PlatformVersionInfo | null>(null);

  useEffect(() => {
    let active = true;
    void apiClient.fetch('/version', { auth: false, cache: 'no-store' })
      .then(async response => {
        if (!response.ok) return null;
        return normalizePlatformVersionInfo(await response.json().catch(() => null));
      })
      .then(nextVersionInfo => {
        if (active) setVersionInfo(nextVersionInfo);
      })
      .catch(() => {
        if (active) setVersionInfo(null);
      });
    return () => {
      active = false;
    };
  }, []);

  const footerText = appVersionFooterText(versionInfo);
  if (!footerText) return null;

  return (
    <footer className="app-footer-shell flex-shrink-0 px-6 py-2 text-right text-xs text-[var(--text-tertiary)]" aria-label="Application version">
      <span>{footerText}</span>
    </footer>
  );
}

export default AppFooter;
