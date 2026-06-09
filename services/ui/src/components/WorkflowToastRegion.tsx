export type WorkflowToast = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

export function WorkflowToastRegion({
  toasts,
  classPrefix = 'pipelines',
}: {
  toasts: WorkflowToast[];
  classPrefix?: 'pipelines' | 'triggers';
}) {
  if (!toasts.length) return null;

  return (
    <div
      className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3"
      aria-label="Notifications"
      aria-live="polite"
    >
      {toasts.map(toast => (
        <div
          key={toast.id}
          className={`${classPrefix}-toast ${classPrefix}-toast--${toast.tone} show`}
          role={toast.tone === 'error' ? 'alert' : 'status'}
        >
          <div className={`${classPrefix}-toast__content`}>{toast.message}</div>
        </div>
      ))}
    </div>
  );
}
