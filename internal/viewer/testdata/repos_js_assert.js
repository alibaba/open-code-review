// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors
//
// Drives the shipped ocrRepos helpers from static/repos.js. Loaded by
// TestReposJS_SearchAndPagination after executing that file in goja.

function assert(cond, msg) {
    if (!cond) {
        throw new Error(msg);
    }
}

function fakeRow(name) {
    return {
        hidden: false,
        querySelector: function (sel) {
            if (sel === "[data-repository-name]") {
                return { textContent: name };
            }
            return null;
        }
    };
}

function visibleNames(rows) {
    var out = [];
    var i;
    for (i = 0; i < rows.length; i++) {
        if (!rows[i].hidden) {
            out.push(rows[i].querySelector("[data-repository-name]").textContent);
        }
    }
    return out.join(",");
}

function fakeDoc(names) {
    var rows = [];
    var i;
    for (i = 0; i < names.length; i++) {
        rows.push(fakeRow(names[i]));
    }
    var input = {
        value: "",
        listeners: {},
        addEventListener: function (type, fn) {
            if (!this.listeners[type]) {
                this.listeners[type] = [];
            }
            this.listeners[type].push(fn);
        },
        fireInput: function () {
            var list = this.listeners.input || [];
            var j;
            for (j = 0; j < list.length; j++) {
                list[j]();
            }
        }
    };
    var nav = {
        hidden: true,
        children: [],
        firstChild: null,
        ownerDocument: {
            createElement: function (tag) {
                return {
                    tagName: tag,
                    type: "",
                    className: "",
                    textContent: "",
                    disabled: false,
                    addEventListener: function () {},
                    setAttribute: function () {}
                };
            }
        },
        removeChild: function (child) {
            var next = [];
            var j;
            for (j = 0; j < this.children.length; j++) {
                if (this.children[j] !== child) {
                    next.push(this.children[j]);
                }
            }
            this.children = next;
            this.firstChild = this.children.length ? this.children[0] : null;
        },
        appendChild: function (child) {
            this.children.push(child);
            this.firstChild = this.children[0];
        }
    };
    var table = {
        querySelectorAll: function (sel) {
            if (sel === "tbody tr") {
                return rows;
            }
            return [];
        }
    };
    return {
        getElementById: function (id) {
            if (id === "repository-search-input") {
                return input;
            }
            if (id === "repositories-table") {
                return table;
            }
            if (id === "repositories-pagination") {
                return nav;
            }
            return null;
        },
        input: input,
        rows: rows,
        nav: nav
    };
}

var api = ocrRepos;
assert(api && typeof api.applyFilterAndPage === "function", "ocrRepos.applyFilterAndPage missing");
assert(api.PAGE_SIZE === 12, "PAGE_SIZE should match the mockup row count, got " + api.PAGE_SIZE);

assert(api.matchesRepositoryName("Alpha-Repo", "alpha") === true, "substring match should be case-insensitive");
assert(api.matchesRepositoryName("Alpha-Repo", "BETA") === false, "non-matching query should hide the name");
assert(api.matchesRepositoryName("Alpha-Repo", "  ") === true, "blank query should match every name");

var rows = [fakeRow("alpha"), fakeRow("beta"), fakeRow("alphabet"), fakeRow("gamma")];
var state = api.applyFilterAndPage(rows, "alp", 1, 10);
assert(state.matchedCount === 2, "query alp should keep alpha and alphabet, got " + state.matchedCount);
assert(visibleNames(rows) === "alpha,alphabet", "non-matching names must be hidden, got " + visibleNames(rows));
assert(state.page === 1 && state.totalPages === 1, "two matches fit on one page");

var paged = [];
var n;
for (n = 0; n < 10; n++) {
    paged.push(fakeRow("repo-" + n));
}
paged.push(fakeRow("other"));
state = api.applyFilterAndPage(paged, "repo-", 2, 3);
assert(state.matchedCount === 10, "repo- prefix should keep ten rows, got " + state.matchedCount);
assert(state.totalPages === 4, "ten matches at page size 3 is four pages, got " + state.totalPages);
assert(state.page === 2, "requested page 2 should stick when in range");
assert(visibleNames(paged) === "repo-3,repo-4,repo-5", "page 2 of the filtered set, got " + visibleNames(paged));
assert(paged[10].hidden === true, "non-matching other row stays hidden while paging");

state = api.applyFilterAndPage(paged, "repo-", 99, 3);
assert(state.page === 4, "out-of-range page should clamp to the last page, got " + state.page);
assert(visibleNames(paged) === "repo-9", "clamped last page should show the remainder, got " + visibleNames(paged));

var items = api.paginationItems(10, 2);
assert(items.length === 6, "mockup window for page 2 of 10 should be 1 2 3 4 ellipsis 10, got " + items.length);
assert(items[0].type === "page" && items[0].page === 1, "first item is page 1");
assert(items[1].type === "page" && items[1].page === 2, "second item is page 2");
assert(items[2].type === "page" && items[2].page === 3, "third item is page 3");
assert(items[3].type === "page" && items[3].page === 4, "fourth item is page 4");
assert(items[4].type === "ellipsis", "fifth item is ellipsis");
assert(items[5].type === "page" && items[5].page === 10, "last item is page 10");

var names = [];
for (n = 0; n < 30; n++) {
    names.push("alpha-" + (n < 10 ? "0" + n : String(n)));
}
var doc = fakeDoc(names);
var ctl = api.bindReposPage(doc, 10);
assert(ctl, "bindReposPage should return a controller");
assert(ctl.getPage() === 1, "initial page is 1");
assert(visibleNames(doc.rows) === "alpha-00,alpha-01,alpha-02,alpha-03,alpha-04,alpha-05,alpha-06,alpha-07,alpha-08,alpha-09", "first page of unfiltered rows, got " + visibleNames(doc.rows));
assert(doc.nav.hidden === false, "pager is shown when more than one page exists");
assert(doc.nav.children.length > 0, "pager renders buttons when visible");

ctl.setPage(3);
assert(ctl.getPage() === 3, "setPage(3) should stick");
assert(visibleNames(doc.rows) === "alpha-20,alpha-21,alpha-22,alpha-23,alpha-24,alpha-25,alpha-26,alpha-27,alpha-28,alpha-29", "page 3 of unfiltered rows, got " + visibleNames(doc.rows));

doc.input.value = "alpha-";
doc.input.fireInput();
assert(ctl.getPage() === 1, "changing the query must reset to page 1, got " + ctl.getPage());
assert(visibleNames(doc.rows) === "alpha-00,alpha-01,alpha-02,alpha-03,alpha-04,alpha-05,alpha-06,alpha-07,alpha-08,alpha-09", "query reset shows the first page of the filtered set, got " + visibleNames(doc.rows));

doc.input.value = "alpha-0";
doc.input.fireInput();
assert(ctl.getPage() === 1, "narrower query stays on page 1");
assert(visibleNames(doc.rows) === "alpha-00,alpha-01,alpha-02,alpha-03,alpha-04,alpha-05,alpha-06,alpha-07,alpha-08,alpha-09", "alpha-0 matches the ten 0x rows, got " + visibleNames(doc.rows));
assert(doc.nav.hidden === true, "pager hides when the filtered set fits on one page");
