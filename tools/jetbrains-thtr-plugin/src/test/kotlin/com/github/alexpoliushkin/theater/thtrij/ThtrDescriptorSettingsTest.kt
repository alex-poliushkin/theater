package com.github.alexpoliushkin.theater.thtrij

import com.intellij.lang.annotation.HighlightSeverity
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import com.intellij.ui.components.JBTextField
import java.awt.Component
import java.awt.Container
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import java.security.MessageDigest

class ThtrDescriptorSettingsTest : BasePlatformTestCase() {
	override fun setUp() {
		super.setUp()
		resetDescriptorSettings()
	}

	override fun tearDown() {
		try {
			resetDescriptorSettings()
		} finally {
			super.tearDown()
		}
	}

	fun testRegistryAndLockSettingsDriveCompletionAndDiagnostics() {
		addDescriptorFiles()
		val settings = ThtrProjectSettings.getInstance(project)

		settings.pluginsConfigPath = "plugins/valid-registry.json"
		settings.pluginsLockPath = "plugins/valid-registry.lock.json"

		assertTrue(capabilityLabels().contains("action.smoke.echo"))
		assertTrue(descriptorDiagnostics().isEmpty())

		settings.pluginsConfigPath = "plugins/missing-registry.json"
		settings.pluginsLockPath = ""

		assertFalse(capabilityLabels().contains("action.smoke.echo"))
		assertTrue(
			descriptorDiagnostics().contains(
				"Theater plugin registry config does not exist: plugins/missing-registry.json",
			),
		)

		settings.pluginsConfigPath = "plugins/invalid-registry.json"

		assertFalse(capabilityLabels().contains("action.smoke.echo"))
		assertTrue(
			descriptorDiagnostics().contains(
				"Theater plugin registry config has unsupported schema: plugins/invalid-registry.json",
			),
		)
	}

	fun testLockMismatchProducesDescriptorDiagnostic() {
		addDescriptorFiles()
		val settings = ThtrProjectSettings.getInstance(project)
		settings.pluginsConfigPath = "plugins/valid-registry.json"
		settings.pluginsLockPath = "plugins/mismatch-registry.lock.json"

		assertFalse(capabilityLabels().contains("action.smoke.echo"))
		assertTrue(
			descriptorDiagnostics().contains(
				"Theater plugin manifest checksum mismatch for smoke-plugin",
			),
		)
	}

	fun testConfigurableApplyUpdatesDescriptorBackedCompletion() {
		addDescriptorFiles()
		val configurable = ThtrProjectSettingsConfigurable(project)
		try {
			val fields = textFields(configurable.createComponent())
			assertEquals(2, fields.size)
			fields[0].text = "plugins/valid-registry.json"
			fields[1].text = "plugins/valid-registry.lock.json"

			assertTrue(configurable.isModified)

			configurable.apply()

			assertFalse(configurable.isModified)
			assertTrue(capabilityLabels().contains("action.smoke.echo"))
			assertTrue(descriptorDiagnostics().isEmpty())
		} finally {
			configurable.disposeUIResources()
		}
	}

	private fun capabilityLabels(): Set<String> {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage smoke
			scenario api
			  act get
			    do action.sm<caret>
			""".trimIndent(),
		)
		return ThtrCompletions.items(file, myFixture.caretOffset).map { it.label }.toSet()
	}

	private fun descriptorDiagnostics(): Set<String> {
		myFixture.configureByText(ThtrFileType.INSTANCE, "stage smoke\n")
		return myFixture.doHighlighting()
			.filter { it.severity == HighlightSeverity.ERROR }
			.mapNotNull { it.description }
			.filter { it.startsWith("Theater plugin") }
			.toSet()
	}

	private fun addDescriptorFiles() {
		val manifest = Files.readString(completionDataRoot().resolve("smoke-manifest.json"))
		myFixture.addFileToProject("plugins/smoke/manifest.json", manifest)
		myFixture.addFileToProject(
			"plugins/valid-registry.json",
			Files.readString(descriptorDataRoot().resolve("valid-registry.json")),
		)
		myFixture.addFileToProject(
			"plugins/invalid-registry.json",
			Files.readString(descriptorDataRoot().resolve("invalid-registry.json")),
		)
		myFixture.addFileToProject("plugins/valid-registry.lock.json", lockFile(sha256(manifest)))
		myFixture.addFileToProject("plugins/mismatch-registry.lock.json", lockFile("sha256:mismatch"))
	}

	private fun lockFile(manifestChecksum: String): String {
		return """
		{
		  "schema": "theater.plugin.lock/v1alpha1",
		  "plugins": {
		    "smoke-plugin": {
		      "manifest_sha256": "$manifestChecksum"
		    }
		  }
		}
		""".trimIndent()
	}

	private fun sha256(text: String): String {
		val digest = MessageDigest.getInstance("SHA-256").digest(text.toByteArray(Charsets.UTF_8))
		return "sha256:" + digest.joinToString("") { "%02x".format(it) }
	}

	private fun completionDataRoot(): Path {
		return Paths.get("src", "test", "testData", "completion").toAbsolutePath().normalize()
	}

	private fun descriptorDataRoot(): Path {
		return Paths.get("src", "test", "testData", "descriptors").toAbsolutePath().normalize()
	}

	private fun textFields(component: Component): List<JBTextField> {
		val fields = mutableListOf<JBTextField>()
		collectTextFields(component, fields)
		return fields
	}

	private fun collectTextFields(component: Component, fields: MutableList<JBTextField>) {
		if (component is JBTextField) {
			fields += component
		}
		if (component is Container) {
			for (child in component.components) {
				collectTextFields(child, fields)
			}
		}
	}

	private fun resetDescriptorSettings() {
		val settings = ThtrProjectSettings.getInstance(project)
		settings.pluginsConfigPath = ""
		settings.pluginsLockPath = ""
	}
}
