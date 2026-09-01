package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 入站消息契约的单测。此层不涉及任何 IDE API，因此 21 个类型可全部直接测试。
 *
 * 重点不在于"能否解析"，而在于**解析失败时不得抛出异常**——前端版本领先宿主一个版本、
 * 或某字段临时缺失时，都不应导致整条消息通道中断。
 */
class WebviewMessagesTest {

    /** locale 仅影响 Malformed 的提示文案，绝大多数用例不关心，固定使用中文即可。 */
    private fun parse(raw: String): WebviewToHost = parseWebviewMessage(raw, SupportedLocale.ZH_CN)

    // ------------------------------------------------------------ 无参数类型

    @Test
    fun `无参数的消息类型逐一命中`() {
        val cases = mapOf(
            "ready" to WebviewToHost.Ready,
            "readyConfigPanel" to WebviewToHost.ReadyConfigPanel,
            "closeConfigPanel" to WebviewToHost.CloseConfigPanel,
            "cancelReview" to WebviewToHost.CancelReview,
            "getConfig" to WebviewToHost.GetConfig,
            "checkCli" to WebviewToHost.CheckCli,
            "checkEnvironment" to WebviewToHost.CheckEnvironment,
            "installCli" to WebviewToHost.InstallCli,
        )
        cases.forEach { (type, expected) ->
            assertEquals(type, expected, parse("""{"type":"$type"}"""))
        }
    }

    // ------------------------------------------------------------ 侧栏

    @Test
    fun `getGitState 解析模式`() {
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.BRANCH),
            parse("""{"type":"getGitState","mode":"branch"}"""),
        )
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.COMMIT),
            parse("""{"type":"getGitState","mode":"commit"}"""),
        )
    }

    @Test
    fun `未知模式退回 workspace`() {
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.WORKSPACE),
            parse("""{"type":"getGitState","mode":"nonsense"}"""),
        )
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.WORKSPACE),
            parse("""{"type":"getGitState"}"""),
        )
    }

    @Test
    fun `getModeFiles 带分支两端`() {
        val msg = parse(
            """{"type":"getModeFiles","mode":"branch","from":"main","to":"dev"}""",
        )
        assertEquals(WebviewToHost.GetModeFiles(ReviewMode.BRANCH, "main", "dev", null), msg)
    }

    @Test
    fun `getModeFiles 的空串等于没填`() {
        // 表单未选择分支时发送的是空串而非省略字段；若按空串传递给 git 会被解析为非法 ref。
        val msg = parse(
            """{"type":"getModeFiles","mode":"branch","from":"","to":"  "}""",
        ) as WebviewToHost.GetModeFiles
        assertNull(msg.from)
        assertNull(msg.to)
    }

    @Test
    fun `openFileDiff 全字段`() {
        val msg = parse(
            """{"type":"openFileDiff","path":"src/a.kt","status":"deleted","mode":"commit","commit":"abc1234"}""",
        )
        assertEquals(
            WebviewToHost.OpenFileDiff("src/a.kt", FileStatus.DELETED, ReviewMode.COMMIT, null, null, "abc1234"),
            msg,
        )
    }

    @Test
    fun `openFileDiff 的未知状态退回 modified`() {
        val msg = parse(
            """{"type":"openFileDiff","path":"a.kt","status":"copied","mode":"workspace"}""",
        ) as WebviewToHost.OpenFileDiff
        assertEquals(FileStatus.MODIFIED, msg.status)
    }

    @Test
    fun `openFileDiff 缺 path 是 Malformed`() {
        val msg = parse("""{"type":"openFileDiff","status":"added","mode":"workspace"}""")
        assertTrue(msg is WebviewToHost.Malformed)
    }

    @Test
    fun `startReview 解析审查参数`() {
        val msg = parse(
            """{"type":"startReview","options":{"mode":"branch","from":"main","to":"dev","concurrency":4}}""",
        ) as WebviewToHost.StartReview
        assertEquals(ReviewMode.BRANCH, msg.options.mode)
        assertEquals("main", msg.options.from)
        assertEquals("dev", msg.options.to)
        assertEquals(4, msg.options.concurrency)
        assertNull(msg.options.customPrompt)
    }

    @Test
    fun `startReview 缺 options 是 Malformed`() {
        assertTrue(parse("""{"type":"startReview"}""") is WebviewToHost.Malformed)
    }

    @Test
    fun `startReview 的 options 类型不对是 Malformed 而不是抛异常`() {
        assertTrue(
            parse("""{"type":"startReview","options":"workspace"}""") is WebviewToHost.Malformed,
        )
        // 字段类型错至反序列化失败时同样必须降级，不得让异常穿出解析层。
        assertTrue(
            parse("""{"type":"startReview","options":{"concurrency":"many"}}""")
                is WebviewToHost.Malformed,
        )
    }

    @Test
    fun `jumpToComment 与 commentAction`() {
        assertEquals(
            WebviewToHost.JumpToComment(3),
            parse("""{"type":"jumpToComment","index":3}"""),
        )
        assertEquals(
            WebviewToHost.CommentAction(0, CommentActionKind.APPLY),
            parse("""{"type":"commentAction","index":0,"action":"apply"}"""),
        )
        assertEquals(
            WebviewToHost.CommentAction(1, CommentActionKind.DISCARD),
            parse("""{"type":"commentAction","index":1,"action":"discard"}"""),
        )
        assertEquals(
            WebviewToHost.CommentAction(2, CommentActionKind.FALSE_POSITIVE),
            parse("""{"type":"commentAction","index":2,"action":"falsePositive"}"""),
        )
    }

    @Test
    fun `index 缺失或非数字都是 Malformed`() {
        assertTrue(parse("""{"type":"jumpToComment"}""") is WebviewToHost.Malformed)
        assertTrue(parse("""{"type":"jumpToComment","index":"abc"}""") is WebviewToHost.Malformed)
        assertTrue(parse("""{"type":"jumpToComment","index":null}""") is WebviewToHost.Malformed)
        assertTrue(
            parse("""{"type":"commentAction","index":1,"action":"nope"}""")
                is WebviewToHost.Malformed,
        )
    }

    @Test
    fun `数字字符串形式的 index 照样接受`() {
        // 与 JavaScript 行为一致：`comments["3"]` 在 JS 中也能取到第 4 个元素，
        // 此处同样保持宽松处理，不为前端不会发送的形态生成 Malformed。
        assertEquals(
            WebviewToHost.JumpToComment(3),
            parse("""{"type":"jumpToComment","index":"3"}"""),
        )
    }

    @Test
    fun `openConfigPanel 原样透传 focus`() {
        val msg = parse(
            """{"type":"openConfigPanel","focus":{"step":2,"tab":"custom"}}""",
        ) as WebviewToHost.OpenConfigPanel
        val focus = msg.focus as JsonObject
        assertEquals(JsonPrimitive(2), focus["step"])
        assertEquals(JsonPrimitive("custom"), focus["tab"])
    }

    @Test
    fun `openConfigPanel 可以不带 focus`() {
        val msg = parse("""{"type":"openConfigPanel"}""") as WebviewToHost.OpenConfigPanel
        assertNull(msg.focus)
    }

    // ------------------------------------------------------------ 配置面板

    @Test
    fun `setConfig 允许空值`() {
        // 清空某个字段即发送空串，不得当作缺字段处理。
        assertEquals(
            WebviewToHost.SetConfig("llm.url", ""),
            parse("""{"type":"setConfig","key":"llm.url","value":""}"""),
        )
    }

    @Test
    fun `setConfig 缺 key 是 Malformed`() {
        assertTrue(parse("""{"type":"setConfig","value":"x"}""") is WebviewToHost.Malformed)
    }

    @Test
    fun `setConfigBatch 与 testConnection 解析条目`() {
        val json = """{"type":"%s","entries":[{"key":"provider","value":"kimi"},{"key":"model","value":""}]}"""
        val batch = parse(json.format("setConfigBatch")) as WebviewToHost.SetConfigBatch
        assertEquals(2, batch.entries.size)
        assertEquals("provider", batch.entries[0].key)
        assertEquals("kimi", batch.entries[0].value)
        assertEquals("", batch.entries[1].value)

        val test = parse(json.format("testConnection")) as WebviewToHost.TestConnection
        assertEquals(batch.entries, test.entries)
    }

    @Test
    fun `entries 里的坏条目被丢掉而不是整条消息失败`() {
        val msg = parse(
            """{"type":"setConfigBatch","entries":[{"value":"没有key"},"字符串",{"key":"model","value":"m"}]}""",
        ) as WebviewToHost.SetConfigBatch
        assertEquals(1, msg.entries.size)
        assertEquals("model", msg.entries[0].key)
    }

    @Test
    fun `entries 缺失时是空列表`() {
        val msg = parse("""{"type":"setConfigBatch"}""") as WebviewToHost.SetConfigBatch
        assertTrue(msg.entries.isEmpty())
    }

    @Test
    fun `自定义 provider 的删除与激活`() {
        assertEquals(
            WebviewToHost.DeleteCustomProvider("my-llm"),
            parse("""{"type":"deleteCustomProvider","name":"my-llm"}"""),
        )
        assertEquals(
            WebviewToHost.ActivateCustomProvider("my-llm"),
            parse("""{"type":"activateCustomProvider","name":"my-llm"}"""),
        )
        assertTrue(parse("""{"type":"deleteCustomProvider"}""") is WebviewToHost.Malformed)
        assertTrue(
            parse("""{"type":"activateCustomProvider","name":" "}""") is WebviewToHost.Malformed,
        )
    }

    @Test
    fun `copyToClipboard 允许空文本`() {
        assertEquals(
            WebviewToHost.CopyToClipboard(""),
            parse("""{"type":"copyToClipboard"}"""),
        )
        assertEquals(
            WebviewToHost.CopyToClipboard("sk-xxx"),
            parse("""{"type":"copyToClipboard","text":"sk-xxx"}"""),
        )
    }

    // ------------------------------------------------------------ 兜底

    @Test
    fun `不认识的类型是 Unknown 而不是 Malformed`() {
        // 前端版本领先宿主时会多发类型，此为正常情况，不应向用户报错。
        assertEquals(
            WebviewToHost.Unknown("somethingNew"),
            parse("""{"type":"somethingNew","x":1}"""),
        )
    }

    @Test
    fun `坏 JSON 与缺 type 都是 Malformed 且不抛异常`() {
        assertTrue(parse("{不是json") is WebviewToHost.Malformed)
        assertTrue(parse("") is WebviewToHost.Malformed)
        assertTrue(parse("[1,2,3]") is WebviewToHost.Malformed)
        assertTrue(parse("""{"foo":"bar"}""") is WebviewToHost.Malformed)
        // type 不是字符串时同样只能降级。
        assertTrue(parse("""{"type":42}""") is WebviewToHost.Malformed)
    }

    @Test
    fun `Malformed 的提示文案跟着 locale 走`() {
        // reason 会原样回传给前端展示，因此必须使用 IDE 界面语言，不得写死中文。
        val raw = """{"type":"setConfig","value":"x"}"""
        assertEquals(
            "setConfig 缺少 key",
            (parseWebviewMessage(raw, SupportedLocale.ZH_CN) as WebviewToHost.Malformed).reason,
        )
        assertEquals(
            "setConfig is missing required field: key",
            (parseWebviewMessage(raw, SupportedLocale.EN) as WebviewToHost.Malformed).reason,
        )
    }
}
