// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.review

import com.alibaba.opencodereview.idea.messages.WebviewChannel

/**
 * The config panel's host window, responsible only for the window lifecycle: create, show, and destroy.
 * Message handling lives in [ConfigPanelRouter]. It is an interface because the concrete panel can only be
 * implemented once the frontend-ready signal arrives; before that signal, an open-config request degrades
 * into an IDE notification instead of a silent no-response.
 */
interface ConfigPanelHost {

    /** Whether the panel is currently open. */
    val isOpen: Boolean

    /** Opens the panel; brings it to the front when already open. */
    fun open()

    /** Closes the panel. */
    fun close()

    /** The panel's outbound channel; implementers should drop messages while the panel is not open. */
    val channel: WebviewChannel
}
