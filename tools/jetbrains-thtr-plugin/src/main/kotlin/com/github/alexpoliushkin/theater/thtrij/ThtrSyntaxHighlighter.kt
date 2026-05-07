package com.github.alexpoliushkin.theater.thtrij

import com.intellij.lexer.Lexer
import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.fileTypes.SyntaxHighlighterBase
import com.intellij.psi.tree.IElementType

class ThtrSyntaxHighlighter : SyntaxHighlighterBase() {
	override fun getHighlightingLexer(): Lexer = ThtrLexer()

	override fun getTokenHighlights(tokenType: IElementType): Array<TextAttributesKey> {
		return ThtrHighlighting.tokenHighlights(tokenType)
	}
}
