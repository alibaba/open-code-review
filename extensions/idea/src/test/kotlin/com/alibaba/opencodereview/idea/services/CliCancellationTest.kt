// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.util.concurrent.CancellationException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlin.concurrent.thread
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class CliCancellationTest {
    @Test
    fun `cancellation before registration is not lost`() {
        val cancellation = CliCancellation()
        val terminations = AtomicInteger()
        cancellation.cancel()
        assertFailsWith<CancellationException> { cancellation.checkCancelled() }
        cancellation.attach { terminations.incrementAndGet() }
        cancellation.cancel()
        assertEquals(1, terminations.get())
    }

    @Test
    fun `concurrent cancellation and registration terminate exactly once`() {
        repeat(50) {
            val cancellation = CliCancellation()
            val start = CountDownLatch(1)
            val finished = CountDownLatch(2)
            val terminations = AtomicInteger()
            val errors = java.util.concurrent.ConcurrentLinkedQueue<Throwable>()
            val register = thread {
                try {
                    start.await()
                    cancellation.attach { terminations.incrementAndGet() }
                } catch (error: Throwable) {
                    errors.add(error)
                } finally {
                    finished.countDown()
                }
            }
            val cancel = thread {
                try {
                    start.await()
                    cancellation.cancel()
                    cancellation.cancel()
                } catch (error: Throwable) {
                    errors.add(error)
                } finally {
                    finished.countDown()
                }
            }
            start.countDown()
            try {
                assertTrue(finished.await(5, TimeUnit.SECONDS), "Cancellation deadlocked")
                assertTrue(errors.isEmpty(), errors.toString())
                assertEquals(1, terminations.get())
            } finally {
                register.interrupt()
                cancel.interrupt()
                register.join(1_000)
                cancel.join(1_000)
            }
        }
    }

    @Test
    fun `completed invocation releases its termination action`() {
        val cancellation = CliCancellation()
        var terminations = 0
        cancellation.attach { terminations++ }
        cancellation.detach()
        cancellation.cancel()
        assertEquals(0, terminations)
    }
}
