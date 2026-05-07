package com.github.alexpoliushkin.theater.thtrij

import com.intellij.ide.structureView.TreeBasedStructureViewBuilder
import com.intellij.ide.util.treeView.smartTree.TreeElement
import com.intellij.lang.folding.FoldingDescriptor
import com.intellij.openapi.editor.Document
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrStructureFoldingTest : BasePlatformTestCase() {
	fun testStructureViewShowsStagesScenariosCallsAndScenarioActs() {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			Files.readString(testDataRoot().resolve("structure-folding.thtr")),
		)
		val builder = ThtrStructureViewFactory().getStructureViewBuilder(file) as TreeBasedStructureViewBuilder
		val model = builder.createStructureViewModel(null)
		val root = model.root
		val rootLabels = labels(root)

		assertEquals(
			listOf(
				"stage checkout",
				"scenario smoke",
				"call shared -> web/check-page",
				"scenario cleanup",
			),
			rootLabels,
		)

		val smoke = root.children[1]
		assertEquals(listOf("act start", "act verify"), labels(smoke))
	}

	fun testFoldingBuilderCoversScenariosActsObjectListsAndLargeDataBlocks() {
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			Files.readString(testDataRoot().resolve("structure-folding.thtr")),
		)
		val document = myFixture.editor.document
		val foldTexts = ThtrFoldingBuilder()
			.buildFoldRegions(file, document, quick = false)
			.map { foldedText(document, it) }

		assertTrue(foldTexts.any { it.contains("act start") && it.contains("act verify") })
		assertTrue(foldTexts.any { it.contains("do action.http(") && it.contains("on pass -> verify") })
		assertTrue(foldTexts.any { it.contains("X_Request") })
		assertTrue(foldTexts.any { it.contains("\"admin\"") && it.contains("\"operator\"") })
		assertTrue(foldTexts.any { it.contains("method: \"POST\"") && it.contains("json: object") })
	}

	private fun labels(element: TreeElement): List<String> {
		return element.children.map { it.presentation.presentableText.orEmpty() }
	}

	private fun foldedText(document: Document, descriptor: FoldingDescriptor): String {
		return document.text.substring(descriptor.range.startOffset, descriptor.range.endOffset)
	}

	private fun testDataRoot(): Path {
		return Paths.get("src", "test", "testData", "structure").toAbsolutePath().normalize()
	}
}
