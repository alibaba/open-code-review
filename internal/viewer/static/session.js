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
    const hideMarkedToggle = document.querySelector('[data-hide-marked]');
    const marksCount = document.querySelector('[data-marks-count]');
    const clearAllButton = document.querySelector('[data-clear-all-marks]');

    if (filters.length === 0 || groups.length === 0) {
        return;
    }

    let activeSeverity = 'all';
    let activeCategory = 'all';

    // localStorage throws when storage is unavailable (private browsing with
    // quota exceeded, storage disabled by policy); the toggle then just uses
    // its default instead of taking the whole feature down.
    function storedHideMarked() {
        try {
            return (localStorage.getItem('ocr-viewer-hide-marked') || '1') === '1';
        } catch (_) {
            return true;
        }
    }

    function storeHideMarked(value) {
        try {
            localStorage.setItem('ocr-viewer-hide-marked', value ? '1' : '0');
        } catch (_) {
            // Nothing to persist without storage; the in-page toggle still works.
        }
    }

    // Hide-marked is presentation, not data: the default (on) matches the
    // mark-and-hide workflow, and the choice persists per browser only.
    let hideMarked = hideMarkedToggle ? storedHideMarked() : false;

    function matchesFilters(card) {
        return (activeSeverity === 'all' || card.dataset.severity === activeSeverity) &&
            (activeCategory === 'all' || card.dataset.category === activeCategory);
    }

    function cardMatches(card) {
        const marked = Boolean(card.dataset.mark);
        return (!hideMarked || !marked) && matchesFilters(card);
    }

    function updateFilterState() {
        filters.forEach(function(filter) {
            const kind = filter.dataset.filterKind;
            const activeValue = kind === 'severity' ? activeSeverity : activeCategory;
            const isActive = activeValue === filter.dataset.filterValue;
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

        // Count mark-hidden separately from filter-hidden so the toolbar
        // attributes each hidden card to its cause: "hidden" means hidden by
        // a mark, never crowded out by the severity/category filters.
        let markedCount = 0;
        let hiddenByMarks = 0;
        Array.from(document.querySelectorAll('[data-comment-card]')).forEach(function(card) {
            if (!card.dataset.mark) {
                return;
            }
            markedCount++;
            if (hideMarked && matchesFilters(card)) {
                hiddenByMarks++;
            }
        });

        if (emptyState) {
            emptyState.hidden = visibleCount !== 0;
            emptyState.textContent = visibleCount === 0 && hiddenByMarks > 0
                ? 'All matching comments are hidden by marks.'
                : 'No comments match this filter.';
        }

        if (marksCount) {
            marksCount.textContent = markedCount + ' marked, ' + hiddenByMarks + ' hidden' +
                (marksSaveFailed ? ' — not saved (storage unavailable)' : '');
        }
    }

    filters.forEach(function(filter) {
        filter.addEventListener('click', function() {
            const kind = filter.dataset.filterKind;
            const value = filter.dataset.filterValue;
            if (kind === 'severity') {
                activeSeverity = activeSeverity === value ? 'all' : value;
            } else {
                activeCategory = activeCategory === value ? 'all' : value;
            }
            updateFilterState();
        });
    });

    // Marks: the viewer server is read-only, so marks are presentation state
    // kept in localStorage, keyed by the session page's path — one session,
    // one browser, no writes anywhere near the session JSONL.
    // An array, not an object literal: property lookups on an object literal
    // reach Object.prototype, so "toString" or "constructor" would pass as a
    // mark state. An array has no such surface.
    const knownMarkStates = ['fixed', 'ignored'];

    // Null-prototype maps: ids come back out of localStorage, and a hostile
    // `__proto__` key must replay as inert data, never reach a prototype.
    function emptyMarks() {
        return Object.create(null);
    }

    function marksStorageKey() {
        return 'ocr-viewer-marks:' + window.location.pathname.replace(/\/$/, '');
    }

    function loadStoredMarks() {
        let raw;
        try {
            raw = localStorage.getItem(marksStorageKey());
        } catch (_) {
            return emptyMarks();
        }
        if (!raw) {
            return emptyMarks();
        }
        let parsed;
        try {
            parsed = JSON.parse(raw);
        } catch (_) {
            return emptyMarks();
        }
        const marks = emptyMarks();
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
            Object.keys(parsed).forEach(function(id) {
                if (knownMarkStates.indexOf(parsed[id]) !== -1) {
                    marks[id] = parsed[id];
                }
            });
        }
        return marks;
    }

    // Set by saveStoredMarks, read by updateFilterState. Every caller
    // persists before triggering the next render, which is what makes the
    // warning appear at the moment a mark silently stops persisting.
    let marksSaveFailed = false;

    function saveStoredMarks(marks) {
        try {
            if (Object.keys(marks).length === 0) {
                // An empty key would accumulate one entry per visited session
                // on a fixed origin with no reclaim path — remove it instead.
                localStorage.removeItem(marksStorageKey());
            } else {
                localStorage.setItem(marksStorageKey(), JSON.stringify(marks));
            }
            marksSaveFailed = false;
        } catch (err) {
            // Storage full or disabled: the in-page marks still work, they
            // just won't survive a reload — say so in the toolbar.
            marksSaveFailed = true;
            console.error('[ocr viewer] could not persist marks:', err);
        }
    }

    let marks = loadStoredMarks();

    function applyMarkState(card, state) {
        const chip = card.querySelector('[data-mark-chip]');
        if (knownMarkStates.indexOf(state) !== -1) {
            card.dataset.mark = state;
            if (chip) {
                chip.textContent = state;
                chip.className = 'comment-badge mark-chip mark-' + state;
                chip.hidden = false;
            }
        } else {
            delete card.dataset.mark;
            if (chip) {
                // Reset fully so a chip revealed by anything but this function
                // cannot show a stale mark label or color.
                chip.textContent = '';
                chip.className = 'comment-badge mark-chip';
                chip.hidden = true;
            }
        }
    }

    function setMark(markID, state) {
        if (state) {
            marks[markID] = state;
        } else {
            delete marks[markID];
        }
        saveStoredMarks(marks);
    }

    document.querySelectorAll('[data-set-mark]').forEach(function(button) {
        button.addEventListener('click', function() {
            const card = button.closest('[data-comment-card]');
            if (!card || !card.dataset.markId) {
                return;
            }
            setMark(card.dataset.markId, button.dataset.setMark);
            applyMarkState(card, button.dataset.setMark);
            updateFilterState();
        });
    });

    if (hideMarkedToggle) {
        hideMarkedToggle.checked = hideMarked;
        hideMarkedToggle.addEventListener('change', function() {
            hideMarked = hideMarkedToggle.checked;
            storeHideMarked(hideMarked);
            updateFilterState();
        });
    }

    if (clearAllButton) {
        clearAllButton.addEventListener('click', function() {
            marks = emptyMarks();
            saveStoredMarks(marks);
            document.querySelectorAll('[data-comment-card]').forEach(function(card) {
                applyMarkState(card, '');
            });
            updateFilterState();
        });
    }

    // Replay stored marks before the first filter pass so hidden counts and
    // chip states are correct on load.
    document.querySelectorAll('[data-comment-card]').forEach(function(card) {
        if (card.dataset.markId && marks[card.dataset.markId]) {
            applyMarkState(card, marks[card.dataset.markId]);
        }
    });

    updateFilterState();
})();
