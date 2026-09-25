// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
 * Reads and writes `~/.opencodereview/config.json`. Division of labor:
 * single-key writes go through the CLI (`ocr config set`), which validates and normalizes;
 * only "delete a custom provider" edits the file directly -- the CLI has no matching unset subcommand.
 */
class ConfigService(
    private val cli: CliService,
    /** Working directory for `ocr config set`. That subcommand does not depend on cwd, so any stable value works. */
    private val cwd: File = File(System.getProperty("user.dir")),
) {

    private fun configPath(): File =
        File(File(System.getProperty("user.home"), ".opencodereview"), "config.json")

    /** Reads the file and converts it to the plugin's internal camelCase config; returns null when the file is missing or invalid. */
    fun read(): OcrConfig? {
        val path = configPath()
        if (!path.isFile) return null
        return runCatching { parseConfig(path.readText()) }.getOrElse {
            thisLogger().warn("[ocr] Failed to parse config, treating as no config: ${path.absolutePath}", it)
            null
        }
    }

    /** Reads the raw snake_case JSON, preserving all unknown fields. Parse failures are logged (same as [read]) and an empty config is returned. */
    private fun readRaw(): RawConfig {
        val path = configPath()
        if (!path.isFile) return emptyRawConfig()
        return runCatching { parseRawConfig(path.readText()) }.getOrElse {
            // Logged to ease troubleshooting; an empty config is returned, and downstream writeRaw/deleteCustomProvider bail
            // when the bucket is missing, so this broken file is never overwritten.
            thisLogger().warn("[ocr] Failed to parse raw config (treated as empty; writes will bail): ${path.absolutePath}", it)
            emptyRawConfig()
        }
    }

    /** Writes the raw config back. When the whole config is empty, delete the file instead of writing `{}` --
     *  keeping an empty file would make the CLI believe it "has been configured".
     *  @Synchronized serializes writes: it avoids tmp-file-name collisions and interleaving between concurrent
     *  writers (delete provider vs the setMany rollback). */
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
            // Tighten even a pre-existing directory (the CLI or older versions may leave 755; the directory holds api_key and must be 700).
            trySetPosixPermissions(dir, "rwx------")
            // Atomic write + permissions tightened first: a uniquely named temp file avoids collisions across
            // instances/IDE windows; the empty file is tightened to rw------- before api_key is written, so the
            // key never sits in a readable file at any point.
            val tmpPath = Files.createTempFile(dir.toPath(), "config-", ".tmp")
            val tmp = tmpPath.toFile()
            trySetPosixPermissions(tmp, "rw-------")
            try {
                tmp.writeText(raw.toPrettyJson())
            } catch (e: IOException) {
                // If writeText fails (e.g. disk full), tmp may already hold a partial api_key; permissions are
                // already tightened to 600, but the file must be deleted so no residue leaks.
                runCatching { Files.deleteIfExists(tmpPath) }
                throw e
            }
            try {
                // Prefer an atomic move; filesystems without atomic-move support (some Windows/network drives) fall back to REPLACE_EXISTING.
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

    /** Whether the config still has real content. Empty strings do not count -- a blank string is falsy in this check. */
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
     * Deletes a custom provider, with cascading cleanup: the `custom_providers` key is removed when its container
     * becomes empty, and when the deleted provider is the currently selected one, `provider`/`model` are cleared
     * too so the config does not point at a provider that no longer exists.
     * The whole read-modify-write is locked: concurrent deletes (a double click) would overwrite each other and lose one delete.
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
     * Runs `ocr llm test` on an isolated temporary HOME, never touching the user's real config file.
     * Returns (success, failure reason).
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
            cli.testConnection(home = testHome)
        } catch (e: Exception) {
            false to (e.message ?: e.javaClass.simpleName)
        } finally {
            testHome.deleteRecursively()
        }
    }

    /** Writes a single config entry and returns the config after the write. [CliService.runRaw] throws [CliException] when the CLI exits non-zero. */
    @Synchronized
    fun set(key: String, value: String): OcrConfig? {
        cli.runRaw(toConfigSetArgs(key, value), cwd, {})
        return read()
    }

    /**
     * Writes several config entries in order. Order matters: `provider` must take effect before `model`,
     * otherwise the model lands at the top level instead of inside the provider entry
     * (see the model branch of [applyConfigEntries]). On a mid-way failure, roll back to the snapshot taken
     * before setMany to avoid a half-applied state (e.g. provider changed but api_key not written).
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
            // applied holds only the entries that succeeded; the failed one is the next entry, entries[applied.size].
            // Logging that entry's key is what makes the message accurate.
            thisLogger().warn("[ocr] setMany failed at '${entries.getOrNull(applied.size)?.key ?: "?"}', rolling back", e)
            // Roll back only when the snapshot has content: with an empty snapshot (original config missing/corrupt),
            // writeRaw would delete the file and lose user data instead.
            if (snapshot.hasContent()) writeRaw(snapshot)
            throw e
        }
    }

    /** Windows has no POSIX permission view, so it is skipped silently; a failure only leaves permissions looser and must not fail the config write. */
    private fun trySetPosixPermissions(target: File, spec: String) {
        // On Windows setPosixFilePermissions always throws UnsupportedOperationException, and logging it on every
        // write would spam the log -- skip directly.
        if (SystemInfo.isWindows) return
        runCatching {
            Files.setPosixFilePermissions(target.toPath(), PosixFilePermissions.fromString(spec))
        }.onFailure { thisLogger().warn("[ocr] Failed to set permissions $spec on ${target.absolutePath}", it) }
    }
}
