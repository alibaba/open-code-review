package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.ConfigEntry
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.util.SystemInfo
import kotlinx.serialization.json.JsonObject
import kotlin.jvm.Synchronized
import java.io.IOException
import java.io.File
import java.nio.file.Files
import java.nio.file.StandardCopyOption
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

    /** 读取原始 snake_case JSON，保留所有未知字段。解析失败记日志（与 [read] 一致），返回空配置。 */
    private fun readRaw(): RawConfig {
        val path = configPath()
        if (!path.isFile) return emptyRawConfig()
        return runCatching { parseRawConfig(path.readText()) }.getOrElse {
            // 记日志便于排查；返回空配置，下游 writeRaw/deleteCustomProvider 在 bucket 取不到时会 bail，不会覆盖这份坏文件。
            thisLogger().warn("[ocr] Failed to parse raw config (treated as empty; writes will bail): ${path.absolutePath}", it)
            emptyRawConfig()
        }
    }

    /** 写回原始配置。整份配置为空时删文件而非写 `{}`——保留空文件会让 CLI 认为"已配置过"。
     *  @Synchronized 串行化写盘：避免并发写（删除 provider vs setMany 回滚）的固定 tmp 文件名碰撞与交错。 */
    @Synchronized
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
            }
            // 已存在的目录也收紧（CLI 或旧版可能留 755，目录含 api_key 须 700）。
            trySetPosixPermissions(dir, "rwx------")
            // 原子写 + 先收紧权限：唯一名临时文件避免跨实例/跨 IDE 窗口碰撞；空文件先收紧 rw------- 再写 api_key，全程不暴露在可读文件里。
            val tmpPath = Files.createTempFile(dir.toPath(), "config-", ".tmp")
            val tmp = tmpPath.toFile()
            trySetPosixPermissions(tmp, "rw-------")
            try {
                tmp.writeText(raw.toPrettyJson())
            } catch (e: IOException) {
                // writeText 失败（如盘满）时 tmp 可能已含部分 api_key；600 权限已收紧，但须删掉避免残留泄漏。
                runCatching { Files.deleteIfExists(tmpPath) }
                throw e
            }
            try {
                // 优先原子 move；不支持原子 move 的 FS（部分 Windows/网络盘）退化为 REPLACE_EXISTING。
                try {
                    Files.move(tmpPath, path.toPath(), StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE)
                } catch (e: java.nio.file.AtomicMoveNotSupportedException) {
                    Files.move(tmpPath, path.toPath(), StandardCopyOption.REPLACE_EXISTING)
                }
            } catch (e: IOException) {
                runCatching { Files.deleteIfExists(tmpPath) }.onFailure { thisLogger().warn("[ocr] Failed to clean tmp config after move failure", it) }
                throw e
            }
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
     * 整段 read-modify-write 加锁：并发删除（用户连点）会互相覆盖、丢一个删除。
     */
    @Synchronized
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
     * （见 [applyConfigEntries] 的 model 分支）。中途失败回滚到 setMany 前的快照，避免半应用（如 provider 改了但 api_key 没写）。
     */
    @Synchronized
    fun setMany(entries: List<ConfigEntry>): OcrConfig? {
        val snapshot = readRaw()
        val applied = mutableListOf<ConfigEntry>()
        try {
            for (entry in entries) {
                cli.runRaw(toConfigSetArgs(entry.key, entry.value), cwd, {})
                applied += entry
            }
            return read()
        } catch (e: Exception) {
            // applied 只含已成功的条目，失败的是下一条 entries[applied.size]；用它的 key 记日志才准确。
            thisLogger().warn("[ocr] setMany failed at '${entries.getOrNull(applied.size)?.key ?: "?"}', rolling back", e)
            // 只在快照有内容时回滚写回；snapshot 为空（原配置缺失/损坏）时 writeRaw 会删文件，反而丢用户数据。
            if (snapshot.hasContent()) writeRaw(snapshot)
            throw e
        }
    }

    /** Windows 无 POSIX 权限视图，静默跳过；失败仅权限未收紧，不应导致写配置失败。 */
    private fun trySetPosixPermissions(target: File, spec: String) {
        // Windows 上 setPosixFilePermissions 必抛 UnsupportedOperationException，每次写都记日志会刷屏——直接跳过。
        if (SystemInfo.isWindows) return
        runCatching {
            Files.setPosixFilePermissions(target.toPath(), PosixFilePermissions.fromString(spec))
        }.onFailure { thisLogger().warn("[ocr] Failed to set permissions $spec on ${target.absolutePath}", it) }
    }
}
