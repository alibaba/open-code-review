// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.review

import com.alibaba.opencodereview.idea.messages.WebviewChannel

/**
 * Host window for the config panel; it owns only the window lifecycle — create, show, dispose.
 * Message handling lives in [ConfigPanelRouter]. The separate interface exists because the real
 * panel can only be implemented once the frontend's ready signal arrives; until that signal, a
 * request to open the configuration degrades to an IDE notification rather than failing silently.
 */
interface ConfigPanelHost {

    /** Whether the panel is currently open. */
    val isOpen: Boolean

    /** Opens the panel; brings it to the front when already open. */
    fun open()

    /** Closes the panel. */
    fun close()

    /** The panel's outbound channel; implementations should drop messages while the panel is
     *  closed. */
    val channel: WebviewChannel
}
