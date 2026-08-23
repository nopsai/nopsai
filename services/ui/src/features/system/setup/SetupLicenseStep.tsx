import { useEffect, useState } from 'react';
import { ScrollText } from 'lucide-react';
import { acceptSetupLicense, fetchSetupLicense } from './api';
import type { SetupLicenseDocument } from './model';
import { StepIntro, WarningCallout } from './SetupWizardPrimitives';

type SetupLicenseStepProps = {
  canManage: boolean;
  onAccepted: () => void;
};

/**
 * The acceptance step. Possession of a NopsAI artifact grants no right to use
 * it, so this is where an administrator agrees to the terms that do. The full
 * notice is shown rather than linked, because acceptance of terms nobody was
 * shown is worth very little.
 */
export function SetupLicenseStep({ canManage, onAccepted }: SetupLicenseStepProps) {
  const [document, setDocument] = useState<SetupLicenseDocument | null>(null);
  const [checked, setChecked] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    fetchSetupLicense()
      .then(loaded => {
        if (!active) return;
        setDocument(loaded);
        setChecked(loaded.accepted);
        if (loaded.accepted) onAccepted();
      })
      .catch((loadError: unknown) => {
        if (!active) return;
        setError(loadError instanceof Error ? loadError.message : 'Failed to load the licence notice.');
      });
    return () => {
      active = false;
    };
  }, [onAccepted]);

  const accept = async () => {
    if (!document) return;
    setSaving(true);
    setError('');
    try {
      const updated = await acceptSetupLicense(document.document_sha256);
      setDocument(updated);
      onAccepted();
    } catch (acceptError: unknown) {
      setError(acceptError instanceof Error ? acceptError.message : 'Failed to record licence acceptance.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-4">
      <StepIntro title="Licence acceptance" icon={<ScrollText className="h-4 w-4" />}>
        NopsAI is proprietary software. Having a copy of the software, an image, or a chart does not by
        itself grant a right to use it. An administrator must accept the notice below before setup can
        complete.
      </StepIntro>

      {document?.reacceptance_required && (
        <WarningCallout>
          This installation previously accepted notice version {document.accepted_version}. The notice has
          changed since then and must be accepted again before setup can complete.
        </WarningCallout>
      )}

      {error && (
        <div
          role="alert"
          className="rounded-md border border-red-500/30 bg-red-500/10 p-3 text-sm leading-6 text-red-700 dark:text-red-300"
        >
          {error}
        </div>
      )}

      {document ? (
        <>
          <div
            tabIndex={0}
            aria-label="NopsAI proprietary software notice"
            className="max-h-80 overflow-auto rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4"
          >
            <pre className="whitespace-pre-wrap font-mono text-sm leading-6 text-[var(--text-primary)]">
              {document.text}
            </pre>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-[var(--text-secondary)]">
            <span>
              Notice version {document.document_version} · SHA-256 {document.document_sha256.slice(0, 12)}…
            </span>
            {document.accepted && document.accepted_at && (
              <span>
                Accepted {document.accepted_at}
                {document.accepted_by ? ` by ${document.accepted_by}` : ''}
              </span>
            )}
          </div>

          {!document.accepted && (
            <div className="space-y-3">
              <label className="flex items-start gap-2 text-sm leading-6 text-[var(--text-primary)]">
                <input
                  type="checkbox"
                  className="mt-1 h-4 w-4"
                  checked={checked}
                  disabled={!canManage || saving}
                  onChange={event => setChecked(event.target.checked)}
                />
                <span>
                  I have read the notice above and I accept it on behalf of this organisation.
                </span>
              </label>
              <button
                type="button"
                className="glass-button-primary"
                disabled={!checked || !canManage || saving}
                onClick={() => void accept()}
              >
                {saving ? 'Recording acceptance…' : 'Accept and continue'}
              </button>
              {!canManage && (
                <p className="text-sm leading-6 text-[var(--text-secondary)]">
                  Your account cannot change system configuration, so it cannot accept the licence.
                </p>
              )}
            </div>
          )}
        </>
      ) : (
        !error && <p className="text-sm leading-6 text-[var(--text-secondary)]">Loading the licence notice…</p>
      )}
    </div>
  );
}
