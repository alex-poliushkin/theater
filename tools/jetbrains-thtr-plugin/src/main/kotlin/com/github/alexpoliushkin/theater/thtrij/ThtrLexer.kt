package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.lexer.LexerBase
import com.intellij.psi.TokenType
import com.intellij.psi.tree.IElementType

private val DURATION_SUFFIXES = listOf("ms", "s", "m", "h")

class ThtrLexer : LexerBase() {
	private var buffer: CharSequence = ""
	private var startOffset: Int = 0
	private var endOffset: Int = 0
	private var tokenStart: Int = 0
	private var tokenEnd: Int = 0
	private var tokenType: IElementType? = null

	override fun start(buffer: CharSequence, startOffset: Int, endOffset: Int, initialState: Int) {
		this.buffer = buffer
		this.startOffset = startOffset
		this.endOffset = endOffset
		this.tokenStart = startOffset
		locateToken()
	}

	override fun getState(): Int = 0

	override fun getTokenType(): IElementType? = tokenType

	override fun getTokenStart(): Int = tokenStart

	override fun getTokenEnd(): Int = tokenEnd

	override fun advance() {
		tokenStart = tokenEnd
		locateToken()
	}

	override fun getBufferSequence(): CharSequence = buffer

	override fun getBufferEnd(): Int = endOffset

	private fun locateToken() {
		if (tokenStart >= endOffset) {
			tokenEnd = endOffset
			tokenType = null
			return
		}

		val char = buffer[tokenStart]
		when {
			char == '\t' -> finish(tokenStart + 1, ThtrTypes.BAD_CHARACTER)
			char.isWhitespace() -> readWhitespace()
			char == '#' -> readComment()
			startsWith("\"\"\"") -> readMultilineString()
			char == 'r' && peek(1) == '"' -> readRawString()
			char == '"' -> readString()
			char == '-' && peek(1) == '>' -> finish(tokenStart + 2, ThtrTypes.ARROW)
			char == '=' && peek(1) == '=' -> finish(tokenStart + 2, ThtrTypes.EQEQ)
			char == '>' && peek(1) == '=' -> finish(tokenStart + 2, ThtrTypes.GTE)
			char == '<' && peek(1) == '=' -> finish(tokenStart + 2, ThtrTypes.LTE)
			char == '-' && peek(1)?.isDigit() == true -> readNumber()
			char.isDigit() -> readNumber()
			char == '$' -> readDollarRef()
			isIdentifierStart(char) -> readIdentifierLike()
			else -> readPunctuationOrBadCharacter(char)
		}
	}

	private fun readWhitespace() {
		var offset = tokenStart + 1
		while (offset < endOffset) {
			val char = buffer[offset]
			if (!char.isWhitespace() || char == '\t') {
				break
			}
			offset++
		}
		val token = if (isBadIndentWhitespace(buffer, tokenStart, offset)) ThtrTypes.BAD_INDENT else TokenType.WHITE_SPACE
		finish(offset, token)
	}

	private fun readComment() {
		var offset = tokenStart + 1
		while (offset < endOffset && buffer[offset] != '\n' && buffer[offset] != '\r') {
			offset++
		}
		finish(offset, ThtrTypes.LINE_COMMENT)
	}

	private fun readMultilineString() {
		var offset = tokenStart + 3
		while (offset < endOffset && !startsWithAt(offset, "\"\"\"")) {
			offset++
		}
		if (offset < endOffset) {
			offset += 3
		}
		finish(offset, ThtrTypes.STRING)
	}

	private fun readRawString() {
		var offset = tokenStart + 2
		while (offset < endOffset && buffer[offset] != '"') {
			offset++
		}
		if (offset < endOffset) {
			offset++
		}
		finish(offset, ThtrTypes.STRING)
	}

	private fun readString() {
		var offset = tokenStart + 1
		var escaped = false
		while (offset < endOffset) {
			val char = buffer[offset]
			when {
				escaped -> escaped = false
				char == '\\' -> escaped = true
				char == '"' -> {
					offset++
					break
				}
			}
			offset++
		}
		finish(offset, ThtrTypes.STRING)
	}

	private fun readNumber() {
		var offset = tokenStart
		if (buffer[offset] == '-') {
			offset++
		}
		while (offset < endOffset && buffer[offset].isDigit()) {
			offset++
		}
		if (offset + 1 < endOffset && buffer[offset] == '.' && buffer[offset + 1].isDigit()) {
			offset++
			while (offset < endOffset && buffer[offset].isDigit()) {
				offset++
			}
		}
		val durationEnd = readDurationSuffix(offset)
		if (durationEnd > offset) {
			finish(durationEnd, ThtrTypes.DURATION)
			return
		}
		finish(offset, ThtrTypes.NUMBER)
	}

	private fun readDurationSuffix(from: Int): Int {
		for (suffix in DURATION_SUFFIXES) {
			if (startsWithAt(from, suffix)) {
				return from + suffix.length
			}
		}
		return from
	}

	private fun readDollarRef() {
		var offset = tokenStart + 1
		if (offset >= endOffset || !isIdentifierStart(buffer[offset])) {
			finish(tokenStart + 1, ThtrTypes.BAD_CHARACTER)
			return
		}
		offset = readQualifiedIdentifier(offset)
		finish(offset, ThtrTypes.DOLLAR_REF)
	}

	private fun readIdentifierLike() {
		val offset = readQualifiedIdentifier(tokenStart)
		val text = buffer.subSequence(tokenStart, offset).toString()
		val token = keywordToken(text) ?: when {
			text.startsWith("generate.") -> ThtrTypes.GENERATE_REF
			text.contains('.') -> ThtrTypes.DOTTED_REF
			else -> ThtrTypes.IDENTIFIER
		}
		finish(offset, token)
	}

	private fun readQualifiedIdentifier(from: Int): Int {
		var offset = readIdentifier(from)
		while (offset + 1 < endOffset && isQualifiedIdentifierSeparator(buffer[offset]) && isIdentifierStart(buffer[offset + 1])) {
			offset = readIdentifier(offset + 1)
		}
		return offset
	}

	private fun readIdentifier(from: Int): Int {
		var offset = from + 1
		while (offset < endOffset && isIdentifierPart(buffer[offset])) {
			offset++
		}
		return offset
	}

	private fun readPunctuationOrBadCharacter(char: Char) {
		val token = when (char) {
			'(' -> ThtrTypes.L_PAREN
			')' -> ThtrTypes.R_PAREN
			'{' -> ThtrTypes.L_BRACE
			'}' -> ThtrTypes.R_BRACE
			'[' -> ThtrTypes.L_BRACKET
			']' -> ThtrTypes.R_BRACKET
			',' -> ThtrTypes.COMMA
			':' -> ThtrTypes.COLON
			'.' -> ThtrTypes.DOT
			'=' -> ThtrTypes.EQUALS
			'|' -> ThtrTypes.PIPE
			'!' -> ThtrTypes.BANG
			'>' -> ThtrTypes.GT
			'<' -> ThtrTypes.LT
			else -> ThtrTypes.BAD_CHARACTER
		}
		finish(tokenStart + 1, token)
	}

	private fun startsWith(value: String): Boolean = startsWithAt(tokenStart, value)

	private fun startsWithAt(offset: Int, value: String): Boolean {
		if (offset + value.length > endOffset) {
			return false
		}
		for (index in value.indices) {
			if (buffer[offset + index] != value[index]) {
				return false
			}
		}
		return true
	}

	private fun peek(relativeOffset: Int): Char? {
		val offset = tokenStart + relativeOffset
		return if (offset < endOffset) buffer[offset] else null
	}

	private fun finish(end: Int, type: IElementType) {
		tokenEnd = end
		tokenType = type
	}
}

private fun keywordToken(text: String): IElementType? {
	return when (text) {
		"stage" -> ThtrTypes.STAGE
		"http" -> ThtrTypes.HTTP
		"state" -> ThtrTypes.STATE
		"session" -> ThtrTypes.SESSION
		"auth" -> ThtrTypes.AUTH
		"identity" -> ThtrTypes.IDENTITY
		"scenario" -> ThtrTypes.SCENARIO
		"act" -> ThtrTypes.ACT
		"call" -> ThtrTypes.CALL
		"name" -> ThtrTypes.NAME
		"do" -> ThtrTypes.DO
		"log" -> ThtrTypes.LOG
		"expect" -> ThtrTypes.EXPECT
		"eventually" -> ThtrTypes.EVENTUALLY
		"prop" -> ThtrTypes.PROP
		"export" -> ThtrTypes.EXPORT
		"on" -> ThtrTypes.ON
		"dependency" -> ThtrTypes.DEPENDENCY
		"when" -> ThtrTypes.WHEN
		"every" -> ThtrTypes.EVERY
		"capture_auth" -> ThtrTypes.CAPTURE_AUTH
		"backend" -> ThtrTypes.BACKEND
		"record" -> ThtrTypes.RECORD
		"pool" -> ThtrTypes.POOL
		"repeatable" -> ThtrTypes.REPEATABLE
		"object" -> ThtrTypes.OBJECT
		"list" -> ThtrTypes.LIST
		"field" -> ThtrTypes.FIELD
		"decode" -> ThtrTypes.DECODE
		"path" -> ThtrTypes.PATH
		"pick" -> ThtrTypes.PICK
		"regexp" -> ThtrTypes.REGEXP
		"true" -> ThtrTypes.TRUE
		"false" -> ThtrTypes.FALSE
		"null" -> ThtrTypes.NULL
		"has" -> ThtrTypes.HAS
		"no" -> ThtrTypes.NO
		"item" -> ThtrTypes.ITEM
		"all" -> ThtrTypes.ALL
		"items" -> ThtrTypes.ITEMS
		"entry" -> ThtrTypes.ENTRY
		"key" -> ThtrTypes.KEY
		"lacks" -> ThtrTypes.LACKS
		"is" -> ThtrTypes.IS
		"between" -> ThtrTypes.BETWEEN
		"and" -> ThtrTypes.AND
		"where" -> ThtrTypes.WHERE
		"matches" -> ThtrTypes.MATCHES
		"contains" -> ThtrTypes.CONTAINS
		"assert" -> ThtrTypes.ASSERT
		else -> null
	}
}

private fun isIdentifierStart(char: Char): Boolean = char in 'A'..'Z' || char in 'a'..'z'

private fun isIdentifierPart(char: Char): Boolean = isIdentifierStart(char) || char in '0'..'9' || char == '_' || char == '-'

private fun isQualifiedIdentifierSeparator(char: Char): Boolean = char == '.' || char == '/'

private fun isBadIndentWhitespace(buffer: CharSequence, start: Int, end: Int): Boolean {
	val lineStart = lastLineStart(buffer, start, end) ?: return false
	var offset = lineStart
	while (offset < end && buffer[offset] == ' ') {
		offset++
	}
	return offset > lineStart && startsWithRootForm(buffer, end)
}

private fun lastLineStart(buffer: CharSequence, start: Int, end: Int): Int? {
	for (offset in end - 1 downTo start) {
		if (buffer[offset] == '\n' || buffer[offset] == '\r') {
			return offset + 1
		}
	}
	return null
}

private fun startsWithRootForm(buffer: CharSequence, offset: Int): Boolean {
	return startsWithWord(buffer, offset, "stage") ||
		startsWithWord(buffer, offset, "http") ||
		startsWithWord(buffer, offset, "state") ||
		startsWithWord(buffer, offset, "scenario") ||
		startsWithWord(buffer, offset, "call")
}

private fun startsWithWord(buffer: CharSequence, offset: Int, word: String): Boolean {
	if (offset + word.length > buffer.length) {
		return false
	}
	for (index in word.indices) {
		if (buffer[offset + index] != word[index]) {
			return false
		}
	}
	val next = offset + word.length
	return next == buffer.length || !isIdentifierPart(buffer[next])
}
