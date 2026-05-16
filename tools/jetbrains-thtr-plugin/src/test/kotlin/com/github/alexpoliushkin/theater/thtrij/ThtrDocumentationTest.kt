package com.github.alexpoliushkin.theater.thtrij

import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrDocumentationTest : BasePlatformTestCase() {
	fun testQuickDocumentationTargetsBuiltInCapabilityRefs() {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage smoke
			scenario api
			  act get
			    do action.http<caret>(method: "GET", url: "/health")
			  act prepare
			    prop start_date = generate.date(format: "basic")
			""".trimIndent(),
		)
		val leaf = findLeaf(file, "action.http")
		val target = ThtrDocumentationTargetProvider().documentationTarget(leaf, leaf)

		assertNotNull(target)
		assertTrue(target!!.computeDocumentationHint()?.contains("built-in action") == true)

		val documentation = target.computeDocumentation().toString()
		assertTrue(documentation.contains("action.http(method: string, url: string, timeout?: duration)"))

		val dateLeaf = findLeaf(file, "generate.date")
		val dateTarget = ThtrDocumentationTargetProvider().documentationTarget(dateLeaf, dateLeaf)
		assertNotNull(dateTarget)
		assertTrue(dateTarget!!.computeDocumentationHint()?.contains("built-in generator") == true)
		assertTrue(dateTarget.computeDocumentation().toString().contains("generate.date(format?: iso|basic, offset?: duration)"))
	}

	fun testQuickDocumentationTargetsDescriptorBackedPluginRefs() {
		addSmokePluginManifest()
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage smoke
			scenario plugin
			  act echo
			    do action.smoke.echo<caret>(value: "hello")
			""".trimIndent(),
		)
		val leaf = findLeaf(file, "action.smoke.echo")
		val target = ThtrDocumentationTargetProvider().documentationTarget(leaf, leaf)

		assertNotNull(target)
		assertTrue(target!!.computeDocumentationHint()?.contains("plugin action from smoke-plugin@0.2.0") == true)

		val documentation = target.computeDocumentation().toString()
		assertTrue(documentation.contains("Emit a simple echo output"))
		assertTrue(documentation.contains("value"))
	}

	fun testQuickDocumentationTargetsMajorThtrConstructs() {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage checkout
			scenario smoke
			  act start
			    prop token = generate.email()
			    log response = object { token: ${'$'}token }
			    export id = field(body)
			    capture_auth bearer = field(body)
			call run = smoke()
			""".trimIndent(),
		)
		val provider = ThtrDocumentationTargetProvider()
		val cases = listOf(
			"checkout" to ".thtr stage" to "stage checkout",
			"smoke" to ".thtr scenario" to "scenario smoke",
			"start" to ".thtr act" to "act start",
			"run" to ".thtr call" to "Target scenario",
			"token" to ".thtr prop" to "prop token",
			"response" to ".thtr log" to "log response",
			"id" to ".thtr export" to "export id",
			"bearer" to ".thtr auth capture" to "capture_auth bearer",
		)

		for ((textAndKind, expectedDocumentation) in cases) {
			val (text, expectedHint) = textAndKind
			val target = provider.documentationTarget(findLeaf(file, text), null)
			assertNotNull(text, target)
			assertTrue(text, target!!.computeDocumentationHint()?.contains(expectedHint) == true)
			assertTrue(text, target.computeDocumentation().toString().contains(expectedDocumentation))
		}
	}

	fun testQuickDocumentationTargetsReferencedThtrDeclarations() {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage checkout
			scenario smoke
			  act start
			    export id = field(body)
			    on pass -> verify
			  act verify
			    expect has-id: ${'$'}id == "42"
			call run = smoke()
			""".trimIndent(),
		)
		val provider = ThtrDocumentationTargetProvider()
		val cases = listOf(
			"smoke" to 1 to "scenario smoke",
			"verify" to 0 to "act verify",
			"$" + "id" to 0 to "export id",
		)

		for ((textAndOccurrence, expectedDocumentation) in cases) {
			val (text, occurrence) = textAndOccurrence
			val leaf = findLeaf(file, text, occurrence)
			val target = provider.documentationTarget(leaf, leaf)
			assertNotNull(text, target)
			assertTrue(text, target!!.computeDocumentation().toString().contains(expectedDocumentation))
		}
	}

	private fun addSmokePluginManifest() {
		myFixture.addFileToProject(
			"plugins/smoke/manifest.json",
			Files.readString(completionDataRoot().resolve("smoke-manifest.json")),
		)
	}

	private fun findLeaf(file: PsiFile, text: String, occurrence: Int = 0): PsiElement {
		val matches = mutableListOf<PsiElement>()
		collectLeaves(file, text, matches)
		assertTrue("missing leaf $text", matches.size > occurrence)
		return matches[occurrence]
	}

	private fun collectLeaves(element: PsiElement, text: String, matches: MutableList<PsiElement>) {
		if (element.firstChild == null) {
			if (element.text == text) {
				matches += element
			}
			return
		}

		var child = element.firstChild
		while (child != null) {
			collectLeaves(child, text, matches)
			child = child.nextSibling
		}
	}

	private fun completionDataRoot(): Path {
		return Paths.get("src", "test", "testData", "completion").toAbsolutePath().normalize()
	}
}
