import { X } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import type { PlatformVersionInfo } from './platformVersion';

/**
 * The proprietary notice shipped with the product.
 *
 * This is the same text `nopsai license` prints, and a Go test
 * (contract/about_dialog_license_test.go) fails if the two drift apart. It is quoted
 * rather than paraphrased because it is a legal notice: nothing here is drafted
 * by the UI.
 */
export const proprietaryLicenseNotice = `NopsAI Proprietary Software Notice

Copyright (c) 2026 Hossein Yousefi. All rights reserved.

NopsAI is proprietary software. Possession of or access to this binary does not
grant a licence to use it. Use is permitted only under a written agreement
signed by Hossein Yousefi or by a successor entity to which the relevant rights
have been assigned.

Third-party components remain subject to their applicable licence terms. See the
LICENSE and THIRD_PARTY_NOTICES.md files supplied with the NopsAI release.

Licensing enquiries: contact@nopsai.com`;

export function AboutDialog({
  versionInfo,
  onClose,
}: {
  versionInfo: PlatformVersionInfo | null;
  onClose: () => void;
}) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    closeRef.current?.focus();
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onClose]);

  const buildRows = versionInfo
    ? ([
        ['Version', versionInfo.productVersion],
        ['Commit', versionInfo.commit],
        ['Built', versionInfo.buildDate],
        ['API', versionInfo.apiVersion],
        ['CLI compatibility', versionInfo.cliCompatibility],
        ['Runner compatibility', versionInfo.runnerCompatibility],
        ['Runner protocol', versionInfo.runnerProtocolVersion],
        ['Release manifest', versionInfo.releaseManifestDigest],
      ] as const).filter(([, value]) => Boolean(value))
    : [];

  // Portalled to the body on purpose. The sidebar that opens this dialog is
  // transformed (it slides in) and clips its overflow, and a transformed
  // ancestor becomes the containing block for position: fixed — which sized the
  // dialog to the sidebar column and cut the text off.
  return createPortal(
    <div
      className="about-dialog-backdrop"
      onMouseDown={event => {
        if (!dialogRef.current?.contains(event.target as Node)) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="about-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="about-dialog-title"
      >
        <div className="about-dialog-head">
          <h2 id="about-dialog-title">About NopsAI</h2>
          <button ref={closeRef} type="button" className="about-dialog-close" onClick={onClose} aria-label="Close about dialog">
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>

        <div className="about-dialog-body">
          <section aria-label="Build">
            <h3>Build</h3>
            {buildRows.length > 0 ? (
              <dl className="about-dialog-facts">
                {buildRows.map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            ) : (
              <p className="about-dialog-note">Build information is unavailable from this server.</p>
            )}
          </section>

          <section aria-label="Licence">
            <h3>Licence</h3>
            <pre className="about-dialog-notice">{proprietaryLicenseNotice}</pre>
          </section>

          <section aria-label="Policies">
            <h3>Policies</h3>
            <p className="about-dialog-note">
              Security and vulnerability disclosure policy:{' '}
              <a href="https://nopsai.com/security/" target="_blank" rel="noreferrer noopener">nopsai.com/security</a>.
              Terms of use and the privacy notice are published on the NopsAI website; the licence that
              governs a deployment is the written agreement signed for it, not this dialog.
            </p>
          </section>
        </div>
      </div>
    </div>,
    document.body
  );
}

export default AboutDialog;
