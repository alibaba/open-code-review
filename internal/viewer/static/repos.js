// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const input = document.getElementById("repository-search-input");
    const table = document.getElementById("repositories-table");
    if (!input || !table) return;

    const rows = table.querySelectorAll("tbody tr");
    input.addEventListener("input", () => {
        const query = input.value.trim().toLowerCase();
        rows.forEach((row) => {
            const nameCell = row.querySelector("[data-repository-name]");
            const name = nameCell ? nameCell.textContent.trim().toLowerCase() : "";
            row.hidden = !name.includes(query);
        });
    });
})();
