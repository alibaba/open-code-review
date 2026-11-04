// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const table = document.getElementById("sessions-table");
    const pager = document.getElementById("sessions-pagination");
    const numbers = document.getElementById("sessions-page-numbers");
    if (!table || !pager || !numbers) return;

    ocrPager({ table, pager, numbers });
    ocrArrowScroll(document.querySelector(".sessions-page .table-scroll"));
})();
