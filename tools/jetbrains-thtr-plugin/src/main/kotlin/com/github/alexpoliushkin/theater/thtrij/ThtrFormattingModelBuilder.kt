package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.formatting.Alignment
import com.intellij.formatting.Block
import com.intellij.formatting.ChildAttributes
import com.intellij.formatting.FormattingContext
import com.intellij.formatting.FormattingModel
import com.intellij.formatting.FormattingModelBuilder
import com.intellij.formatting.FormattingModelProvider
import com.intellij.formatting.Indent
import com.intellij.formatting.Spacing
import com.intellij.formatting.SpacingBuilder
import com.intellij.formatting.Wrap
import com.intellij.formatting.WrapType
import com.intellij.lang.ASTNode
import com.intellij.openapi.util.TextRange
import com.intellij.psi.TokenType
import com.intellij.psi.codeStyle.CodeStyleSettings
import com.intellij.psi.tree.IElementType

private const val DEFAULT_INDENT_SIZE = 2

class ThtrFormattingModelBuilder : FormattingModelBuilder {
	override fun createModel(formattingContext: FormattingContext): FormattingModel {
		val settings = formattingContext.codeStyleSettings
		val indentSize = thtrIndentSize(settings)
		return FormattingModelProvider.createFormattingModelForPsiFile(
			formattingContext.containingFile,
			ThtrBlock(
				formattingContext.node,
				Wrap.createWrap(WrapType.NONE, false),
				Alignment.createAlignment(),
				createSpaceBuilder(settings),
				indentSize,
			),
			settings,
		)
	}
}

private class ThtrBlock(
	val astNode: ASTNode,
	private val wrap: Wrap?,
	private val alignment: Alignment?,
	private val spacingBuilder: SpacingBuilder,
	private val indentSize: Int,
) : Block {
	private val children: List<Block> by lazy { buildChildren() }

	override fun getTextRange(): TextRange = astNode.textRange

	override fun getSubBlocks(): List<Block> = children

	override fun getWrap(): Wrap? = wrap

	override fun getIndent(): Indent {
		return when (astNode.elementType) {
			ThtrTypes.ACT_DECLARATION, ThtrTypes.BACKEND_DECLARATION, ThtrTypes.RECORD_DECLARATION,
			ThtrTypes.POOL_DECLARATION, ThtrTypes.SESSION_DECLARATION, ThtrTypes.AUTH_DECLARATION,
			ThtrTypes.IDENTITY_DECLARATION, ThtrTypes.DEPENDENCY_STATEMENT -> Indent.getSpaceIndent(indentSize)
			ThtrTypes.DO_STATEMENT, ThtrTypes.LOG_STATEMENT, ThtrTypes.EXPECT_STATEMENT, ThtrTypes.EVENTUALLY_STATEMENT,
			ThtrTypes.PROP_STATEMENT, ThtrTypes.EXPORT_STATEMENT, ThtrTypes.TRANSITION_STATEMENT,
			ThtrTypes.CAPTURE_AUTH_STATEMENT, ThtrTypes.NAME_STATEMENT -> Indent.getSpaceIndent(indentSize * 2)
			else -> Indent.getNoneIndent()
		}
	}

	override fun getAlignment(): Alignment? = alignment

	override fun getSpacing(child1: Block?, child2: Block): Spacing? {
		val left = child1 as? ThtrBlock
		val right = child2 as? ThtrBlock
		if (left != null && right != null) {
			if (hasLineBreakBetween(left, right)) {
				return lineBreakSpacing()
			}
			lineSpacing(left, right)?.let { return it }
			tokenSpacing(left.astNode.elementType, right.astNode.elementType)?.let { return it }
		}
		return spacingBuilder.getSpacing(this, child1, child2)
	}

	override fun getChildAttributes(newChildIndex: Int): ChildAttributes = ChildAttributes(Indent.getNoneIndent(), null)

	override fun isIncomplete(): Boolean = false

	override fun isLeaf(): Boolean = astNode.firstChildNode == null

	override fun getDebugName(): String = astNode.elementType.toString()

	private fun buildChildren(): List<Block> {
		val blocks = mutableListOf<Block>()
		var child = astNode.firstChildNode
		while (child != null) {
			if (child.elementType != TokenType.WHITE_SPACE) {
				blocks += ThtrBlock(
					child,
					Wrap.createWrap(WrapType.NONE, false),
					Alignment.createAlignment(),
					spacingBuilder,
					indentSize,
				)
			}
			child = child.treeNext
		}
		return blocks
	}
}

private fun createSpaceBuilder(settings: CodeStyleSettings): SpacingBuilder {
	return SpacingBuilder(settings, ThtrLanguage.INSTANCE)
		.around(ThtrTypes.EQUALS).spaces(1)
		.around(ThtrTypes.EQEQ).spaces(1)
		.around(ThtrTypes.ARROW).spaces(1)
		.around(ThtrTypes.PIPE).spaces(1)
		.around(ThtrTypes.GT).spaces(1)
		.around(ThtrTypes.GTE).spaces(1)
		.around(ThtrTypes.LT).spaces(1)
		.around(ThtrTypes.LTE).spaces(1)
		.before(ThtrTypes.COMMA).none()
		.after(ThtrTypes.COMMA).spaces(1)
		.before(ThtrTypes.COLON).none()
		.after(ThtrTypes.COLON).spaces(1)
		.before(ThtrTypes.L_PAREN).none()
		.after(ThtrTypes.L_PAREN).none()
		.before(ThtrTypes.R_PAREN).none()
}

private fun thtrIndentSize(settings: CodeStyleSettings): Int {
	val indentOptions = settings.getCommonSettings(ThtrLanguage.INSTANCE).indentOptions
	return indentOptions?.INDENT_SIZE?.takeIf { it > 0 } ?: DEFAULT_INDENT_SIZE
}

private fun lineSpacing(left: ThtrBlock, right: ThtrBlock): Spacing? {
	if (left.astNode.elementType == ThtrTypes.DECLARATION && right.astNode.elementType == ThtrTypes.DECLARATION) {
		return lineBreakSpacing()
	}
	if (left.astNode.elementType == ThtrTypes.LINE_COMMENT) {
		return lineBreakSpacing()
	}
	if (right.astNode.elementType == ThtrTypes.LINE_COMMENT) {
		return oneSpace()
	}
	return null
}

private fun hasLineBreakBetween(left: ThtrBlock, right: ThtrBlock): Boolean {
	val fileText = left.astNode.psi?.containingFile?.text ?: return false
	val start = left.astNode.textRange.endOffset
	val end = right.astNode.textRange.startOffset
	if (start >= end || end > fileText.length) {
		return false
	}
	return fileText.subSequence(start, end).any { it == '\n' || it == '\r' }
}

private fun tokenSpacing(left: IElementType, right: IElementType): Spacing? {
	return when {
		right == ThtrTypes.L_PAREN ||
			right == ThtrTypes.R_PAREN ||
			right == ThtrTypes.COMMA ||
			right == ThtrTypes.COLON ->
			noSpace()
		left == ThtrTypes.L_PAREN ->
			noSpace()
		left == ThtrTypes.COMMA ||
			left == ThtrTypes.COLON ->
			oneSpace()
		left == ThtrTypes.L_BRACE ||
			right == ThtrTypes.R_BRACE ||
			left == ThtrTypes.L_BRACKET ||
			right == ThtrTypes.R_BRACKET ->
			oneSpace()
		isBinaryOperator(left) || isBinaryOperator(right) ->
			oneSpace()
		isSourceToken(left) && isSourceToken(right) ->
			oneSpace()
		else -> null
	}
}

private fun isBinaryOperator(type: IElementType): Boolean {
	return type == ThtrTypes.EQUALS ||
		type == ThtrTypes.EQEQ ||
		type == ThtrTypes.ARROW ||
		type == ThtrTypes.PIPE ||
		type == ThtrTypes.GT ||
		type == ThtrTypes.GTE ||
		type == ThtrTypes.LT ||
		type == ThtrTypes.LTE
}

private fun isSourceToken(type: IElementType): Boolean {
	return type !in setOf(
		TokenType.WHITE_SPACE,
		ThtrTypes.DECLARATION,
		ThtrTypes.DESCRIPTOR_REF,
		ThtrTypes.GENERATOR_CALL,
		ThtrTypes.SELECTOR_CALL,
	)
}

private fun noSpace(): Spacing = Spacing.createSpacing(0, 0, 0, false, 0)

private fun oneSpace(): Spacing = Spacing.createSpacing(1, 1, 0, false, 0)

private fun lineBreakSpacing(): Spacing = Spacing.createSpacing(0, 0, 1, true, 1)
