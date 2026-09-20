// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const input = document.getElementById("repository-search-input");
    const table = document.getElementById("repositories-table");
    const pager = document.getElementById("repos-pagination");
    const numbers = document.getElementById("repos-page-numbers");
    if (!input || !table || !pager || !numbers) return;

    let query = "";

    // A row is visible when it matches the search query and sits on the
    // current page of the filtered list; the search writes through the
    // pager's single render pass.
    const matches = (row) => {
        if (!query) return true;
        const cell = row.querySelector("[data-repository-name]");
        return cell ? cell.textContent.trim().toLowerCase().includes(query) : false;
    };

    const pagerApi = ocrPager({ table, pager, numbers, filter: matches });
    ocrArrowScroll(document.querySelector(".repos-page .table-scroll"));

    input.addEventListener("input", () => {
        query = input.value.trim().toLowerCase();
        pagerApi.refresh();
    });
})();
