// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

document.querySelectorAll('.response-text').forEach(function(el) {
    const text = el.textContent;
    const esc = function(s) {
        return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    };
    const codeBlocks = [];
    let html = esc(text);
    html = html.replace(/
    html = html
        .replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>')
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/^### (.+)$/gm, '<div class="md-h3">$1</div>')
        .replace(/^## (.+)$/gm, '<div class="md-h2">$1</div>')
        .replace(/^# (.+)$/gm, '<div class="md-h1">$1</div>')
        .replace(/^[-*] (.+)$/gm, '<div class="md-li">&bull; $1</div>')
        .replace(/\n{2,}/g, '<br><br>')
        .replace(/\n/g, '<br>');
    codeBlocks.forEach(function(code, i) {
        html = html.replace('%%CODEBLOCK_' + i + '%%',
            '<pre class="code-block"><code>' + code + '</code></pre>');
    });
    el.innerHTML = html;
});
