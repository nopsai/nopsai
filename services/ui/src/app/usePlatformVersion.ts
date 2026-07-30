import { useEffect, useState } from 'react';
import { fetchPlatformVersionInfo } from './platformVersionApi';
import type { PlatformVersionInfo } from './platformVersion';

export function usePlatformVersionInfo() {
  const [versionInfo, setVersionInfo] = useState<PlatformVersionInfo | null>(null);

  useEffect(() => {
    let active = true;
    void fetchPlatformVersionInfo()
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

  return versionInfo;
}
