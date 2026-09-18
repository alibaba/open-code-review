// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Small accessibility helpers shared by the viewer pages.
(() => {
    // The scrollable table wrappers are focusable regions, but not every
    // browser scrolls a focused container with the arrow keys; route the
    // keys through scrollBy so keyboard users can reach overflowing columns
    // and rows. Only the region itself listens: focus inside the table (a
    // link, for instance) keeps its own key behavior.
    window.ocrArrowScroll = (region) => {
        if (!region) return;
        region.addEventListener("keydown", (event) => {
            if (event.target !== region) return;
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
