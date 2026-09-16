// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(function (root) {
    "use strict";

    const PAGE_SIZE = 12;

    function matchesRepositoryName(name, query) {
        const q = String(query || "").trim().toLowerCase();
        const n = String(name || "").trim().toLowerCase();
        return n.indexOf(q) !== -1;
    }

    function rowRepositoryName(row) {
        const nameCell = row.querySelector("[data-repository-name]");
        return nameCell ? String(nameCell.textContent || "").trim() : "";
    }

    function pageCount(itemCount, pageSize) {
        const size = pageSize || PAGE_SIZE;
        if (itemCount <= 0) {
            return 0;
        }
        return Math.ceil(itemCount / size);
    }

    function clampPage(page, totalPages) {
        if (totalPages <= 0) {
            return 1;
        }
        if (page < 1) {
            return 1;
        }
        if (page > totalPages) {
            return totalPages;
        }
        return page;
    }

    // Numbered items for the pager, matching the mockup's compact window
    // (e.g. page 2 of 10 -> 1 2 3 4 … 10). Prev/next are rendered separately.
    function paginationItems(totalPages, currentPage) {
        const current = clampPage(currentPage, totalPages);
        const items = [];
        let start;
        let end;
        let p;
        if (totalPages <= 0) {
            return items;
        }
        function addPage(n) {
            items.push({ type: "page", page: n });
        }
        if (totalPages <= 7) {
            for (p = 1; p <= totalPages; p++) {
                addPage(p);
            }
            return items;
        }
        addPage(1);
        if (current <= 3) {
            start = 2;
            end = 4;
        } else if (current >= totalPages - 2) {
            start = totalPages - 3;
            end = totalPages - 1;
        } else {
            start = current - 1;
            end = current + 1;
        }
        if (start > 2) {
            items.push({ type: "ellipsis" });
        }
        for (p = start; p <= end; p++) {
            addPage(p);
        }
        if (end < totalPages - 1) {
            items.push({ type: "ellipsis" });
        }
        addPage(totalPages);
        return items;
    }

    function applyFilterAndPage(rows, query, page, pageSize) {
        const size = pageSize || PAGE_SIZE;
        const list = Array.prototype.slice.call(rows);
        const matched = [];
        let i;
        for (i = 0; i < list.length; i++) {
            if (matchesRepositoryName(rowRepositoryName(list[i]), query)) {
                matched.push(list[i]);
            }
        }
        const totalPages = pageCount(matched.length, size);
        const current = clampPage(page, totalPages);
        const start = totalPages === 0 ? 0 : (current - 1) * size;
        const pageRows = matched.slice(start, start + size);
        for (i = 0; i < list.length; i++) {
            list[i].hidden = true;
        }
        for (i = 0; i < pageRows.length; i++) {
            pageRows[i].hidden = false;
        }
        return {
            matchedCount: matched.length,
            page: current,
            totalPages: totalPages,
            items: paginationItems(totalPages, current)
        };
    }

    function createElement(nav, tag) {
        const doc = nav && nav.ownerDocument;
        if (doc && typeof doc.createElement === "function") {
            return doc.createElement(tag);
        }
        return null;
    }

    function renderPagination(nav, state, onPage) {
        if (!nav) {
            return;
        }
        while (nav.firstChild) {
            nav.removeChild(nav.firstChild);
        }
        if (!state || state.totalPages <= 1) {
            nav.hidden = true;
            return;
        }
        nav.hidden = false;

        function addButton(label, page, opts) {
            const btn = createElement(nav, "button");
            if (!btn) {
                return;
            }
            btn.type = "button";
            btn.className = "repos-page-btn";
            btn.textContent = label;
            if (opts && opts.current) {
                btn.className += " is-current";
                if (btn.setAttribute) {
                    btn.setAttribute("aria-current", "page");
                }
            }
            if (opts && opts.disabled) {
                btn.disabled = true;
            }
            if (opts && opts.label && btn.setAttribute) {
                btn.setAttribute("aria-label", opts.label);
            }
            if (typeof btn.addEventListener === "function") {
                btn.addEventListener("click", function () {
                    if (!btn.disabled) {
                        onPage(page);
                    }
                });
            }
            nav.appendChild(btn);
        }

        addButton("\u2039", state.page - 1, {
            disabled: state.page <= 1,
            label: "Previous page"
        });
        let i;
        let item;
        for (i = 0; i < state.items.length; i++) {
            item = state.items[i];
            if (item.type === "ellipsis") {
                const span = createElement(nav, "span");
                if (span) {
                    span.className = "repos-page-ellipsis";
                    span.textContent = "\u2026";
                    if (span.setAttribute) {
                        span.setAttribute("aria-hidden", "true");
                    }
                    nav.appendChild(span);
                }
                continue;
            }
            addButton(String(item.page), item.page, {
                current: item.page === state.page
            });
        }
        addButton("\u203A", state.page + 1, {
            disabled: state.page >= state.totalPages,
            label: "Next page"
        });
    }

    function bindReposPage(doc, pageSize) {
        const documentRef = doc || (typeof document !== "undefined" ? document : null);
        if (!documentRef || typeof documentRef.getElementById !== "function") {
            return null;
        }
        const input = documentRef.getElementById("repository-search-input");
        const table = documentRef.getElementById("repositories-table");
        const nav = documentRef.getElementById("repositories-pagination");
        if (!input || !table) {
            return null;
        }
        const rows = table.querySelectorAll("tbody tr");
        let page = 1;
        const size = pageSize || PAGE_SIZE;

        function refresh() {
            const state = applyFilterAndPage(rows, input.value, page, size);
            page = state.page;
            renderPagination(nav, state, function (next) {
                page = next;
                refresh();
            });
            return state;
        }

        if (typeof input.addEventListener === "function") {
            input.addEventListener("input", function () {
                page = 1;
                refresh();
            });
        }

        refresh();
        return {
            refresh: refresh,
            setPage: function (p) {
                page = p;
                return refresh();
            },
            getPage: function () {
                return page;
            }
        };
    }

    const api = {
        PAGE_SIZE: PAGE_SIZE,
        matchesRepositoryName: matchesRepositoryName,
        rowRepositoryName: rowRepositoryName,
        pageCount: pageCount,
        clampPage: clampPage,
        paginationItems: paginationItems,
        applyFilterAndPage: applyFilterAndPage,
        renderPagination: renderPagination,
        bindReposPage: bindReposPage
    };

    root.ocrRepos = api;

    if (typeof document !== "undefined") {
        bindReposPage(document);
    }
})(typeof globalThis !== "undefined" ? globalThis : this);
