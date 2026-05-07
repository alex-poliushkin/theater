package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.parser.ThtrParser
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrFile
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTokenSets
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.lang.ASTNode
import com.intellij.lang.ParserDefinition
import com.intellij.lang.PsiParser
import com.intellij.lexer.Lexer
import com.intellij.openapi.project.Project
import com.intellij.psi.FileViewProvider
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.tree.IFileElementType
import com.intellij.psi.tree.TokenSet

class ThtrParserDefinition : ParserDefinition {
	override fun createLexer(project: Project?): Lexer = ThtrLexer()

	override fun createParser(project: Project?): PsiParser = ThtrParser()

	override fun getFileNodeType(): IFileElementType = FILE

	override fun getCommentTokens(): TokenSet = ThtrTokenSets.COMMENTS

	override fun getStringLiteralElements(): TokenSet = ThtrTokenSets.STRINGS

	override fun createElement(node: ASTNode): PsiElement = ThtrTypes.Factory.createElement(node)

	override fun createFile(viewProvider: FileViewProvider): PsiFile = ThtrFile(viewProvider)

	companion object {
		@JvmField
		val FILE: IFileElementType = IFileElementType(ThtrLanguage.INSTANCE)
	}
}
