// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Small accessibility helpers shared by the viewer pages.
(() => {
    const themeKey = "ocr-viewer-theme";
    const applyTheme = (theme) => {
        const nextTheme = theme === "light" ? "light" : "dark";
        document.body.dataset.theme = nextTheme;
        const button = document.querySelector("[data-theme-toggle]");
        if (button) {
            button.setAttribute("aria-pressed", String(nextTheme === "light"));
            button.textContent = nextTheme === "light" ? "☀" : "☾";
            button.title = nextTheme === "light" ? "Switch to dark mode" : "Switch to light mode";
        }
        try {
            localStorage.setItem(themeKey, nextTheme);
        } catch {
            // Storage may be unavailable; the page still keeps the in-memory mode.
        }
    };

    const storedTheme = (() => {
        try {
            return localStorage.getItem(themeKey);
        } catch {
            return null;
        }
    })();
    applyTheme(storedTheme === "light" ? "light" : "dark");

    const button = document.querySelector("[data-theme-toggle]");
    if (button) {
        button.addEventListener("click", () => {
            const nextTheme = document.body.dataset.theme === "light" ? "dark" : "light";
            applyTheme(nextTheme);
        });
    }

    // The scrollable table wrappers are focusable regions, but not every
    // browser scrolls a focused container with the arrow keys; route the
    // keys through scrollBy so keyboard users can reach overflowing columns
    // and rows. Only the region itself listens: focus inside the table (a
    // link, for instance) keeps its own key behavior. The region only joins
    // the tab order while it actually overflows — a static tabindex would
    // leave a dead tab stop on wide screens — and the keys fall through to
    // their native behavior (Home/End jump the page) when there is nothing
    // to scroll.
    window.ocrArrowScroll = (region) => {
        if (!region) return;
        const scrolls = () => region.scrollWidth > region.clientWidth;
        const sync = () => {
            region.tabIndex = scrolls() ? 0 : -1;
        };
        sync();
        window.addEventListener("resize", sync);
        // A collapsed <details> measures 0 wide, so re-sync when it opens.
        const owner = region.closest("details");
        if (owner) owner.addEventListener("toggle", sync);
        region.addEventListener("keydown", (event) => {
            if (event.target !== region || !scrolls()) return;
            const step = {
                ArrowLeft: -40,
                ArrowRight: 40,
                Home: -Number.MAX_SAFE_INTEGER,
                End: Number.MAX_SAFE_INTEGER,
            }[event.key];
            if (step === undefined) return;
            region.scrollBy({ left: step });
            event.preventDefault();
        });
    };
})();
