import type { ReactNode } from "react";
import { useDialogFocus } from "../../../components/useDialogFocus";

type AccessEditorDrawerProps = {
  open: boolean;
  label: string;
  children: ReactNode;
  onClose: () => void;
};

export function AccessEditorDrawer({
  open,
  label,
  children,
  onClose,
}: AccessEditorDrawerProps) {
  const dialogRef = useDialogFocus(onClose, open);

  if (!open) return null;

  return (
    <>
      <button
        type="button"
        className="access-editor-backdrop"
        aria-label={`Close ${label}`}
        onClick={onClose}
      />
      <div
        ref={dialogRef}
        className="access-editor-pane access-editor-pane--drawer"
        role="dialog"
        aria-modal="true"
        aria-label={label}
        tabIndex={-1}
      >
        {children}
      </div>
    </>
  );
}
