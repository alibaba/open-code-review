// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewState
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * [resultToState] 的判定表，对应 ReviewSession 对应测试用例的 5 个场景。
 *
 * 这几条看似琐碎，但判定错误即会导致前端进入错误的界面：`empty` 显示"没有发现问题"，
 * `failed` 显示错误页，`done` 才渲染评论列表。尤其 `completed_with_errors` 那两条——
 * 部分文件审查失败但已产出评论时，必须当作 done 将评论呈现给用户，不得将整轮判定为失败。
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
    fun `有 comments 就是 done`() {
        assertEquals(ReviewState.DONE, resultToState(result("success", comments = 1)))
    }

    @Test
    fun `success 但没有 comments 是 empty`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("success")))
    }

    @Test
    fun `skipped 且没有 comments 是 empty`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("skipped")))
    }

    @Test
    fun `completed_with_errors 且没有 comments 是 failed`() {
        assertEquals(ReviewState.FAILED, resultToState(result("completed_with_errors")))
    }

    @Test
    fun `completed_with_errors 但有 comments 仍然是 done`() {
        // 同样遵循"有评论优先"原则：comments 的判定先于 status。
        assertEquals(ReviewState.DONE, resultToState(result("completed_with_errors", comments = 1)))
    }
}
