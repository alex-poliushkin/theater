package com.github.alexpoliushkin.theater.thtrij

import com.intellij.codeInsight.navigation.actions.GotoDeclarationAction
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiNamedElement
import com.intellij.psi.search.LocalSearchScope
import com.intellij.psi.search.searches.ReferencesSearch
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import com.intellij.util.ProcessingContext
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrNavigationRenameTest : BasePlatformTestCase() {
	fun testGotoDeclarationUsesRegisteredReferences() {
		val cases = listOf(
			"scenario call" to """
				stage checkout
				scenario smoke
				  act start
				    on pass -> verify
				  act verify
				    expect ok: field(status_code) == 200
				call run = sm<caret>oke()
			""".trimIndent() to "smoke",
			"transition" to """
				stage checkout
				scenario smoke
				  act start
				    on pass -> ver<caret>ify
				  act verify
				    expect ok: field(status_code) == 200
			""".trimIndent() to "verify",
			"value reference to export" to """
				stage checkout
				scenario smoke
				  act start
				    export id = field(body)
				  act verify
				    expect has-id: ${'$'}i<caret>d == "42"
			""".trimIndent() to "id",
		)

		for ((labelAndText, targetName) in cases) {
			val (label, text) = labelAndText
			myFixture.configureByText(ThtrFileType.INSTANCE, text)
			val targets = GotoDeclarationAction.findAllTargetElements(project, myFixture.editor, myFixture.caretOffset)

			assertEquals(label, 1, targets.size)
			assertEquals(label, targetName, (targets.single() as PsiNamedElement).name)
		}
	}

	fun testFindUsagesSearchesSupportedDeclarations() {
		val file = myFixture.configureByText(ThtrFileType.INSTANCE, Files.readString(findUsagesDataRoot().resolve("symbols.thtr")))
		val provider = ThtrFindUsagesProvider()

		val scenario = referenceAt(file, "smoke", occurrence = 1).resolve() as PsiNamedElement
		val act = referenceAt(file, "verify", occurrence = 0).resolve() as PsiNamedElement
		val value = referenceAt(file, "$" + "id").resolve() as PsiNamedElement
		val selector = findLeaf(file, "field", 0)

		assertTrue(provider.canFindUsagesFor(scenario))
		assertEquals("scenario", provider.getType(scenario))
		assertEquals("smoke", provider.getDescriptiveName(scenario))
		assertEquals(listOf("smoke"), referenceTexts(scenario))

		assertTrue(provider.canFindUsagesFor(act))
		assertEquals("act", provider.getType(act))
		assertEquals(listOf("verify"), referenceTexts(act))

		assertTrue(provider.canFindUsagesFor(value))
		assertEquals("value", provider.getType(value))
		assertEquals(listOf("$" + "id"), referenceTexts(value))

		assertFalse(provider.canFindUsagesFor(selector))
	}

	fun testRenameAtDeclarationUpdatesSupportedLocalReferences() {
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			Files.readString(renameDataRoot().resolve("local-symbols.thtr")).replace("export id", "export <caret>id"),
		)
		myFixture.renameElementAtCaret("order_id")

		assertTrue(myFixture.file.text.contains("expect has-id: ${'$'}order_id == \"42\""))
		assertFalse(myFixture.file.text.contains("export id = field(body)"))

		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			Files.readString(renameDataRoot().resolve("local-symbols.thtr")).replace("act verify", "act <caret>verify"),
		)
		myFixture.renameElementAtCaret("confirm")

		assertTrue(myFixture.file.text.contains("on pass -> confirm"))
		assertFalse(myFixture.file.text.contains("act verify"))

		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			Files.readString(renameDataRoot().resolve("local-symbols.thtr")).replace("scenario smoke", "scenario <caret>smoke"),
		)
		myFixture.renameElementAtCaret("checkout/smoke")

		assertTrue(myFixture.file.text.contains("call run = checkout/smoke()"))
		assertFalse(myFixture.file.text.contains("scenario smoke"))
	}

	fun testNamesValidatorAndRenameVetoBoundUnsupportedScopes() {
		val validator = ThtrNamesValidator()
		assertTrue(validator.isIdentifier("checkout/smoke", project))
		assertTrue(validator.isIdentifier("order_id", project))
		assertFalse(validator.isIdentifier("123bad", project))
		assertFalse(validator.isIdentifier("stage", project))

		val file = myFixture.addFileToProject(
			"theater/lib/web/check-page.thtr",
			"""
			scenario web/check-page
			  act open
			    export status = field(status_code)
			""".trimIndent(),
		)
		val declaration = findLeaf(file, "web/check-page", 0)
		myFixture.configureFromExistingVirtualFile(file.virtualFile)
		val before = myFixture.file.text

		assertTrue(ThtrRenameVetoCondition().value(declaration))
		runCatching {
			myFixture.renameElement(declaration.parent, "web/renamed")
		}
		assertEquals(before, myFixture.file.text)
	}

	private fun referenceTexts(element: PsiNamedElement): List<String> {
		return ReferencesSearch.search(element, LocalSearchScope(myFixture.file))
			.findAll()
			.map { it.element.text }
			.sorted()
	}

	private fun referenceAt(file: PsiFile, text: String, occurrence: Int = 0): ThtrReference {
		val leaf = findLeaf(file, text, occurrence)
		return ThtrReferenceProvider()
			.getReferencesByElement(leaf, ProcessingContext())
			.filterIsInstance<ThtrReference>()
			.single()
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

	private fun findUsagesDataRoot(): Path {
		return Paths.get("src", "test", "testData", "findUsages").toAbsolutePath().normalize()
	}

	private fun renameDataRoot(): Path {
		return Paths.get("src", "test", "testData", "rename").toAbsolutePath().normalize()
	}
}
