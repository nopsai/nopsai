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
    >
      <div className="space-y-4">
        <p className="text-sm text-[var(--text-primary)]">{message}</p>
        <div className="flex items-center justify-end gap-2">
          <button data-dialog-initial-focus type="button" className="access-inline-btn" onClick={onCancel} disabled={pending}>
            Cancel
          </button>
          <button type="button" className="glass-button-danger" onClick={() => void onConfirm()} disabled={pending}>
            {pending ? 'Working…' : 'Delete'}
          </button>
        </div>
      </div>
    </AccessModal>
  );
}
