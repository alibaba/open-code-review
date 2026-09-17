// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const PAGE_SIZE = 10;
    const NEIGHBOURS = 2;

    const table = document.getElementById("sessions-table");
    const pager = document.getElementById("sessions-pagination");
    const numbers = document.getElementById("sessions-page-numbers");
    if (!table || !pager || !numbers) return;

    const rows = Array.from(table.querySelectorAll("tbody tr"));
    const pageCount = Math.ceil(rows.length / PAGE_SIZE);
    if (pageCount < 2) return;

    const steps = Array.from(pager.querySelectorAll("[data-page-step]"));
    let current = 1;

    const pageList = () => {
        if (pageCount <= 5) {
            return Array.from({ length: pageCount }, (_, index) => index + 1);
        }
        const wanted = [1, pageCount];
        for (let page = current - NEIGHBOURS; page <= current + NEIGHBOURS; page++) {
            wanted.push(page);
        }
        const pages = wanted
            .filter((page) => page >= 1 && page <= pageCount)
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

    const show = (page) => {
        current = Math.min(Math.max(page, 1), pageCount);
        rows.forEach((row, index) => {
            row.hidden = Math.floor(index / PAGE_SIZE) + 1 !== current;
        });
        const hadFocus = numbers.contains(document.activeElement);
        numbers.replaceChildren();
        for (const item of pageList()) {
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
            if (item === current) {
                button.setAttribute("aria-current", "page");
            }
            button.addEventListener("click", () => show(item));
            numbers.append(button);
        }
        if (hadFocus) {
            const active = numbers.querySelector('[aria-current="page"]');
            if (active) active.focus({ preventScroll: true });
        }
        const focused = steps.find((step) => document.activeElement === step);
        const states = steps.map((step) => {
            const delta = Number(step.dataset.pageStep);
            return { step, disabled: delta < 0 ? current === 1 : current === pageCount };
        });
        for (const state of states) {
            state.step.disabled = state.disabled;
        }
        if (focused && focused.disabled) {
            const fallback = steps.find((step) => !step.disabled);
            if (fallback) fallback.focus({ preventScroll: true });
        }
    };

    for (const step of steps) {
        step.addEventListener("click", () => show(current + Number(step.dataset.pageStep)));
    }

    pager.hidden = false;
    show(current);
})();
