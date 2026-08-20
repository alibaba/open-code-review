package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CommentStatus
import com.alibaba.opencodereview.idea.model.CommentSyncState
import com.alibaba.opencodereview.idea.model.EnvCheckResult
import com.alibaba.opencodereview.idea.model.EnvToolStatus
import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.GitState
import com.alibaba.opencodereview.idea.model.LogLevel
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.ReviewState
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 出站消息的 JSON 形状。
 *
 * 此组断言看似琐碎，但它是唯一能拦截"前端静默空白"问题的手段：
 * 字段名拼写错误、`type` 字面量不匹配、枚举序列化为 Kotlin 的大写常量，
 * 编译器均不会报告，运行时也不抛异常，仅表现为前端无法渲染内容。
 */
class HostMessagesTest {

    private fun parse(json: String) = Json.parseToJsonElement(json).jsonObject

    @Test
    fun `init 必须带齐 config gitState locale`() {
        val json = parse(
            HostToWebview.Init(
                config = OcrConfig(provider = "kimi", model = "k2"),
                gitState = GitState(currentBranch = "main"),
                locale = SupportedLocale.ZH_CN,
            ).toJson(),
        )
        assertEquals("init", json["type"].toString().trim('"'))
        assertTrue(json.containsKey("config"))
        assertTrue(json.containsKey("gitState"))
        // config 缺失时前端配置就绪判定恒为 false，界面将停留在配置视图。
        assertEquals("kimi", json["config"]!!.jsonObject["provider"].toString().trim('"'))
        assertEquals("main", json["gitState"]!!.jsonObject["currentBranch"].toString().trim('"'))
        assertEquals("zh-cn", json["locale"].toString().trim('"'))
    }

    @Test
    fun `locale 序列化成前端认的字面量`() {
        val en = parse(
            HostToWebview.Init(null, GitState(), SupportedLocale.EN).toJson(),
        )
        assertEquals("en", en["locale"].toString().trim('"'))
    }

    @Test
    fun `config 为 null 时字段整个消失而不是发 null`() {
        val json = parse(HostToWebview.Config(null).toJson())
        assertEquals("config", json["type"].toString().trim('"'))
        // explicitNulls = false：字段缺失而非显式 null，与前端配置就绪判定语义一致。
        assertFalse(json.containsKey("config"))
    }

    @Test
    fun `stateChange 无错误时不带 error 字段`() {
        val ok = parse(HostToWebview.StateChange(ReviewState.RUNNING).toJson())
        assertEquals("stateChange", ok["type"].toString().trim('"'))
        assertEquals("running", ok["state"].toString().trim('"'))
        assertFalse(ok.containsKey("error"))

        val failed = parse(HostToWebview.StateChange(ReviewState.FAILED, "炸了").toJson())
        assertEquals("failed", failed["state"].toString().trim('"'))
        assertEquals("炸了", failed["error"].toString().trim('"'))
    }

    @Test
    fun `modeFiles 的模式与状态都是小写字面量`() {
        val json = parse(
            HostToWebview.ModeFiles(
                ReviewMode.BRANCH,
                listOf(FileChange("src/a.kt", FileStatus.RENAMED)),
            ).toJson(),
        )
        assertEquals("modeFiles", json["type"].toString().trim('"'))
        assertEquals("branch", json["mode"].toString().trim('"'))
        val first = json["files"].toString()
        assertTrue(first.contains("\"renamed\""))
        assertTrue(first.contains("\"src/a.kt\""))
    }

    @Test
    fun `logLine 包一层 line 对象`() {
        val json = parse(HostToWebview.Log(LogLine("hello", LogLevel.ERROR)).toJson())
        assertEquals("logLine", json["type"].toString().trim('"'))
        assertEquals("hello", json["line"]!!.jsonObject["text"].toString().trim('"'))
        assertEquals("error", json["line"]!!.jsonObject["level"].toString().trim('"'))
    }

    @Test
    fun `reviewDone 把结果包在 result 里且评论字段是 camelCase`() {
        val json = parse(
            HostToWebview.ReviewDone(
                CliResult(
                    status = "success",
                    comments = listOf(
                        ReviewComment(
                            path = "a.kt",
                            content = "改这里",
                            suggestionCode = "val x = 1",
                            existingCode = "var x = 1",
                            startLine = 10,
                            endLine = 12,
                        ),
                    ),
                ),
            ).toJson(),
        )
        assertEquals("reviewDone", json["type"].toString().trim('"'))
        val result = json["result"]!!.jsonObject.toString()
        // CLI 输出侧为 suggestion_code / existing_code / start_line，这些名称不得透传到前端。
        assertTrue(result.contains("\"suggestionCode\""))
        assertTrue(result.contains("\"existingCode\""))
        assertTrue(result.contains("\"startLine\""))
        assertFalse(result.contains("suggestion_code"))
        assertFalse(result.contains("start_line"))
    }

    @Test
    fun `commentSync 的状态字面量是 falsePositive`() {
        val json = parse(
            HostToWebview.CommentSync(
                listOf(
                    CommentSyncState(0, CommentStatus.FALSE_POSITIVE, jumpable = false),
                    CommentSyncState(1),
                ),
            ).toJson(),
        )
        assertEquals("commentSync", json["type"].toString().trim('"'))
        val body = json["comments"].toString()
        assertTrue(body.contains("\"falsePositive\""))
        assertFalse(body.contains("false_positive"))
        assertTrue(body.contains("\"pending\""))
        assertTrue(body.contains("\"jumpable\":false"))
    }

    @Test
    fun `gitState 消息的 type 是 gitState 而不是 gitStateChanged`() {
        // Kotlin 侧类名为 GitStateChanged，但消息契约中的字面量为 gitState。
        val json = parse(HostToWebview.GitStateChanged(GitState()).toJson())
        assertEquals("gitState", json["type"].toString().trim('"'))
    }

    // ------------------------------------------------------------ 配置面板

    @Test
    fun `configPanelInit 带 focus env skipEnvCheck`() {
        val focus = buildJsonObject { put("step", 2) }
        val json = parse(
            ConfigPanelHostToWebview.Init(
                config = OcrConfig(),
                focus = focus,
                env = EnvCheckResult(node = EnvToolStatus(ok = true, version = "v20.0.0")),
                skipEnvCheck = true,
                locale = SupportedLocale.EN,
            ).toJson(),
        )
        assertEquals("configPanelInit", json["type"].toString().trim('"'))
        assertEquals(2, json["focus"]!!.jsonObject["step"].toString().toInt())
        assertTrue(json["env"].toString().contains("\"v20.0.0\""))
        assertEquals("true", json["skipEnvCheck"].toString())
    }

    @Test
    fun `configPanel 的其余出站消息形状`() {
        assertEquals(
            "configPanelFocus",
            parse(ConfigPanelHostToWebview.Focus(null).toJson())["type"].toString().trim('"'),
        )
        val conn = parse(ConfigPanelHostToWebview.ConnectionResult(false, "401").toJson())
        assertEquals("connectionResult", conn["type"].toString().trim('"'))
        assertEquals("false", conn["ok"].toString())
        assertEquals("401", conn["message"].toString().trim('"'))

        // 成功时 message 为 null，对应字段应消失。
        assertFalse(parse(ConfigPanelHostToWebview.ConnectionResult(true).toJson()).containsKey("message"))

        assertEquals(
            "environmentResult",
            parse(ConfigPanelHostToWebview.EnvironmentResult(EnvCheckResult()).toJson())["type"]
                .toString().trim('"'),
        )
        assertEquals(
            "cliStatus",
            parse(ConfigPanelHostToWebview.CliStatus(true).toJson())["type"].toString().trim('"'),
        )
        assertEquals(
            "panelError",
            parse(ConfigPanelHostToWebview.PanelError("x").toJson())["type"].toString().trim('"'),
        )
        assertEquals(
            "installLog",
            parse(ConfigPanelHostToWebview.InstallLog(LogLine("npm...")).toJson())["type"]
                .toString().trim('"'),
        )
        assertEquals(
            "installDone",
            parse(ConfigPanelHostToWebview.InstallDone(true).toJson())["type"].toString().trim('"'),
        )
    }

    @Test
    fun `copyDone 是只有 type 的空消息`() {
        val json = parse(ConfigPanelHostToWebview.CopyDone.toJson())
        assertEquals("copyDone", json["type"].toString().trim('"'))
        assertEquals(1, json.size)
    }
}
