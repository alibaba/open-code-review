// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.util.concurrent.CancellationException

/** A single invocation owns this handle, including cancellation before process registration. */
class CliCancellation {
    private val lock = Any()
    private var cancelled = false
    private var attached = false
    private var terminate: (() -> Unit)? = null

    fun cancel() {
        val action = synchronized(lock) {
            if (cancelled) return
            cancelled = true
            terminate.also { terminate = null }
        }
        action?.invoke()
    }

    internal fun checkCancelled() {
        synchronized(lock) {
            if (cancelled) throw CancellationException("Review cancelled")
        }
    }

    internal fun attach(action: () -> Unit) {
        val cancelNow = synchronized(lock) {
            check(!attached) { "A cancellation handle cannot be reused" }
            attached = true
            if (cancelled) true else {
                terminate = action
                false
            }
        }
        if (cancelNow) action()
    }

    internal fun detach() {
        synchronized(lock) { terminate = null }
    }
}
