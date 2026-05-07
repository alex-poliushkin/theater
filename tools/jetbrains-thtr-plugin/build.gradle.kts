plugins {
	kotlin("jvm") version "2.1.20"
	id("org.jetbrains.intellij.platform") version "2.11.0"
	id("org.jetbrains.grammarkit") version "2023.3.0.3"
}

import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import org.jetbrains.grammarkit.tasks.GenerateParserTask
import org.jetbrains.intellij.platform.gradle.IntelliJPlatformType

group = providers.gradleProperty("pluginGroup").get()
version = providers.gradleProperty("pluginVersion").get()

repositories {
	mavenCentral()
	intellijPlatform {
		defaultRepositories()
	}
}

dependencies {
	testImplementation("junit:junit:4.13.2")

	intellijPlatform {
		goland(providers.gradleProperty("platformVersion").get())
		testFramework(org.jetbrains.intellij.platform.gradle.TestFrameworkType.Platform)
	}
}

sourceSets {
	main {
		java.srcDir("src/main/gen")
	}
}

tasks.named<GenerateParserTask>("generateParser") {
	sourceFile.set(layout.projectDirectory.file("src/main/grammars/Thtr.bnf"))
	targetRootOutputDir.set(layout.projectDirectory.dir("src/main/gen"))
	pathToParser.set("com/github/alexpoliushkin/theater/thtrij/parser/ThtrParser.java")
	pathToPsiRoot.set("com/github/alexpoliushkin/theater/thtrij/psi")
	purgeOldFiles.set(true)
}

kotlin {
	compilerOptions {
		jvmTarget = JvmTarget.JVM_21
	}
}

tasks.named("compileKotlin") {
	dependsOn("generateParser")
}

tasks.named("compileJava") {
	dependsOn("generateParser")
}

tasks.register("nativePluginCheck") {
	group = "verification"
	description = "Runs native plugin tests, builds the plugin distribution and verifies declared IDE compatibility."
	dependsOn("test", "buildPlugin", "verifyPluginProjectConfiguration", "verifyPluginStructure", "verifyPlugin")
}

intellijPlatform {
	buildSearchableOptions = false

	pluginVerification {
		ides {
			create(IntelliJPlatformType.GoLand, providers.gradleProperty("platformVersion").get())
			create(IntelliJPlatformType.IntellijIdeaCommunity, providers.gradleProperty("platformVersion").get())
		}
	}

	pluginConfiguration {
		name = providers.gradleProperty("pluginName")
		version = providers.gradleProperty("pluginVersion")
		description = provider {
			"""
			Native JetBrains language plugin scaffold for Theater `.thtr` files.
			It registers the file type and prepares the IntelliJ Language API, Grammar-Kit, and native parser test surface.
			""".trimIndent()
		}
		vendor {
			name = "alex-poliushkin"
			url = "https://github.com/alex-poliushkin/theater"
		}
		ideaVersion {
			sinceBuild = "252"
		}
	}
}
