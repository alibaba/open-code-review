// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewState
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * The [resultToState] decision table covers the five scenarios in the corresponding ReviewSession tests.
 *
 * Incorrect decisions send the frontend to the wrong view: `empty` shows no findings,
 * `failed` shows an error page, and only `done` renders comments. The two `completed_with_errors` cases matter especially:
 * if some files failed but comments were produced, show those comments as done instead of failing the whole review.
 */
class ReviewSessionTest {

    @Test
    fun `incomplete and unknown results cannot report a clean review`() {
        for (status in listOf("partial", "failed", "", "future_status")) {
            assertEquals(ReviewState.FAILED, resultToState(result(status)), status)
        }
    }

    @Test
    fun `complete results and partial findings remain visible`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("complete")))
        assertEquals(ReviewState.DONE, resultToState(result("partial", comments = 1)))
    }

    private fun result(status: String, comments: Int = 0) =
        CliResult(status = status, comments = List(comments) { ReviewComment(path = "a.ts") })

    @Test
    fun `comments result in done`() {
        assertEquals(ReviewState.DONE, resultToState(result("success", comments = 1)))
    }

    @Test
    fun `success without comments results in empty`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("success")))
    }

    @Test
    fun `skipped without comments results in empty`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("skipped")))
    }

    @Test
    fun `completed_with_errors without comments results in failed`() {
        assertEquals(ReviewState.FAILED, resultToState(result("completed_with_errors")))
    }

    @Test
    fun `completed_with_errors with comments still results in done`() {
        // Comments take precedence: check comments before status.
        assertEquals(ReviewState.DONE, resultToState(result("completed_with_errors", comments = 1)))
    }
}
