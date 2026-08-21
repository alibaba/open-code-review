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
    // 阿里云镜像 unreachable 时（境外开发者 / CI）回落到 Maven Central，保证全球可构建。
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
        // localIdePath 是"这台机器"的配置，不要提交到仓库：写在 ~/.gradle/gradle.properties 里。
        // 路径不存在时（例如换了操作系统）回退到下载 platformVersion 指定的 IDE，而不是让构建直接失败。
        val localIdePath = providers.gradleProperty("localIdePath").orNull?.takeIf(String::isNotBlank)
        val localIde = localIdePath?.let(::file)?.takeIf(File::isDirectory)
        if (localIdePath != null && localIde == null) {
            logger.warn("[ocr] localIdePath '$localIdePath' 不存在，回退到下载 IC ${providers.gradleProperty("platformVersion").get()}")
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
}

// ---------------------------------------------------------------------------
// 前端构建
//
// frontend/ 是 VS Code 扩展前端的副本（见 frontend/UPSTREAM.md），产物直接落到
// src/main/resources/webview/。processResources 依赖它，所以 ./gradlew build
// 会自动带上 npm install + npm run build。
//
// 没装 node、或者只想编 Kotlin 时：./gradlew build -PskipFrontend=true
// ---------------------------------------------------------------------------
val frontendDir = layout.projectDirectory.dir("frontend")
val webviewOutDir = layout.projectDirectory.dir("src/main/resources/webview")
val skipFrontend = providers.gradleProperty("skipFrontend").map(String::toBoolean).getOrElse(false)
val npmCommand = if (System.getProperty("os.name").startsWith("Windows", ignoreCase = true)) "npm.cmd" else "npm"

// npm 在 PATH 里的绝对路径，配置期解析一次。
// 之所以要自己找而不是直接 commandLine("npm")：nvm 装的 node 只在登录 shell 的 PATH 里，
// 而 GUI 启动的 IDEA 里跑 Gradle 时 PATH 常常是精简版，报错会是难懂的 "Cannot run program npm"。
val npmExecutable: String? = providers.environmentVariable("PATH").orElse("").get()
    .split(File.pathSeparator)
    .asSequence()
    .filter(String::isNotBlank)
    .map { File(it, npmCommand) }
    .firstOrNull { it.canExecute() }
    ?.absolutePath

val npmMissingHint = """
    [ocr] 找不到 $npmCommand（PATH 里没有）。二选一：
      · 在能跑 npm 的终端里执行 ./gradlew ...（nvm 装的 node 只在登录 shell 的 PATH 里）
      · 或者跳过前端构建：./gradlew <task> -PskipFrontend=true
        （跳过时用的是 src/main/resources/webview/ 里已有的产物，可能是旧的）
""".trimIndent()

// doFirst / onlyIf 里一律只用**局部变量**，不要直接引用上面那些脚本级的 val：
// Kotlin DSL 里脚本级 val 是脚本对象的字段，lambda 捕获它等于捕获整个脚本对象，
// 而脚本对象没法进配置缓存（"cannot serialize Gradle script object references"）。
// 同理 logger 用 task 自己的（doFirst 的 it），不用脚本的 project.logger。
val frontendInstall by tasks.registering(Exec::class) {
    val npm = npmExecutable
    val hint = npmMissingHint
    val skip = skipFrontend
    group = "frontend"
    description = "安装 frontend/ 的 npm 依赖"
    workingDir = frontendDir.asFile
    commandLine(npm ?: npmCommand, "install", "--no-audit", "--no-fund")
    doFirst { if (npm == null) throw GradleException(hint) }
    // package.json 变了才重装；node_modules 作为产物让 Gradle 判 UP-TO-DATE。
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
    description = "把 frontend/ 打包到 src/main/resources/webview/"
    dependsOn(frontendInstall)
    workingDir = dir
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
        println("[ocr] 构建前端：$dir")
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

    // 把 `-Pocr.xxx=...` 转发成沙箱 IDE 的系统属性。
    // `./gradlew runIde -Docr.devtools=true` 是**没用**的：那个 -D 只加在 Gradle 自己的 JVM 上，
    // 沙箱 IDE 是另一个进程，读不到——这个转发补上之前，devtools 开关其实一直是失效的。
    // 插件里靠 System.getProperty 取的开关都得走这里。
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
