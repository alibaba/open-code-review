package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.ConfigEntry
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import com.intellij.openapi.diagnostic.thisLogger
import kotlinx.serialization.json.JsonObject
import java.io.File
import java.nio.file.Files
import java.nio.file.attribute.PosixFilePermissions

/**
 * 读写 `~/.opencodereview/config.json`。分工：
 * 写单个键走 CLI（`ocr config set`），因 CLI 自带校验和规范化；只有"删除自定义 provider"直接改文件——CLI 无对应 unset 子命令。
 */
class ConfigService(
    private val cli: CliService,
    /** `ocr config set` 的工作目录。该子命令不依赖 cwd，取稳定值即可。 */
    private val cwd: File = File(System.getProperty("user.dir")),
) {

    private fun configPath(): File =
        File(File(System.getProperty("user.home"), ".opencodereview"), "config.json")

    /** 读取并转换为插件内部使用的 camelCase 配置；文件不存在或内容非法返回 null。 */
    fun read(): OcrConfig? {
        val path = configPath()
        if (!path.isFile) return null
        return runCatching { parseConfig(path.readText()) }.getOrElse {
            thisLogger().warn("[ocr] Failed to parse config, treating as no config: ${path.absolutePath}", it)
            null
        }
    }

    /** 读取原始 snake_case JSON，保留所有未知字段。 */
    private fun readRaw(): RawConfig {
        val path = configPath()
        if (!path.isFile) return emptyRawConfig()
        return runCatching { parseRawConfig(path.readText()) }.getOrElse { emptyRawConfig() }
    }

    /** 写回原始配置。整份配置为空时删文件而非写 `{}`——保留空文件会让 CLI 认为"已配置过"。 */
    private fun writeRaw(raw: RawConfig): OcrConfig? {
        val path = configPath()
        if (!raw.hasContent()) {
            if (path.exists() && !path.delete()) {
                thisLogger().warn("[ocr] Failed to delete empty config file: ${path.absolutePath}")
            }
            return null
        }
        val dir = path.parentFile
        runCatching {
            if (!dir.isDirectory) {
                Files.createDirectories(dir.toPath())
                trySetPosixPermissions(dir, "rwxr-xr-x")
            }
            path.writeText(raw.toPrettyJson())
            // 配置含 api_key，权限须收紧为仅本人可读。
            trySetPosixPermissions(path, "rw-------")
        }.onFailure {
            thisLogger().warn("[ocr] Failed to write config file: ${path.absolutePath}", it)
            return read()
        }
        return read()
    }

    /** 判断配置是否还有实质内容。空字符串不计入——空串在此判定中为 falsy。 */
    private fun RawConfig.hasContent(): Boolean {
        fun nonEmptyStr(key: String): Boolean {
            val prim = this[key] as? kotlinx.serialization.json.JsonPrimitive ?: return false
            return prim.isString && prim.content.isNotEmpty()
        }

        fun nonEmptyObj(key: String): Boolean =
            (this[key] as? JsonObject)?.isNotEmpty() == true

        return nonEmptyStr("provider") ||
            nonEmptyStr("model") ||
            nonEmptyObj("providers") ||
            nonEmptyObj("custom_providers") ||
            nonEmptyObj("llm")
    }

    /**
     * 删除一个自定义 provider。连带清理：容器为空则删除 `custom_providers` 键；所删 provider 正是当前选中项时
     * 一并清除 `provider`/`model`，避免配置指向不存在的 provider。
     */
    fun deleteCustomProvider(name: String): OcrConfig? {
        val raw = readRaw()
        val bucket = (raw["custom_providers"] as? JsonObject)?.toMutableMap() ?: return read()
        if (bucket.remove(name) == null) return read()
        if (bucket.isEmpty()) {
            raw.remove("custom_providers")
        } else {
            raw["custom_providers"] = JsonObject(bucket)
        }
        val currentProvider = (raw["provider"] as? kotlinx.serialization.json.JsonPrimitive)
            ?.takeIf { it.isString }?.content
        if (currentProvider == name) {
            raw.remove("provider")
            raw.remove("model")
        }
        return writeRaw(raw)
    }

    /**
     * 在隔离的临时 HOME 上执行 `ocr llm test`，不触及用户真正的配置文件。
     * 返回 (是否成功, 失败原因)。
     */
    fun testWithEntries(entries: List<ConfigEntry>): Pair<Boolean, String?> {
        val draft = applyConfigEntries(readRaw(), entries)
        val testHome = runCatching {
            Files.createTempDirectory("ocr-test-home-").toFile()
        }.getOrElse {
            return false to HostStrings.t(
                currentIdeLocale(),
                "ext.config.tempDirFailed",
                "message" to it.message.orEmpty(),
            )
        }

        return try {
            val configDir = File(testHome, ".opencodereview")
            Files.createDirectories(configDir.toPath())
            trySetPosixPermissions(configDir, "rwx------")
            val configFile = File(configDir, "config.json")
            configFile.writeText(draft.toPrettyJson())
            trySetPosixPermissions(configFile, "rw-------")
            cli.testConnection(home = testHome, configPath = configFile)
        } catch (e: Exception) {
            false to (e.message ?: e.javaClass.simpleName)
        } finally {
            testHome.deleteRecursively()
        }
    }

    /** 写单个配置项，返回写后的配置。CLI 退出码非 0 时 [CliService.runRaw] 会抛 [CliException]。 */
    fun set(key: String, value: String): OcrConfig? {
        cli.runRaw(toConfigSetArgs(key, value), cwd, {})
        return read()
    }

    /**
     * 按顺序写多个配置项。顺序有意义：`provider` 须在 `model` 前生效，否则 model 写到顶层而非 provider 条目
     * （见 [applyConfigEntries] 的 model 分支）。中途失败直接抛出、已写入部分保留。
     */
    fun setMany(entries: List<ConfigEntry>): OcrConfig? {
        for (entry in entries) {
            cli.runRaw(toConfigSetArgs(entry.key, entry.value), cwd, {})
        }
        return read()
    }

    /** Windows 无 POSIX 权限视图，静默跳过；失败仅权限未收紧，不应导致写配置失败。 */
    private fun trySetPosixPermissions(target: File, spec: String) {
        runCatching {
            Files.setPosixFilePermissions(target.toPath(), PosixFilePermissions.fromString(spec))
        }
    }
}
