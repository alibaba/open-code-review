// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "2.1.0"
    id("org.jetbrains.kotlin.plugin.serialization") version "2.1.0"
    id("org.jetbrains.intellij.platform") version "2.11.0"
}

group = "com.alibaba"
version = providers.gradleProperty("pluginVersion").get()

repositories {
    maven {
        url = uri("https://maven.aliyun.com/repository/public")
    }
    // Falls back to Maven Central when the Aliyun mirror is unreachable (developers
    // outside China, CI), so the build works from anywhere.
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

// Configure IntelliJ Platform Gradle Plugin
// Read more: https://plugins.jetbrains.com/docs/intellij/tools-intellij-platform-gradle-plugin.html
dependencies {
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    testImplementation(kotlin("test-junit"))
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")

    intellijPlatform {
        // localIdePath is a per-machine setting and must not be committed: put it in
        // ~/.gradle/gradle.properties.
        // When the path does not exist (after switching operating systems, say) the build
        // falls back to downloading the IDE named by platformVersion instead of failing.
        val localIdePath = providers.gradleProperty("localIdePath").orNull?.takeIf(String::isNotBlank)
        val localIde = localIdePath?.let(::file)?.takeIf(File::isDirectory)
        if (localIdePath != null && localIde == null) {
            logger.warn("[ocr] localIdePath '$localIdePath' does not exist, falling back to downloading IC ${providers.gradleProperty("platformVersion").get()}")
        }
        if (localIde != null) local(localIde.absolutePath) else create("IC", providers.gradleProperty("platformVersion"))
        testFramework(org.jetbrains.intellij.platform.gradle.TestFrameworkType.Platform)
    }
}

intellijPlatform {
    pluginConfiguration {
        ideaVersion {
            sinceBuild = "251"
        }

        changeNotes = "Initial IntelliJ IDEA implementation."
    }

    // verifyPlugin requires the target IDE to be declared explicitly (2.x no longer infers it);
    // the same version as the build, so the local cache can be reused.
    // This affects only the verifyPlugin check task, not packaging: the produced zip is unchanged.
    pluginVerification {
        ides {
            ide("IC", providers.gradleProperty("platformVersion").get())
        }
    }
}

// ---------------------------------------------------------------------------
// Frontend build
//
// frontend/ is a copy of the VS Code extension frontend, and its output goes
// straight into src/main/resources/webview/. processResources depends on it, so
// ./gradlew build pulls in npm install + npm run build automatically.
//
// Without node installed, or to compile only Kotlin: ./gradlew build -PskipFrontend=true
// ---------------------------------------------------------------------------
val frontendDir = layout.projectDirectory.dir("../frontend")
val webviewOutDir = layout.projectDirectory.dir("src/main/resources/webview")
val skipFrontend = providers.gradleProperty("skipFrontend").map(String::toBoolean).getOrElse(false)
val npmCommand = if (System.getProperty("os.name").startsWith("Windows", ignoreCase = true)) "npm.cmd" else "npm"

// The absolute path of npm on PATH, resolved once at configuration time.
// Resolved here instead of passing commandLine("npm") directly because node installed by nvm
// lives only on the login shell's PATH, while Gradle run from a GUI-launched IDEA often sees a
// reduced PATH, where the failure is the cryptic "Cannot run program npm".
val npmExecutable: String? = providers.environmentVariable("PATH").orElse("").get()
    .split(File.pathSeparator)
    .asSequence()
    .filter(String::isNotBlank)
    .map { File(it, npmCommand) }
    .firstOrNull { it.canExecute() }
    ?.absolutePath

val npmMissingHint = """
    [ocr] Cannot find $npmCommand (it is not on PATH). Either:
      · run ./gradlew ... from a terminal that can run npm (node installed by nvm is only on the login shell's PATH)
      · or skip the frontend build: ./gradlew <task> -PskipFrontend=true
        (skipping reuses whatever is already in src/main/resources/webview/, which may be stale)
""".trimIndent()

// doFirst / onlyIf use **local variables only**, never the script-level vals above:
// in the Kotlin DSL a script-level val is a field of the script object, so a lambda that captures
// it captures the whole script object, and script objects cannot go into the configuration cache
// ("cannot serialize Gradle script object references").
// For the same reason use the task's own logger (doFirst's it), not the script's project.logger.
val frontendInstall by tasks.registering(Exec::class) {
    val npm = npmExecutable
    val hint = npmMissingHint
    val skip = skipFrontend
    group = "frontend"
    description = "Install frontend/ npm dependencies"
    workingDir = frontendDir.asFile
    commandLine(npm ?: npmCommand, "install", "--no-audit", "--no-fund")
    doFirst { if (npm == null) throw GradleException(hint) }
    // Reinstall only when package.json changes; declaring node_modules as an output lets Gradle
    // mark the task UP-TO-DATE.
    inputs.file(frontendDir.file("package.json"))
    outputs.dir(frontendDir.dir("node_modules"))
    onlyIf { !skip }
}

val frontendBuild by tasks.registering(Exec::class) {
    val npm = npmExecutable
    val hint = npmMissingHint
    val skip = skipFrontend
    val dir = frontendDir.asFile
    group = "frontend"
    description = "Bundle frontend/ into src/main/resources/webview/"
    dependsOn(frontendInstall)
    workingDir = dir
    environment("OCR_TARGET", "idea")
    commandLine(npm ?: npmCommand, "run", "build")
    inputs.dir(frontendDir.dir("src"))
    inputs.files(
        frontendDir.file("package.json"),
        frontendDir.file("tsconfig.json"),
        frontendDir.file("webpack.config.js"),
    )
    outputs.dir(webviewOutDir)
    onlyIf { !skip }
    doFirst {
        if (npm == null) throw GradleException(hint)
        println("[ocr] Building frontend: $dir")
    }
}

tasks {
    withType<JavaCompile> {
        sourceCompatibility = "21"
        targetCompatibility = "21"
    }

    processResources {
        dependsOn(frontendBuild)
    }

    // Forwards `-Pocr.xxx=...` as system properties on the sandbox IDE.
    // `./gradlew runIde -Docr.devtools=true` does **not** work: that -D only reaches Gradle's own
    // JVM, while the sandbox IDE is a separate process and cannot read it — before this forwarding
    // was added, the devtools switch was in fact dead.
    // Every plugin switch read through System.getProperty has to go through here.
    runIde {
        listOf("ocr.devtools").forEach { key ->
            providers.gradleProperty(key).orNull?.let { systemProperty(key, it) }
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_21)
    }
}
