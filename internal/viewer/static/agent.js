// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Per-finding handoff into a coding agent.
//
// Every vendor's deep link is prefilled but never submitted - Claude Code even
// shows a "Prompt from an external link" warning - so the user always confirms
// before anything runs. The menu is built around that: it hands over context,
// it does not start work.
//
// The parameter name for the prompt is NOT consistent across vendors (q /
// prompt / text), and each truncates at a different length, so the whole
// mapping lives here rather than being spread across the markup.

(() => {
    "use strict";

    // Documented limits. Claude Code's CLI caps q at 5000 chars and Cursor caps
    // text at 10000 URL-encoded; truncating to the smaller bound everywhere
    // keeps one rule instead of four.
    const MAX_PROMPT = 4000;

    // Builds the prompt body shared by every target. Kept as plain text with a
    // fenced diff for the code, because these are terminal/chat agents and
    // markdown survives the trip better than structured data would.
    function buildPrompt(ctx) {
        const lines = [];
        lines.push("Fix this code review finding.");
        lines.push("");
        if (ctx.severity) lines.push(`Severity: ${ctx.severity}`);
        if (ctx.category) lines.push(`Category: ${ctx.category}`);
        lines.push(`File: ${ctx.file}${lineRange(ctx)}`);
        if (ctx.branch) lines.push(`Branch: ${ctx.branch}`);
        lines.push("");
        lines.push("Finding:");
        lines.push(ctx.content || "(no description)");
        if (ctx.existing) {
            lines.push("");
            lines.push("Current code:");
            lines.push("```");
            lines.push(ctx.existing);
            lines.push("```");
        }
        if (ctx.suggestion) {
            lines.push("");
            lines.push("Suggested change (from the reviewer, adapt as needed):");
            lines.push("```");
            lines.push(ctx.suggestion);
            lines.push("```");
        }
        lines.push("");
        lines.push("Make the change in the repository above. Keep it minimal and match the surrounding style.");
        return lines.join("\n").slice(0, MAX_PROMPT);
    }

    function lineRange(ctx) {
        if (!ctx.start) return "";
        if (ctx.end && ctx.end !== ctx.start) return `:${ctx.start}-${ctx.end}`;
        return `:${ctx.start}`;
    }

    // Absolute path for agents that take a workspace. A relative or empty CWD
    // would silently open the agent in the wrong directory, so it is dropped
    // rather than passed through.
    function repoPath(ctx) {
        const repo = (ctx.repo || "").trim();
        if (!repo) return "";
        return repo.startsWith("/") || /^[A-Za-z]:[\\/]/.test(repo) ? repo : "";
    }

    // Each entry returns a URL. Grouped by vendor and ordered app -> CLI ->
    // extension, which is the order a user reaches for: the app is already
    // running, the CLI is the fallback, the editor extension is last.
    //
    // Only combinations with a documented, prefilled-prompt scheme are listed.
    // Notably there is no CLI scheme for any of them - `claude`, `codex app`
    // and `cursor agent` take a prompt as a positional argument with no URI
    // equivalent - so "Copy prompt for agent" is the CLI path, and the menu
    // says so rather than offering a link that silently does nothing.
    //
    // Sources: Claude Code deep-links doc (claude-cli://open?q=, cwd=) and
    // VS Code doc (vscode://anthropic.claude-code/open?prompt=); Cursor
    // deeplink reference (cursor://anysphere.cursor-deeplink/prompt?text=);
    // Codex commands doc (codex://new?prompt=, path=); VS Code command-line
    // doc (vscode://agents/new?prompt=, workspace=).
    const TARGETS = {
        // Claude Code, native app / CLI handler. Registered by the Claude Code
        // app; opens the terminal session with the prompt prefilled.
        "claude-app": (prompt, ctx) => {
            const p = new URLSearchParams({ q: prompt });
            const dir = repoPath(ctx);
            if (dir) p.set("cwd", dir);
            return "claude-cli://open?" + p.toString();
        },
        // Claude Code VS Code extension, which registers its own URI handler.
        "claude-vscode": (prompt) =>
            "vscode://anthropic.claude-code/open?" + new URLSearchParams({ prompt }).toString(),
        // Codex native app. codex://new needs at least one of prompt/path, and
        // path is what puts the chat in the right workspace.
        codex: (prompt, ctx) => {
            const p = new URLSearchParams({ prompt });
            const dir = repoPath(ctx);
            if (dir) p.set("path", dir);
            return "codex://new?" + p.toString();
        },
        cursor: (prompt) =>
            "cursor://anysphere.cursor-deeplink/prompt?" + new URLSearchParams({ text: prompt }).toString(),
        // VS Code's own agent draft, for the editor rather than one vendor.
        "vscode-agents": (prompt, ctx) => {
            const p = new URLSearchParams({ prompt });
            const dir = repoPath(ctx);
            // workspace takes a folder URI, not a bare path.
            if (dir) p.set("workspace", "file://" + dir);
            return "vscode://agents/new?" + p.toString();
        },
    };

    function readContext(el) {
        const d = el.dataset;
        return {
            file: d.file || "",
            start: parseInt(d.start, 10) || 0,
            end: parseInt(d.end, 10) || 0,
            category: d.category || "",
            severity: d.severity || "",
            content: d.content || "",
            existing: d.existing || "",
            suggestion: d.suggestion || "",
            branch: d.branch || "",
            repo: d.repo || "",
        };
    }

    async function copyText(text) {
        // execCommand is the fallback for browsers and insecure origins where
        // the async clipboard API is unavailable - the viewer is routinely
        // reached over plain http on a LAN address.
        if (navigator.clipboard && window.isSecureContext) {
            try {
                await navigator.clipboard.writeText(text);
                return true;
            } catch {
                /* fall through to the legacy path */
            }
        }
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.setAttribute("readonly", "");
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        let ok = false;
        try {
            ok = document.execCommand("copy");
        } catch {
            ok = false;
        }
        document.body.removeChild(ta);
        return ok;
    }

    function initActions(root) {
        const menu = root.querySelector(".agent-menu");
        const toggle = root.querySelector("[data-agent-menu-toggle]");
        if (!menu || !toggle) return;

        const links = Array.from(menu.querySelectorAll("[data-agent-open]"));

        // Build each anchor's real href as the menu opens.
        //
        // The href has to be in place before the click is dispatched, and the
        // browser has to be the one performing the navigation. Both matter: an
        // anchor whose href is assigned from inside its own click handler needs
        // either a re-dispatched click or a manual location change, and both of
        // those run outside the transient user activation that every current
        // browser requires before it will hand an external protocol
        // (claude-cli:, codex:, cursor:) to the desktop. The result is a click
        // that does nothing at all, with no error to show for it.
        //
        // So the URL is built here, while the menu is being opened by a real
        // click, and the click handler below only closes the menu - the browser
        // follows the href itself, carrying the user's gesture with it.
        const buildLinks = () => {
            const ctx = readContext(root);
            const prompt = buildPrompt(ctx);
            links.forEach((item) => {
                const build = TARGETS[item.dataset.agentOpen];
                if (build) item.href = build(prompt, ctx);
            });
        };

        const close = () => {
            menu.hidden = true;
            toggle.setAttribute("aria-expanded", "false");
        };
        const open = () => {
            buildLinks();
            menu.hidden = false;
            toggle.setAttribute("aria-expanded", "true");
        };

        toggle.addEventListener("click", (e) => {
            e.stopPropagation();
            menu.hidden ? open() : close();
        });

        // No preventDefault here: the default action IS the handoff. Keyboard
        // activation lands on the anchor as a real click too, so it needs
        // nothing extra.
        links.forEach((item) => {
            item.addEventListener("click", close);
        });

        const copyBtn = root.querySelector("[data-agent-copy]");
        const copied = root.querySelector("[data-agent-copied]");
        if (copyBtn) {
            copyBtn.addEventListener("click", async (e) => {
                e.preventDefault();
                const ok = await copyText(buildPrompt(readContext(root)));
                if (copied) {
                    copied.textContent = ok ? "Copied" : "Copy failed";
                    copied.hidden = false;
                    setTimeout(() => {
                        copied.hidden = true;
                    }, 2000);
                }
                close();
            });
        }

        // A click anywhere else, or Escape, dismisses the menu.
        document.addEventListener("click", (e) => {
            if (!root.contains(e.target)) close();
        });
        root.addEventListener("keydown", (e) => {
            if (e.key === "Escape") {
                close();
                toggle.focus();
            }
        });
    }

    function initAll() {
        document.querySelectorAll("[data-agent-actions]").forEach(initActions);
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", initAll);
    } else {
        initAll();
    }
})();
