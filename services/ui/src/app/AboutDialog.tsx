import { X } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import type { PlatformVersionInfo } from './platformVersion';

/**
 * The licence notice shipped with the product.
 *
 * This is the same text `nopsai license` prints, and a Go test
 * (contract/about_dialog_license_test.go) fails if the two drift apart. It is quoted
 * rather than paraphrased because it is a legal notice: nothing here is drafted
 * by the UI.
 */
export const licenseNotice = `NopsAI Licence

Copyright (c) 2026 Hossein Yousefi. All rights reserved.

NopsAI is licensed under the PolyForm Noncommercial License 1.0.0. It is free
for any noncommercial purpose: personal use, study, research, experimentation,
hobby projects, and use by charitable organizations, educational institutions,
public research organizations, public safety or health organizations,
environmental protection organizations and government institutions.

Commercial use is not granted by this licence. Using NopsAI in or for a
business, or for any other commercial purpose, requires a separate
written agreement.

Third-party components remain subject to their applicable licence terms. See the
LICENSE and THIRD_PARTY_NOTICES.md files supplied with the NopsAI release.

Commercial licensing enquiries: contact@nopsai.com`;

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
            <pre className="about-dialog-notice">{licenseNotice}</pre>
          </section>

          <section aria-label="Policies">
            <h3>Policies</h3>
            <p className="about-dialog-note">
              Security and vulnerability disclosure policy:{' '}
              <a href="https://nopsai.com/security/" target="_blank" rel="noreferrer noopener">nopsai.com/security</a>.
              Terms of use and the privacy notice are published on the NopsAI website. NopsAI is free for
              any non-commercial purpose under the licence quoted above; commercial use requires a separate
              written agreement, which governs the deployment rather than this dialog.
            </p>
          </section>
        </div>
      </div>
    </div>,
    document.body
  );
}

export default AboutDialog;
