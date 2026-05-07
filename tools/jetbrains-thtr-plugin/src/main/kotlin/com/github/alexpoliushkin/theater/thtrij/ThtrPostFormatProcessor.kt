package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.openapi.util.TextRange
import com.intellij.psi.PsiDocumentManager
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.TokenType
import com.intellij.psi.codeStyle.CodeStyleSettings
import com.intellij.psi.impl.source.codeStyle.PostFormatProcessor
import com.intellij.psi.tree.IElementType

private const val INDENT_UNIT = 2

class ThtrPostFormatProcessor : PostFormatProcessor {
	override fun processElement(source: PsiElement, settings: CodeStyleSettings): PsiElement {
		val file = source.containingFile ?: return source
		processFile(file)
		return source
	}

	override fun processText(source: PsiFile, rangeToReformat: TextRange, settings: CodeStyleSettings): TextRange {
		if (source.fileType != ThtrFileType.INSTANCE) {
			return rangeToReformat
		}
		val formatted = processFile(source)
		return TextRange(0, formatted.length)
	}
}

internal object ThtrTextFormatter {
	fun format(source: String): String {
		if (source.isEmpty()) {
			return source
		}

		val normalized = source.replace("\r\n", "\n").replace('\r', '\n')
		val endsWithNewLine = normalized.endsWith("\n")
		val sourceLines = normalized.split('\n')
		val result = mutableListOf<String>()
		val state = ThtrLineFormatState()
		val lineCount = if (endsWithNewLine) sourceLines.size - 1 else sourceLines.size

		for (index in 0 until lineCount) {
			val trimmed = sourceLines[index].trim()
			if (trimmed.isEmpty()) {
				appendBlankLine(result)
				continue
			}

			val root = rootForm(trimmed)
			if (root != null && root != "stage") {
				appendBlankLine(result)
			}

			val indent = state.indentFor(trimmed)
			result += " ".repeat(indent) + renderLine(trimmed)
			state.advance(trimmed)
		}

		while (result.lastOrNull()?.isEmpty() == true) {
			result.removeAt(result.lastIndex)
		}
		return result.joinToString("\n") + "\n"
	}
}

private data class ThtrFormatToken(val type: IElementType, val text: String)

private class ThtrLineFormatState {
	private var root: String? = null
	private var inStateEntry = false
	private var inHttpEntry = false
	private var inAct = false
	private var inActArguments = false
	private var inParenthesizedContinuation = false
	private var dataBlockDepth = 0

	fun indentFor(trimmed: String): Int {
		rootForm(trimmed)?.let { return 0 }
		if (trimmed == ")") {
			return INDENT_UNIT * 2
		}
		if (inParenthesizedContinuation) {
			return INDENT_UNIT * 3
		}
		if (trimmed.startsWith("#")) {
			return commentIndent()
		}
		return when (root) {
			"state" -> stateIndent(trimmed)
			"http" -> httpIndent(trimmed)
			"scenario" -> scenarioIndent(trimmed)
			"call" -> callIndent(trimmed)
			else -> 0
		}
	}

	fun advance(trimmed: String) {
		rootForm(trimmed)?.let {
			root = it
			inStateEntry = false
			inHttpEntry = false
			inAct = false
			inActArguments = false
			inParenthesizedContinuation = false
			dataBlockDepth = 0
			return
		}

		when (root) {
			"state" -> advanceState(trimmed)
			"http" -> advanceHttp(trimmed)
			"scenario" -> advanceScenario(trimmed)
		}
	}

	private fun stateIndent(trimmed: String): Int {
		if (isStateEntry(trimmed)) {
			return INDENT_UNIT
		}
		return if (inStateEntry) INDENT_UNIT * 2 else INDENT_UNIT
	}

	private fun httpIndent(trimmed: String): Int {
		if (isHttpEntry(trimmed)) {
			return INDENT_UNIT
		}
		return if (inHttpEntry) INDENT_UNIT * 2 else INDENT_UNIT
	}

	private fun scenarioIndent(trimmed: String): Int {
		return when {
			trimmed.startsWith("act ") -> INDENT_UNIT
			isActStatement(trimmed) -> INDENT_UNIT * 2
			dataBlockDepth > 0 && isClosingDataLine(trimmed) -> INDENT_UNIT * (1 + dataBlockDepth)
			dataBlockDepth > 0 -> INDENT_UNIT * (2 + dataBlockDepth)
			inActArguments -> INDENT_UNIT * 3
			inAct -> INDENT_UNIT * 2
			else -> INDENT_UNIT
		}
	}

	private fun callIndent(trimmed: String): Int {
		return if (trimmed.startsWith("dependency ") || trimmed.startsWith("export ")) INDENT_UNIT else 0
	}

	private fun commentIndent(): Int {
		return when {
			inParenthesizedContinuation || inActArguments -> INDENT_UNIT * 3
			dataBlockDepth > 0 -> INDENT_UNIT * (2 + dataBlockDepth)
			inAct -> INDENT_UNIT * 2
			root == "state" && inStateEntry -> INDENT_UNIT * 2
			root == "http" && inHttpEntry -> INDENT_UNIT * 2
			root == "state" || root == "http" || root == "scenario" -> INDENT_UNIT
			else -> 0
		}
	}

	private fun advanceState(trimmed: String) {
		if (isStateEntry(trimmed)) {
			inStateEntry = true
		}
	}

	private fun advanceHttp(trimmed: String) {
		if (isHttpEntry(trimmed)) {
			inHttpEntry = true
		}
	}

	private fun advanceScenario(trimmed: String) {
		when {
			trimmed.startsWith("act ") -> {
				inAct = true
				inActArguments = false
				inParenthesizedContinuation = false
				dataBlockDepth = 0
			}
			trimmed == ")" -> {
				inParenthesizedContinuation = false
				inActArguments = false
				dataBlockDepth = 0
			}
			isActStatement(trimmed) -> {
				inActArguments = startsBlockArgumentForm(trimmed)
				inParenthesizedContinuation = trimmed.endsWith("(")
				dataBlockDepth = if (inParenthesizedContinuation) 0 else dataBlockDelta(trimmed).coerceAtLeast(0)
			}
			inParenthesizedContinuation -> {
				inParenthesizedContinuation = true
			}
			dataBlockDepth > 0 -> {
				dataBlockDepth = (dataBlockDepth + dataBlockDelta(trimmed)).coerceAtLeast(0)
			}
		}
	}
}

private fun processFile(file: PsiFile): String {
	if (file.fileType != ThtrFileType.INSTANCE) {
		return file.text
	}
	val documentManager = PsiDocumentManager.getInstance(file.project)
	val document = documentManager.getDocument(file) ?: return file.text
	val formatted = ThtrTextFormatter.format(document.text)
	if (formatted != document.text) {
		document.replaceString(0, document.textLength, formatted)
		documentManager.commitDocument(document)
	}
	return formatted
}

private fun renderLine(trimmed: String): String {
	val tokens = lexLine(trimmed)
	val commentIndex = tokens.indexOfFirst { it.type == ThtrTypes.LINE_COMMENT }
	if (commentIndex == 0) {
		return tokens[0].text.trimEnd()
	}

	val codeTokens = if (commentIndex == -1) tokens else tokens.take(commentIndex)
	val comment = if (commentIndex == -1) null else tokens[commentIndex].text.trimEnd()
	val code = renderTokens(codeTokens)
	return when {
		code.isEmpty() -> comment.orEmpty()
		comment != null -> "$code $comment"
		else -> code
	}
}

private fun renderTokens(tokens: List<ThtrFormatToken>): String {
	val result = StringBuilder()
	var previous: ThtrFormatToken? = null
	for (token in tokens) {
		if (previous != null && needsSpace(previous.type, token.type)) {
			result.append(' ')
		}
		result.append(token.text)
		previous = token
	}
	return result.toString()
}

private fun lexLine(line: String): List<ThtrFormatToken> {
	val lexer = ThtrLexer()
	lexer.start(line, 0, line.length, 0)
	val tokens = mutableListOf<ThtrFormatToken>()
	while (lexer.tokenType != null) {
		val type = lexer.tokenType
		if (type != null && type != TokenType.WHITE_SPACE && type != ThtrTypes.BAD_INDENT) {
			tokens += ThtrFormatToken(type, line.substring(lexer.tokenStart, lexer.tokenEnd))
		}
		lexer.advance()
	}
	return tokens
}

private fun needsSpace(left: IElementType, right: IElementType): Boolean {
	return when {
		right == ThtrTypes.R_PAREN ||
		right == ThtrTypes.R_BRACKET ||
			right == ThtrTypes.COMMA ||
			right == ThtrTypes.COLON ->
			false
		right == ThtrTypes.L_PAREN && !isBinaryOperator(left) ->
			false
		left == ThtrTypes.R_PAREN && isWordLike(right) ->
			true
		left == ThtrTypes.L_BRACE && right != ThtrTypes.R_BRACE ->
			true
		right == ThtrTypes.R_BRACE && left != ThtrTypes.L_BRACE ->
			true
		left == ThtrTypes.L_PAREN ||
			left == ThtrTypes.L_BRACKET ||
			left == ThtrTypes.L_BRACE ->
			false
		left == ThtrTypes.COMMA ||
			left == ThtrTypes.COLON ||
			left == ThtrTypes.PIPE ->
			true
		right == ThtrTypes.L_BRACE ||
			right == ThtrTypes.L_BRACKET ->
			true
		isBinaryOperator(left) || isBinaryOperator(right) ->
			true
		isWordLike(left) && isWordLike(right) ->
			true
		else -> false
	}
}

private fun rootForm(trimmed: String): String? {
	return when {
		trimmed.startsWith("stage ") || trimmed == "stage" -> "stage"
		trimmed.startsWith("http") && isWholeKeyword(trimmed, "http") -> "http"
		trimmed.startsWith("state") && isWholeKeyword(trimmed, "state") -> "state"
		trimmed.startsWith("scenario ") || trimmed == "scenario" -> "scenario"
		trimmed.startsWith("call ") || trimmed == "call" -> "call"
		else -> null
	}
}

private fun isWholeKeyword(trimmed: String, keyword: String): Boolean {
	return trimmed.length == keyword.length || trimmed.getOrNull(keyword.length)?.isWhitespace() == true
}

private fun isStateEntry(trimmed: String): Boolean {
	return trimmed.startsWith("backend ") || trimmed.startsWith("record ") || trimmed.startsWith("pool ")
}

private fun isHttpEntry(trimmed: String): Boolean {
	return trimmed.startsWith("session ") || trimmed.startsWith("auth ") || trimmed.startsWith("identity ")
}

private fun isActStatement(trimmed: String): Boolean {
	return trimmed.startsWith("do ") ||
		trimmed.startsWith("log ") ||
		trimmed.startsWith("expect ") ||
		trimmed.startsWith("eventually ") ||
		trimmed.startsWith("prop ") ||
		trimmed.startsWith("export ") ||
		trimmed.startsWith("on ") ||
		trimmed.startsWith("capture_auth ") ||
		trimmed.startsWith("name ")
}

private fun startsBlockArgumentForm(trimmed: String): Boolean {
	return trimmed.startsWith("do ") && !trimmed.contains("(") ||
		trimmed.startsWith("capture_auth ")
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

private fun isWordLike(type: IElementType): Boolean {
	return type != ThtrTypes.L_PAREN &&
		type != ThtrTypes.R_PAREN &&
		type != ThtrTypes.L_BRACKET &&
		type != ThtrTypes.R_BRACKET &&
		type != ThtrTypes.L_BRACE &&
		type != ThtrTypes.R_BRACE &&
		type != ThtrTypes.COMMA &&
		type != ThtrTypes.COLON &&
		type != ThtrTypes.DOT &&
		!isBinaryOperator(type)
}

private fun isClosingDataLine(trimmed: String): Boolean {
	return trimmed.startsWith("}") || trimmed.startsWith("]")
}

private fun dataBlockDelta(trimmed: String): Int {
	return lexLine(trimmed).sumOf { token ->
		when (token.type) {
			ThtrTypes.L_BRACE, ThtrTypes.L_BRACKET -> 1
			ThtrTypes.R_BRACE, ThtrTypes.R_BRACKET -> -1
			else -> 0
		}
	}
}

private fun appendBlankLine(result: MutableList<String>) {
	if (result.isNotEmpty() && result.last().isNotEmpty()) {
		result += ""
	}
}
