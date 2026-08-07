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
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, function(_, lang, code) {
        codeBlocks.push(code.replace(/^\n|\n$/g, ''));
        return '%%CODEBLOCK_' + (codeBlocks.length - 1) + '%%';
    });
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

(function() {
    const filters = Array.from(document.querySelectorAll('.comment-filter-chip[data-filter-kind]'));
    const groups = Array.from(document.querySelectorAll('.comment-file-group'));
    const emptyState = document.querySelector('[data-comment-filter-empty]');

    if (filters.length === 0 || groups.length === 0) {
        return;
    }

    let activeKind = 'all';
    let activeValue = '';

    function cardMatches(card) {
        if (activeKind === 'all') {
            return true;
        }
        return card.dataset[activeKind] === activeValue;
    }

    function updateFilterState() {
        filters.forEach(function(filter) {
            const isActive = activeKind === filter.dataset.filterKind &&
                activeValue === filter.dataset.filterValue;
            filter.classList.toggle('is-active', isActive);
            filter.setAttribute('aria-pressed', String(isActive));
        });

        let visibleCount = 0;
        groups.forEach(function(group) {
            const cards = Array.from(group.querySelectorAll('[data-comment-card]'));
            let groupVisibleCount = 0;
            cards.forEach(function(card) {
                const visible = cardMatches(card);
                card.hidden = !visible;
                if (visible) {
                    groupVisibleCount++;
                    visibleCount++;
                }
            });
            group.hidden = groupVisibleCount === 0;
            const count = group.querySelector('[data-comment-count]');
            if (count) {
                count.textContent = groupVisibleCount + ' comment' + (groupVisibleCount === 1 ? '' : 's');
            }
        });

        if (emptyState) {
            emptyState.hidden = visibleCount !== 0;
        }
    }

    filters.forEach(function(filter) {
        filter.addEventListener('click', function() {
            const kind = filter.dataset.filterKind;
            const value = filter.dataset.filterValue || '';
            if (kind === 'all' || (activeKind === kind && activeValue === value)) {
                activeKind = 'all';
                activeValue = '';
            } else {
                activeKind = kind;
                activeValue = value;
            }
            updateFilterState();
        });
    });

    updateFilterState();
})();
