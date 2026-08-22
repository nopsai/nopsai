import { Trash2 } from 'lucide-react';
import { AccessModal } from './AccessModal';

type AccessConfirmationDialogProps = {
  message: string;
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

export function AccessConfirmationDialog({ message, pending, onCancel, onConfirm }: AccessConfirmationDialogProps) {
  return (
    <AccessModal
      kicker="Confirm"
      title="Please confirm"
      subtitle="This action cannot be undone."
      onClose={onCancel}
      icon={<Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />}
      variant="minimal"
      actions={
        <>
          <button data-dialog-initial-focus type="button" className="glass-button-ghost" onClick={onCancel} disabled={pending}>
            Cancel
          </button>
          <button type="button" className="glass-button-danger" onClick={() => void onConfirm()} disabled={pending}>
            {pending ? 'Working…' : 'Delete'}
          </button>
        </>
      }
    >
      <p className="text-sm text-[var(--text-primary)]">{message}</p>
    </AccessModal>
  );
}
