// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const PAGE_SIZE = 10;
    const NEIGHBOURS = 2;

    const input = document.getElementById("repository-search-input");
    const table = document.getElementById("repositories-table");
    const pager = document.getElementById("repos-pagination");
    const numbers = document.getElementById("repos-page-numbers");
    if (!input || !table || !pager || !numbers) return;

    const rows = Array.from(table.querySelectorAll("tbody tr"));
    const steps = Array.from(pager.querySelectorAll("[data-page-step]"));
    let query = "";
    let current = 1;

    // A row is visible when it matches the search query and sits on the
    // current page of the filtered list, so both controls write through the
    // single render pass below.
    const matches = (row) => {
        if (!query) return true;
        const cell = row.querySelector("[data-repository-name]");
        return cell ? cell.textContent.trim().toLowerCase().includes(query) : false;
    };

    const pageList = (total) => {
        // Pages [1, total] plus a window of NEIGHBOURS pages around the
        // current one; a gap wider than 2 collapses into an ellipsis.
        if (total <= 5) {
            return Array.from({ length: total }, (_, index) => index + 1);
        }
        const wanted = [1, total];
        for (let page = current - NEIGHBOURS; page <= current + NEIGHBOURS; page++) {
            wanted.push(page);
        }
        const pages = wanted
            .filter((page) => page >= 1 && page <= total)
            .filter((page, index, all) => all.indexOf(page) === index)
            .sort((a, b) => a - b);
        const items = [];
        let previous = 0;
        for (const page of pages) {
            if (page - previous > 2) items.push(null);
            if (page - previous === 2) items.push(page - 1);
            items.push(page);
            previous = page;
        }
        return items;
    };

    const render = (page) => {
        if (page !== undefined) {
            current = page;
        }
        // Filter once; both the page count and the visibility loop read the
        // same filtered list.
        const filtered = rows.filter(matches);
        const total = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
        current = Math.min(Math.max(current, 1), total);
        for (const row of rows) {
            row.hidden = true;
        }
        for (const row of filtered.slice((current - 1) * PAGE_SIZE, current * PAGE_SIZE)) {
            row.hidden = false;
        }

        const hadFocus = numbers.contains(document.activeElement);
        numbers.replaceChildren();
        for (const item of pageList(total)) {
            if (item === null) {
                const gap = document.createElement("span");
                gap.className = "page-gap";
                gap.setAttribute("aria-hidden", "true");
                gap.textContent = "…";
                numbers.append(gap);
                continue;
            }
            const button = document.createElement("button");
            button.type = "button";
            button.className = "page-number";
            button.textContent = String(item);
            button.setAttribute("aria-label", `Page ${item}`);
            if (item === current) {
                button.setAttribute("aria-current", "page");
            }
            button.addEventListener("click", () => render(item));
            numbers.append(button);
        }
        if (hadFocus) {
            const active = numbers.querySelector('[aria-current="page"]');
            if (active) active.focus({ preventScroll: true });
        }

        const focused = steps.find((step) => document.activeElement === step);
        for (const step of steps) {
            const delta = Number(step.dataset.pageStep);
            step.disabled = delta < 0 ? current === 1 : current === total;
        }
        if (focused && focused.disabled) {
            const fallback = steps.find((step) => !step.disabled);
            if (fallback) fallback.focus({ preventScroll: true });
        }

        pager.hidden = total < 2;
    };

    input.addEventListener("input", () => {
        query = input.value.trim().toLowerCase();
        current = 1;
        render();
    });

    for (const step of steps) {
        step.addEventListener("click", () => render(current + Number(step.dataset.pageStep)));
    }

    render();
})();
