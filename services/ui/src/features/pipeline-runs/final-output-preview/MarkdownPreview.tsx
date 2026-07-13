import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

export function MarkdownPreview({ content }: { content: string }) {
  return (
    <article className="max-w-none space-y-3 text-sm leading-6 text-[var(--text-primary)]">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: props => <h4 className="border-b border-[var(--border-primary)] pb-2 text-2xl font-bold" {...props} />,
          h2: props => <h5 className="mt-5 text-xl font-bold" {...props} />,
          h3: props => <h6 className="mt-4 text-base font-bold" {...props} />,
          p: props => <p className="whitespace-pre-line" {...props} />,
          ul: props => <ul className="list-disc space-y-1 pl-6" {...props} />,
          ol: props => <ol className="list-decimal space-y-1 pl-6" {...props} />,
          a: props => <a className="text-blue-600 underline" target="_blank" rel="noreferrer" {...props} />,
          blockquote: props => <blockquote className="border-l-4 border-blue-300 pl-3 text-[var(--text-secondary)]" {...props} />,
          code: props => <code className="rounded bg-slate-100 px-1 py-0.5 font-mono text-xs dark:bg-slate-800" {...props} />,
          pre: props => <pre className="overflow-auto bg-slate-950 p-3 text-xs text-slate-100 dark:text-[var(--text-primary)]" {...props} />,
          table: props => <table className="min-w-full border-collapse text-left text-xs" {...props} />,
          th: props => <th className="border border-[var(--border-primary)] bg-slate-100 px-3 py-2 font-bold dark:bg-slate-800" {...props} />,
          td: props => <td className="border border-[var(--border-primary)] px-3 py-2 align-top" {...props} />,
        }}
      >
        {content}
      </ReactMarkdown>
    </article>
  );
}
