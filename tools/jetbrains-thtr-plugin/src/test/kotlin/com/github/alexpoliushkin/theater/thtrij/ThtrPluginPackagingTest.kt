package com.github.alexpoliushkin.theater.thtrij

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Paths
import javax.xml.parsers.DocumentBuilderFactory

private val FORBIDDEN_PRODUCT_DEPENDENCIES = listOf(
	"com.intellij.modules.lsp",
	"com.intellij.modules.ultimate",
	"com.intellij.modules.goland",
	"org.jetbrains.plugins.go",
)

class ThtrPluginPackagingTest {
	@Test
	fun pluginDescriptorDeclaresOnlyPlatformLanguageModules() {
		val pluginXml = Files.readString(pluginXmlPath())
		val document = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(pluginXmlPath().toFile())
		val dependencies = document.getElementsByTagName("depends").asTextList()

		assertEquals(listOf("com.intellij.modules.platform", "com.intellij.modules.lang"), dependencies)
		for (forbidden in FORBIDDEN_PRODUCT_DEPENDENCIES) {
			assertFalse("plugin.xml must not declare $forbidden", pluginXml.contains(forbidden))
		}
	}

	@Test
	fun nativePluginCheckRunsDeclaredCompatibilityMatrix() {
		val buildScript = Files.readString(Paths.get("build.gradle.kts").toAbsolutePath().normalize())

		assertTrue(buildScript.contains("\"verifyPlugin\""))
		assertTrue(buildScript.contains("IntelliJPlatformType.GoLand"))
		assertTrue(buildScript.contains("IntelliJPlatformType.IntellijIdeaCommunity"))
	}

	private fun pluginXmlPath() =
		Paths.get("src", "main", "resources", "META-INF", "plugin.xml").toAbsolutePath().normalize()

	private fun org.w3c.dom.NodeList.asTextList(): List<String> {
		return (0 until length).map { index -> item(index).textContent.trim() }
	}
}
