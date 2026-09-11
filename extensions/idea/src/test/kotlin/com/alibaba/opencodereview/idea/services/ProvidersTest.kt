package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.FrontendSources
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 预置 provider 清单的一致性检查。
 *
 * 宿主只需要"名称"（用于判断某个 provider 应写入 `providers` 桶还是 `custom_providers` 桶），
 * 完整的预置表由前端维护。两端名称集合必须一致：
 * 前端新增官方 provider 而宿主侧未跟进时，该 provider 会被当作自定义 provider 处理，
 * 配置写入错误的桶，CLI 无法读取——**不报错**，仅表现为审查时提示未配置模型。
 */
class ProvidersTest {

    /** 前端预置表中每个预置项均为 `name: 'xxx',` 形式。 */
    private val nameRegex = Regex("""\bname:\s*'([^']+)'""")

    @Test
    fun `预置 provider 名与前端 providers_ts 完全一致`() {
        val ts = FrontendSources.file("src/shared/providers.ts").readText()
        val fromFrontend = nameRegex.findAll(ts).map { it.groupValues[1] }.toSortedSet()

        assertTrue(
            "没能从 providers.ts 里正则出任何 name，说明上游改了写法，这个用例本身需要更新",
            fromFrontend.size >= 10,
        )
        assertEquals(
            "预置 provider 清单与前端不一致。差集：" +
                "前端多出 ${fromFrontend - presetProviderNames()}，" +
                "宿主多出 ${presetProviderNames() - fromFrontend}",
            fromFrontend,
            presetProviderNames().toSortedSet(),
        )
    }

    @Test
    fun `isPresetProvider 忽略大小写和首尾空格`() {
        val any = presetProviderNames().first()
        assertTrue(isPresetProvider(any))
        assertTrue(isPresetProvider(any.uppercase()))
        assertTrue(isPresetProvider("  $any  "))
        assertFalse(isPresetProvider("my-own-llm"))
        assertFalse(isPresetProvider(""))
    }
}
