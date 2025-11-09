(function (global) {
    global = global || (window.NopsAI = window.NopsAI || {});
    global.yaml = global.yaml || {};

    const defaultEscape = (value) => String(value || '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');

    function renderLines(yamlString, escapeFn) {
        const escapeHtml = typeof escapeFn === 'function' ? escapeFn : defaultEscape;
        const safeYaml = yamlString == null ? '' : String(yamlString);
        const lines = safeYaml.split('\n');
        return lines.map((line, idx) => buildLineHtml(line, idx, escapeHtml)).join('');
    }

    function renderTokens(yamlString, escapeFn) {
        const escapeHtml = typeof escapeFn === 'function' ? escapeFn : defaultEscape;
        const safeYaml = yamlString == null ? '' : String(yamlString);
        const lines = safeYaml.split('\n');
        return lines.map(line => formatLine(line ?? '', escapeHtml)).join('\n');
    }

    function buildLineHtml(line, index, escapeHtml) {
        const content = formatLine(line ?? '', escapeHtml);
        return `<div class="yaml-line"><span class="yaml-line-number">${index + 1}</span><span class="yaml-line-text">${content}</span></div>`;
    }

    function formatLine(line, escapeHtml) {
        if (line === '') {
            return '&nbsp;';
        }
        let html = '';
        let working = line;

        const indentMatch = working.match(/^\s+/);
        if (indentMatch) {
            html += wrapToken(indentMatch[0], 'yaml-token yaml-token--indent', escapeHtml);
            working = working.slice(indentMatch[0].length);
        }

        const commentIndex = findCommentIndex(working);
        let commentText = '';
        if (commentIndex !== -1) {
            commentText = working.slice(commentIndex);
            working = working.slice(0, commentIndex);
        }

        if (working) {
            html += formatCode(working, escapeHtml);
        }

        if (commentText) {
            html += wrapToken(commentText, 'yaml-token yaml-token--comment', escapeHtml);
        }

        return html || '&nbsp;';
    }

    function formatCode(text, escapeHtml) {
        if (!text) return '';
        let html = '';
        let working = text;

        const dashMatch = working.match(/^(-\s+)/);
        if (dashMatch) {
            html += wrapToken(dashMatch[0], 'yaml-token yaml-token--dash', escapeHtml);
            working = working.slice(dashMatch[0].length);
        }

        const keyMatch = working.match(/^([A-Za-z0-9_.-]+)(\s*):(.*)$/);
        if (keyMatch) {
            html += wrapToken(keyMatch[1], 'yaml-token yaml-token--key', escapeHtml);
            const colonPart = `${keyMatch[2] || ''}:`;
            html += wrapToken(colonPart, 'yaml-token yaml-token--punctuation', escapeHtml);
            const rawValue = keyMatch[3] || '';
            if (rawValue) {
                const whitespaceMatch = rawValue.match(/^\s+/);
                if (whitespaceMatch) {
                    html += wrapToken(whitespaceMatch[0], 'yaml-token yaml-token--ws', escapeHtml);
                }
                const valuePortion = rawValue.slice(whitespaceMatch ? whitespaceMatch[0].length : 0);
                if (valuePortion) {
                    html += formatValue(valuePortion, escapeHtml);
                }
            }
        } else if (working.trim()) {
            html += formatValue(working, escapeHtml);
        } else {
            html += wrapToken(working, 'yaml-token yaml-token--ws', escapeHtml);
        }

        return html;
    }

    function formatValue(rawValue, escapeHtml) {
        if (!rawValue) return '';
        const trimmed = rawValue.trim();
        if (!trimmed) {
            return wrapToken(rawValue, 'yaml-token yaml-token--value', escapeHtml);
        }

        let className = 'yaml-token yaml-token--scalar';
        if (/^['"].*['"]$/.test(trimmed)) {
            className = 'yaml-token yaml-token--string';
        } else if (/^(true|false|null)$/i.test(trimmed)) {
            className = 'yaml-token yaml-token--boolean';
        } else if (/^-?\d+(?:\.\d+)?$/.test(trimmed)) {
            className = 'yaml-token yaml-token--number';
        } else if (/^[>|]/.test(trimmed)) {
            className = 'yaml-token yaml-token--operator';
        } else if (/^!!/.test(trimmed)) {
            className = 'yaml-token yaml-token--type';
        }

        return wrapToken(rawValue, className, escapeHtml);
    }

    function wrapToken(text, className, escapeHtml) {
        if (!text) return '';
        const safe = escapeHtml(text);
        if (!className) {
            return safe;
        }
        return `<span class="${className}">${safe}</span>`;
    }

    function findCommentIndex(text) {
        if (!text) return -1;
        let inSingle = false;
        let inDouble = false;
        for (let i = 0; i < text.length; i += 1) {
            const char = text[i];
            if (char === '"' && !inSingle) {
                const escaped = i > 0 && text[i - 1] === '\\';
                if (!escaped) inDouble = !inDouble;
            } else if (char === '\'' && !inDouble) {
                const escaped = i > 0 && text[i - 1] === '\\';
                if (!escaped) inSingle = !inSingle;
            } else if (char === '#' && !inSingle && !inDouble) {
                return i;
            }
        }
        return -1;
    }

    global.yaml.renderLines = renderLines;
    global.yaml.renderTokens = renderTokens;
})(window.NopsAI = window.NopsAI || {});
