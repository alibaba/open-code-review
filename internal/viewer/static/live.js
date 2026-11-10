// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Live status and duration for sessions that are still running.
//
// A running review has written no session_end yet, so the server cannot know
// its duration or final state - only that a process still holds the session's
// lock. The duration is therefore computed here from the start timestamp the
// page already carries, and the whole page is re-fetched when a run finishes,
// so the terminal numbers always come from the server rather than from a
// guess made in the browser.

(() => {
    "use strict";

    const POLL_MS = 2000;

    // Same thresholds the Go helper uses (formatDuration), so a value that
    // ticks past a minute does not reformat differently than the server would.
    function formatDuration(seconds) {
        if (seconds < 60) return seconds.toFixed(1) + "s";
        const minutes = Math.floor(seconds / 60);
        const sec = Math.floor(seconds) - minutes * 60;
        return minutes + "m" + sec + "s";
    }

    function tick() {
        const now = Date.now() / 1000;
        document.querySelectorAll("[data-live-duration]").forEach((el) => {
            const started = parseFloat(el.dataset.started);
            if (!started) return;
            // A finished run has a nonzero end stamp; it is not ticking, and
            // the server's own duration is authoritative for it.
            const ended = parseFloat(el.dataset.ended);
            const end = ended && ended > 0 ? ended : now;
            const next = formatDuration(Math.max(0, end - started));
            if (el.textContent !== next) el.textContent = next;
        });
    }

    // Refresh the page when a run that was live is no longer live. Reloading
    // rather than patching the DOM is deliberate: a finished run brings new
    // findings, token totals and a terminal state, and re-rendering those
    // correctly is what the server already does.
    function watchForCompletion() {
        const isSessionsPage = !!document.querySelector(".sessions-page");
        let previous = runningSessionKeys();
        if (previous === "[]" && !isSessionsPage) return;

        setInterval(async () => {
            // Pause while the tab is hidden: a background review page that
            // reloads itself the moment the user returns is hostile, and the
            // state is re-read on visibility anyway.
            if (document.hidden) return;
            try {
                const res = await fetch(window.location.href, {
                    headers: { Accept: "text/html" },
                    cache: "no-store",
                });
                if (!res.ok) return;
                const html = await res.text();
                const current = runningSessionKeys(html);
                if (current !== previous) {
                    previous = current;
                    window.location.reload();
                }
            } catch {
                /* a transient network error just means we try again next tick */
            }
        }, POLL_MS);
    }

    // A set of running session URLs catches one run ending while another
    // starts, which a page-wide running boolean cannot distinguish.
    function runningSessionKeys(html) {
        const root = html === undefined
            ? document
            : new DOMParser().parseFromString(html, "text/html");
        return JSON.stringify(Array.from(root.querySelectorAll("[data-live-status].status-running"))
            .map((el) => {
                const link = el.closest("tr")?.querySelector("a.session-id");
                return link ? link.getAttribute("href") : window.location.pathname;
            })
            .sort());
    }

    function init() {
        tick();
        watchRepoActivity();
        watchForCompletion();
        if (!document.querySelector("[data-live-duration]")) return;
        setInterval(tick, 1000);
    }

    // The repositories page polls the active session URL for each repository.
    //
    // Capture active session URLs once and compare them on each poll, so a new
    // run or a change between concurrently active runs is visible.
    function watchRepoActivity() {
        if (!document.querySelector(".repos-page")) return;

        let previous = activeRepoKeys();

        setInterval(async () => {
            // Checked per tick, not once at setup: a page opened in a background
            // tab reports document.hidden at setup, and returning there would
            // mean polling never begins at all - the tab that needs it most is
            // exactly the one that would never start.
            if (document.hidden) return;
            try {
                const res = await fetch(window.location.href, {
                    headers: { Accept: "text/html" },
                    cache: "no-store",
                });
                if (!res.ok) return;
                const current = activeRepoKeys(await res.text());
                if (current !== previous) {
                    previous = current;
                    window.location.reload();
                }
            } catch {
                /* try again next tick */
            }
        }, POLL_MS);
    }

    function activeRepoKeys(html) {
        const root = html === undefined
            ? document
            : new DOMParser().parseFromString(html, "text/html");
        return JSON.stringify(Array.from(root.querySelectorAll("[data-repo-active]"))
            .map((el) => el.getAttribute("href"))
            .sort());
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", init);
    } else {
        init();
    }
})();
