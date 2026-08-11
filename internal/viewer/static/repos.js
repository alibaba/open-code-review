// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const input = document.getElementById("repository-search-input");
    const table = document.getElementById("repositories-table");
    const empty = document.getElementById("repository-search-empty");
    if (!input || !table || !empty) return;

    const rows = table.querySelectorAll("tbody tr");
    input.addEventListener("input", () => {
        const query = input.value.trim().toLowerCase();
        let visible = 0;
        rows.forEach((row) => {
            const nameCell = row.querySelector("[data-repository-name]");
            const name = nameCell ? nameCell.textContent.trim().toLowerCase() : "";
            const sessionLinks = row.querySelectorAll("[data-session-id]");
            let matchingSessions = 0;
            sessionLinks.forEach((session) => {
                const matches = query !== "" && (session.dataset.sessionId || "").toLowerCase().includes(query);
                session.hidden = !matches;
                if (matches) matchingSessions++;
            });
            const matches = query === "" || name.includes(query) || matchingSessions > 0;
            row.hidden = !matches;
            if (matches) visible++;
        });
        empty.hidden = visible > 0;
    });
})();
