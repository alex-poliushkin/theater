package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrActDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrAuthDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrBackendDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCallDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrDependencyStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrExpectStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrExportStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrIdentityDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrLogStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPoolDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPropStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrRecordDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrScenarioDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrSessionDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrStageDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTransitionStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.openapi.editor.DefaultLanguageHighlighterColors
import com.intellij.openapi.editor.HighlighterColors
import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.editor.colors.TextAttributesKey.createTextAttributesKey
import com.intellij.psi.PsiComment
import com.intellij.psi.PsiElement
import com.intellij.psi.TokenType
import com.intellij.psi.tree.IElementType
import com.intellij.psi.tree.TokenSet

private val EMPTY_KEYS: Array<TextAttributesKey> = emptyArray()

private val KEYWORDS: TokenSet = TokenSet.create(
	ThtrTypes.STAGE,
	ThtrTypes.HTTP,
	ThtrTypes.STATE,
	ThtrTypes.SESSION,
	ThtrTypes.AUTH,
	ThtrTypes.IDENTITY,
	ThtrTypes.SCENARIO,
	ThtrTypes.ACT,
	ThtrTypes.BIND,
	ThtrTypes.CALL,
	ThtrTypes.NAME,
	ThtrTypes.DO,
	ThtrTypes.LOG,
	ThtrTypes.EXPECT,
	ThtrTypes.EVENTUALLY,
	ThtrTypes.PROP,
	ThtrTypes.EXPORT,
	ThtrTypes.ON,
	ThtrTypes.DEPENDENCY,
	ThtrTypes.WHEN,
	ThtrTypes.EVERY,
	ThtrTypes.CAPTURE_AUTH,
	ThtrTypes.BACKEND,
	ThtrTypes.RECORD,
	ThtrTypes.POOL,
	ThtrTypes.REPEATABLE,
	ThtrTypes.OBJECT,
	ThtrTypes.LIST,
	ThtrTypes.TRUE,
	ThtrTypes.FALSE,
	ThtrTypes.NULL,
	ThtrTypes.HAS,
	ThtrTypes.NO,
	ThtrTypes.ITEM,
	ThtrTypes.ALL,
	ThtrTypes.ITEMS,
	ThtrTypes.ENTRY,
	ThtrTypes.KEY,
	ThtrTypes.LACKS,
	ThtrTypes.IS,
	ThtrTypes.BETWEEN,
	ThtrTypes.AND,
	ThtrTypes.WHERE,
	ThtrTypes.MATCHES,
	ThtrTypes.CONTAINS,
	ThtrTypes.ASSERT,
)

private val SELECTORS: TokenSet = TokenSet.create(
	ThtrTypes.FIELD,
	ThtrTypes.DECODE,
	ThtrTypes.PATH,
	ThtrTypes.PICK,
	ThtrTypes.REGEXP,
)

private val OPERATORS: TokenSet = TokenSet.create(
	ThtrTypes.L_PAREN,
	ThtrTypes.R_PAREN,
	ThtrTypes.L_BRACE,
	ThtrTypes.R_BRACE,
	ThtrTypes.L_BRACKET,
	ThtrTypes.R_BRACKET,
	ThtrTypes.COMMA,
	ThtrTypes.COLON,
	ThtrTypes.DOT,
	ThtrTypes.EQUALS,
	ThtrTypes.EQEQ,
	ThtrTypes.ARROW,
	ThtrTypes.PIPE,
	ThtrTypes.BANG,
	ThtrTypes.GT,
	ThtrTypes.GTE,
	ThtrTypes.LT,
	ThtrTypes.LTE,
)

private val IDENTIFIER_LIKE: TokenSet = TokenSet.create(
	ThtrTypes.IDENTIFIER,
	ThtrTypes.DOTTED_REF,
	ThtrTypes.DOLLAR_REF,
)

object ThtrHighlighting {
	@JvmField
	val KEYWORD: TextAttributesKey = createTextAttributesKey("THTR_KEYWORD", DefaultLanguageHighlighterColors.KEYWORD)

	@JvmField
	val IDENTIFIER: TextAttributesKey = createTextAttributesKey("THTR_IDENTIFIER", DefaultLanguageHighlighterColors.IDENTIFIER)

	@JvmField
	val DECLARATION_ID: TextAttributesKey = createTextAttributesKey("THTR_DECLARATION_ID", DefaultLanguageHighlighterColors.FUNCTION_DECLARATION)

	@JvmField
	val DATA_KEY: TextAttributesKey = createTextAttributesKey("THTR_DATA_KEY", DefaultLanguageHighlighterColors.INSTANCE_FIELD)

	@JvmField
	val REFERENCE: TextAttributesKey = createTextAttributesKey("THTR_REFERENCE", DefaultLanguageHighlighterColors.LOCAL_VARIABLE)

	@JvmField
	val CAPABILITY_REF: TextAttributesKey = createTextAttributesKey("THTR_CAPABILITY_REF", DefaultLanguageHighlighterColors.FUNCTION_CALL)

	@JvmField
	val SELECTOR: TextAttributesKey = createTextAttributesKey("THTR_SELECTOR", DefaultLanguageHighlighterColors.FUNCTION_CALL)

	@JvmField
	val GENERATOR_REF: TextAttributesKey = createTextAttributesKey("THTR_GENERATOR_REF", DefaultLanguageHighlighterColors.FUNCTION_CALL)

	@JvmField
	val STRING: TextAttributesKey = createTextAttributesKey("THTR_STRING", DefaultLanguageHighlighterColors.STRING)

	@JvmField
	val NUMBER: TextAttributesKey = createTextAttributesKey("THTR_NUMBER", DefaultLanguageHighlighterColors.NUMBER)

	@JvmField
	val DURATION: TextAttributesKey = createTextAttributesKey("THTR_DURATION", DefaultLanguageHighlighterColors.NUMBER)

	@JvmField
	val COMMENT: TextAttributesKey = createTextAttributesKey("THTR_COMMENT", DefaultLanguageHighlighterColors.LINE_COMMENT)

	@JvmField
	val OPERATOR: TextAttributesKey = createTextAttributesKey("THTR_OPERATOR", DefaultLanguageHighlighterColors.OPERATION_SIGN)

	@JvmField
	val BAD_CHARACTER: TextAttributesKey = createTextAttributesKey("THTR_BAD_CHARACTER", HighlighterColors.BAD_CHARACTER)

	fun tokenHighlights(tokenType: IElementType): Array<TextAttributesKey> {
		return when {
			KEYWORDS.contains(tokenType) -> arrayOf(KEYWORD)
			SELECTORS.contains(tokenType) -> arrayOf(SELECTOR)
			OPERATORS.contains(tokenType) -> arrayOf(OPERATOR)
			tokenType == ThtrTypes.IDENTIFIER -> arrayOf(IDENTIFIER)
			tokenType == ThtrTypes.DOTTED_REF -> arrayOf(REFERENCE)
			tokenType == ThtrTypes.DOLLAR_REF -> arrayOf(REFERENCE)
			tokenType == ThtrTypes.GENERATE_REF -> arrayOf(GENERATOR_REF)
			tokenType == ThtrTypes.STRING -> arrayOf(STRING)
			tokenType == ThtrTypes.NUMBER -> arrayOf(NUMBER)
			tokenType == ThtrTypes.DURATION -> arrayOf(DURATION)
			tokenType == ThtrTypes.LINE_COMMENT -> arrayOf(COMMENT)
			tokenType == ThtrTypes.BAD_CHARACTER -> arrayOf(BAD_CHARACTER)
			tokenType == ThtrTypes.BAD_INDENT -> arrayOf(BAD_CHARACTER)
			else -> EMPTY_KEYS
		}
	}

	fun semanticKey(element: PsiElement): TextAttributesKey? {
		if (element is PsiComment || element.node == null || element.node.elementType == TokenType.WHITE_SPACE) {
			return null
		}
		return when {
			isDeclarationIdentifier(element) -> DECLARATION_ID
			isDataKey(element) -> DATA_KEY
			isReference(element) -> REFERENCE
			isCapabilityReference(element) -> CAPABILITY_REF
			else -> null
		}
	}

	private fun isDeclarationIdentifier(element: PsiElement): Boolean {
		if (!IDENTIFIER_LIKE.contains(element.node.elementType) || !isDeclarationParent(element.parent)) {
			return false
		}
		return firstIdentifierLikeChild(element.parent) == element
	}

	private fun isDeclarationParent(parent: PsiElement?): Boolean {
		return parent is ThtrStageDeclaration ||
			parent is ThtrScenarioDeclaration ||
			parent is ThtrActDeclaration ||
			parent is ThtrCallDeclaration ||
			parent is ThtrSessionDeclaration ||
			parent is ThtrAuthDeclaration ||
			parent is ThtrIdentityDeclaration ||
			parent is ThtrBackendDeclaration ||
			parent is ThtrRecordDeclaration ||
			parent is ThtrPoolDeclaration ||
			parent is ThtrExpectStatement ||
			parent is ThtrLogStatement ||
			parent is ThtrPropStatement ||
			parent is ThtrExportStatement ||
			parent is ThtrDependencyStatement ||
			parent is ThtrTransitionStatement
	}

	private fun firstIdentifierLikeChild(parent: PsiElement): PsiElement? {
		var child = parent.firstChild
		while (child != null) {
			if (child.node != null && IDENTIFIER_LIKE.contains(child.node.elementType)) {
				return child
			}
			child = child.nextSibling
		}
		return null
	}

	private fun isDataKey(element: PsiElement): Boolean {
		return element.node.elementType == ThtrTypes.IDENTIFIER &&
			nextSignificantSibling(element)?.node?.elementType == ThtrTypes.COLON
	}

	private fun isReference(element: PsiElement): Boolean {
		return element.node.elementType == ThtrTypes.DOLLAR_REF
	}

	private fun isCapabilityReference(element: PsiElement): Boolean {
		val tokenType = element.node.elementType
		return tokenType == ThtrTypes.GENERATE_REF ||
			(tokenType == ThtrTypes.DOTTED_REF && element.text.contains('.'))
	}

	private fun nextSignificantSibling(element: PsiElement): PsiElement? {
		var sibling = element.nextSibling
		while (sibling != null) {
			if (sibling.node != null && sibling.node.elementType != TokenType.WHITE_SPACE && sibling !is PsiComment) {
				return sibling
			}
			sibling = sibling.nextSibling
		}
		return null
	}
}
