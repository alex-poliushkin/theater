package com.github.alexpoliushkin.theater.thtrij

import com.intellij.lang.folding.FoldingBuilderEx
import com.intellij.lang.folding.FoldingDescriptor
import com.intellij.openapi.editor.Document
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.util.TextRange
import com.intellij.psi.PsiElement

class ThtrFoldingBuilder : FoldingBuilderEx(), DumbAware {
	override fun buildFoldRegions(root: PsiElement, document: Document, quick: Boolean): Array<FoldingDescriptor> {
		if (root.language != ThtrLanguage.INSTANCE) {
			return FoldingDescriptor.EMPTY_ARRAY
		}
		return collectFoldRegions(document)
			.distinctBy { it.range(document) }
			.mapNotNull { region ->
				val range = region.range(document)
				if (range.length <= 1) {
					null
				} else {
					FoldingDescriptor(root.node, range, null, region.placeholder)
				}
			}
			.toTypedArray()
	}

	override fun getPlaceholderText(node: com.intellij.lang.ASTNode): String = "..."

	override fun getPlaceholderText(node: com.intellij.lang.ASTNode, range: TextRange): String = "..."

	override fun isCollapsedByDefault(node: com.intellij.lang.ASTNode): Boolean = false
}

private data class ThtrFoldRegion(
	val startLine: Int,
	val endLine: Int,
	val placeholder: String,
) {
	fun range(document: Document): TextRange {
		return TextRange(document.getLineEndOffset(startLine), document.getLineEndOffset(endLine))
	}
}

private fun collectFoldRegions(document: Document): List<ThtrFoldRegion> {
	val lines = (0 until document.lineCount).map { line ->
		document.charsSequence.substring(document.getLineStartOffset(line), document.getLineEndOffset(line)).toString()
	}
	val regions = mutableListOf<ThtrFoldRegion>()
	for (line in lines.indices) {
		val text = lines[line]
		val trimmed = text.trimStart()
		val indent = text.length - trimmed.length
		when {
			trimmed.startsWith("scenario ") ->
				addIndentBlock(regions, lines, line, indent, " scenario ...")
			trimmed.startsWith("act ") ->
				addIndentBlock(regions, lines, line, indent, " act ...")
			startsMultilineObject(trimmed) ->
				addIndentBlock(regions, lines, line, indent, " object { ... }")
			startsMultilineList(trimmed) ->
				addIndentBlock(regions, lines, line, indent, " list [ ... ]")
			startsLargeCallBlock(trimmed) ->
				addIndentBlock(regions, lines, line, indent, " (...)")
		}
	}
	return regions
}

private fun addIndentBlock(
	regions: MutableList<ThtrFoldRegion>,
	lines: List<String>,
	startLine: Int,
	baseIndent: Int,
	placeholder: String,
) {
	val endLine = findIndentedBlockEnd(lines, startLine, baseIndent)
	if (endLine > startLine) {
		regions += ThtrFoldRegion(startLine, endLine, placeholder)
	}
}

private fun findIndentedBlockEnd(lines: List<String>, startLine: Int, baseIndent: Int): Int {
	var endLine = startLine
	for (line in startLine + 1 until lines.size) {
		val text = lines[line]
		if (text.isBlank()) {
			endLine = line
			continue
		}
		val trimmed = text.trimStart()
		val indent = text.length - trimmed.length
		if (indent <= baseIndent) {
			if (isClosingDataLine(trimmed)) {
				endLine = line
			}
			break
		}
		endLine = line
	}
	return endLine
}

private fun startsMultilineObject(trimmed: String): Boolean {
	val start = trimmed.indexOf("object {")
	return start >= 0 && !trimmed.substring(start + "object {".length).contains("}")
}

private fun startsMultilineList(trimmed: String): Boolean {
	val start = trimmed.indexOf("list [")
	return start >= 0 && !trimmed.substring(start + "list [".length).contains("]")
}

private fun startsLargeCallBlock(trimmed: String): Boolean {
	return trimmed.contains("(") && !trimmed.contains(")")
}

private fun isClosingDataLine(trimmed: String): Boolean {
	return trimmed.startsWith("}") || trimmed.startsWith("]") || trimmed.startsWith(")")
}
