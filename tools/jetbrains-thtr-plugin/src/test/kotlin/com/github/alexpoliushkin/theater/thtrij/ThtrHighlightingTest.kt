package com.github.alexpoliushkin.theater.thtrij

import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiFileFactory
import com.intellij.psi.TokenType
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrHighlightingTest : BasePlatformTestCase() {
	fun testSyntaxHighlightingSnapshot() {
		val source = readHighlightingFixture("syntax.thtr")

		assertEquals(readHighlightingFixture("syntax.expected").trim(), renderSyntaxHighlights(source))
	}

	fun testSemanticHighlightingSnapshot() {
		val source = readHighlightingFixture("semantic.thtr")
		val file = parse(source)

		assertEquals(readHighlightingFixture("semantic.expected").trim(), renderSemanticHighlights(file))
	}

	fun testScenarioLogIdIsHighlightedAsDeclarationWithoutValueSemantics() {
		val file = parse(
			"""
			stage smoke
			scenario login
			  act submit
			    do action.http
			    log response = object { status: field(status_code) }
			""".trimIndent(),
		)
		val response = findLeaf(file, "response")

		assertEquals(ThtrHighlighting.DECLARATION_ID, ThtrHighlighting.semanticKey(response))
		assertNull(ThtrSymbols.declarationKind(response))
	}

	private fun parse(source: String) =
		PsiFileFactory.getInstance(project).createFileFromText("highlight.thtr", ThtrFileType.INSTANCE, source)

	private fun readHighlightingFixture(name: String): String {
		return Files.readString(highlightingDataRoot().resolve(name))
	}

	private fun findLeaf(file: PsiFile, text: String): PsiElement {
		val matches = mutableListOf<PsiElement>()
		collectLeaves(file, text, matches)
		assertTrue("missing leaf $text", matches.isNotEmpty())
		return matches.first()
	}

	private fun highlightingDataRoot(): Path {
		return Paths.get("src", "test", "testData", "highlighting").toAbsolutePath().normalize()
	}

	private fun renderSyntaxHighlights(source: String): String {
		val highlighter = ThtrSyntaxHighlighter()
		val lexer = highlighter.highlightingLexer
		val lines = mutableListOf<String>()
		lexer.start(source)
		while (true) {
			val tokenType = lexer.tokenType ?: break
			if (tokenType != TokenType.WHITE_SPACE) {
				val keys = highlighter.getTokenHighlights(tokenType).map { it.externalName }
				if (keys.isNotEmpty()) {
					lines += "${escape(source.substring(lexer.tokenStart, lexer.tokenEnd))} -> ${keys.joinToString("+")}"
				}
			}
			lexer.advance()
		}
		return lines.joinToString("\n")
	}

	private fun renderSemanticHighlights(file: PsiFile): String {
		val lines = mutableListOf<String>()
		collectSemanticHighlights(file, lines)
		return lines.joinToString("\n")
	}

	private fun collectSemanticHighlights(element: PsiElement, lines: MutableList<String>) {
		if (element.firstChild == null) {
			val key = ThtrHighlighting.semanticKey(element)
			if (key != null) {
				lines += "${element.textRange.startOffset}-${element.textRange.endOffset} ${escape(element.text)} -> ${key.externalName}"
			}
			return
		}

		var child = element.firstChild
		while (child != null) {
			collectSemanticHighlights(child, lines)
			child = child.nextSibling
		}
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

	private fun escape(text: String): String {
		return text
			.replace("\r", "\\r")
			.replace("\n", "\\n")
	}
}
