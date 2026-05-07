package com.github.alexpoliushkin.theater.thtrij

import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiNamedElement
import com.intellij.testFramework.DumbModeTestUtils
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import com.intellij.util.ProcessingContext

class ThtrReferencesTest : BasePlatformTestCase() {
	fun testLocalReferencesResolveThroughPsi() {
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

		assertResolvesTo(file, "smoke", occurrence = 1, targetText = "smoke")
		assertResolvesTo(file, "verify", occurrence = 0, targetText = "verify")
		assertResolvesTo(file, "${'$'}id", occurrence = 0, targetText = "id")
	}

	fun testScenarioInputValueReferencesResolveThroughPsi() {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage message-library

			scenario messages/make(text: string!)
			  act create
			    do action.generate
			      outputs:
			        message: ${'$'}text
			    expect message: field(values) | path("/message") == ${'$'}text
			    export message = field(values) | path("/message")
			""".trimIndent(),
		)

		assertResolvesToText(file, "${'$'}text", occurrence = 0, targetText = "text")
		assertResolvesToText(file, "${'$'}text", occurrence = 1, targetText = "text")
	}

	fun testScenarioInputsDoNotResolveAcrossScenarios() {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage message-library

			scenario source(token: string!)
			  act create
			    do action.generate
			      outputs:
			        token: ${'$'}token

			scenario other
			  act check
			    do action.generate
			      outputs:
			        token: "missing"
			    expect missing: ${'$'}token == "missing"
			""".trimIndent(),
		)

		assertNull(referenceAt(file, "${'$'}token", occurrence = 1).resolve())
	}

	fun testScenarioCallExportsResolveAgainstReferencedScenarioValues() {
		myFixture.addFileToProject(
			"theater/lib/messages/make.thtr",
			"""
			stage message-library

			scenario messages/make(text: string!)
			  act create
			    do action.generate
			      outputs:
			        message: ${'$'}text
			    export message = field(values) | path("/message")
			""".trimIndent(),
		)
		val file = myFixture.addFileToProject(
			"theater/flows/reusable-scenario/reuse-message.thtr",
			"""
			stage reusable-message-flow

			scenario verify-message(message: string!, expected: string!, actual: string!)
			  act check
			    do action.generate
			      outputs:
			        actual: ${'$'}actual
			    expect message: field(values) | path("/actual") == ${'$'}expected

			call make-message = messages/make(
			  text: "hello from Theater"
			)
			  export shared_message = ${'$'}message

			call check-message = verify-message(message: "caller-local", expected: "hello from Theater", actual: ${'$'}shared_message)
			  dependency make-message
			""".trimIndent(),
		)

		assertResolvesToText(file, "${'$'}actual", occurrence = 0, targetText = "actual")
		assertResolvesToText(file, "${'$'}expected", occurrence = 0, targetText = "expected")
		assertResolvesToPathSuffix(file, "${'$'}message", occurrence = 0, targetPathSuffix = "theater/lib/messages/make.thtr")
		assertResolvesTo(file, "${'$'}shared_message", occurrence = 0, targetText = "shared_message")
	}

	fun testRepoLibraryScenarioReferencesUseProjectIndexWhenAvailable() {
		myFixture.addFileToProject(
			"theater/lib/web/check-page.thtr",
			"""
			scenario web/check-page
			  act open
			    export status = field(status_code)
			""".trimIndent(),
		)
		val file = myFixture.addFileToProject(
			"theater/flows/http/page.thtr",
			"""
			stage page
			call check = web/check-page()
			""".trimIndent(),
		)

		val reference = referenceAt(file, "web/check-page")

		assertEquals("web/check-page", (reference.resolve() as PsiNamedElement).name)
		assertNull(reference.resolve(useProjectIndex = false))
	}

	fun testRepoLibraryScenarioReferencesSkipProjectIndexDuringDumbMode() {
		myFixture.addFileToProject(
			"theater/lib/web/check-page.thtr",
			"""
			scenario web/check-page
			  act open
			    export status = field(status_code)
			""".trimIndent(),
		)
		val file = myFixture.addFileToProject(
			"theater/flows/http/page.thtr",
			"""
			stage page
			call check = web/check-page()
			""".trimIndent(),
		)
		val reference = referenceAt(file, "web/check-page")

		DumbModeTestUtils.runInDumbModeSynchronously(project) {
			assertNull(reference.resolve())
		}
	}

	fun testUnresolvedReferencesProduceStableDiagnostics() {
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage checkout
			scenario smoke
			  act start
			    expect missing: ${'$'}missing == "42"
			    on pass -> missing-act
			call run = missing-scenario()
			""".trimIndent(),
		)

		val descriptions = myFixture.doHighlighting().mapNotNull { it.description }.toSet()

		assertTrue(descriptions.contains("Unresolved .thtr value reference: missing"))
		assertTrue(descriptions.contains("Unresolved .thtr act reference: missing-act"))
		assertTrue(descriptions.contains("Unresolved .thtr scenario reference: missing-scenario"))
	}

	private fun assertResolvesTo(file: PsiFile, text: String, occurrence: Int, targetText: String) {
		val reference = referenceAt(file, text, occurrence)

		assertEquals(targetText, (reference.resolve() as PsiNamedElement).name)
	}

	private fun assertResolvesToText(file: PsiFile, text: String, occurrence: Int, targetText: String) {
		val reference = referenceAt(file, text, occurrence)

		assertEquals(targetText, reference.resolve()?.text)
	}

	private fun assertResolvesToPathSuffix(file: PsiFile, text: String, occurrence: Int, targetPathSuffix: String) {
		val reference = referenceAt(file, text, occurrence)
		val path = reference.resolve()?.containingFile?.virtualFile?.path

		assertTrue("resolved path $path must end with $targetPathSuffix", path?.endsWith(targetPathSuffix) == true)
	}

	private fun referenceAt(file: PsiFile, text: String, occurrence: Int = 0): ThtrReference {
		val leaf = findLeaf(file, text, occurrence)
		val references = ThtrReferenceProvider()
			.getReferencesByElement(leaf, ProcessingContext())
			.filterIsInstance<ThtrReference>()
		assertEquals(text, 1, references.size)
		return references.single()
	}

	private fun findLeaf(file: PsiFile, text: String, occurrence: Int): PsiElement {
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
}
