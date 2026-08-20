package com.alibaba.opencodereview.idea

import java.io.File

/**
 * 定位 `frontend/` 目录，供"宿主与前端是否保持一致"这类用例读取前端源码。
 *
 * 这些用例存在的价值：前端同步新增内容（新 provider、新主题变量）后，宿主侧未跟进时，
 * 界面表现为**静默降级**——不报错、不白屏，仅某块内容或颜色缺失，靠人工观察几乎无法发现。
 * 此类偏移只能通过构建期校验发现。
 */
object FrontendSources {

    /** 项目根。Gradle 执行测试时工作目录即为项目根，其他执行方式不一定，因此向上查找一层作为兜底。 */
    private val projectRoot: File by lazy {
        var dir: File? = File(System.getProperty("user.dir")).absoluteFile
        while (dir != null) {
            if (File(dir, "frontend/src/shared").isDirectory) return@lazy dir
            dir = dir.parentFile
        }
        error("找不到 frontend/ 目录（从 ${System.getProperty("user.dir")} 往上找过了）")
    }

    val frontendDir: File get() = File(projectRoot, "frontend")

    fun file(relative: String): File = File(frontendDir, relative).also {
        check(it.isFile) { "前端文件不存在：$it（前端目录结构变了？）" }
    }

    /** 递归读取某个子目录下的全部源码文本，合并为整体字符串以便正则扫描。 */
    fun readAllText(relativeDir: String, vararg extensions: String): String {
        val dir = File(frontendDir, relativeDir)
        check(dir.isDirectory) { "前端目录不存在：$dir" }
        return dir.walkTopDown()
            .filter { it.isFile && extensions.any { ext -> it.name.endsWith(ext) } }
            .joinToString("\n") { it.readText() }
    }
}
