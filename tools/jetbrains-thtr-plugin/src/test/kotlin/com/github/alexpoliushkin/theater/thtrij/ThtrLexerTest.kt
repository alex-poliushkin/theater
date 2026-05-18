package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.psi.TokenType
import com.intellij.psi.tree.IElementType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ThtrLexerTest {
	@Test
	fun emitsNativeBaselineTokens() {
		val tokens = lex(
			"""
			stage demo
			scenario smoke
			  act request
			    do action.http(method: "GET", url: "/health")
			    expect ${'$'}response.status field status_code
			""".trimIndent(),
		).filter { it.type != TokenType.WHITE_SPACE }

		assertEquals(
			listOf(
				ThtrTypes.STAGE,
				ThtrTypes.IDENTIFIER,
				ThtrTypes.SCENARIO,
				ThtrTypes.IDENTIFIER,
				ThtrTypes.ACT,
				ThtrTypes.IDENTIFIER,
				ThtrTypes.DO,
				ThtrTypes.DOTTED_REF,
				ThtrTypes.L_PAREN,
				ThtrTypes.IDENTIFIER,
				ThtrTypes.COLON,
				ThtrTypes.STRING,
				ThtrTypes.COMMA,
				ThtrTypes.IDENTIFIER,
				ThtrTypes.COLON,
				ThtrTypes.STRING,
				ThtrTypes.R_PAREN,
				ThtrTypes.EXPECT,
				ThtrTypes.DOLLAR_REF,
				ThtrTypes.FIELD,
				ThtrTypes.IDENTIFIER,
			),
			tokens.map { it.type },
		)
	}

	@Test
	fun emitsBadCharacterTokenWithoutAborting() {
		val tokens = lex("stage demo\n\tact request @")

		assertTrue(tokens.any { it.type == ThtrTypes.BAD_CHARACTER && it.text == "\t" })
		assertTrue(tokens.any { it.type == ThtrTypes.BAD_CHARACTER && it.text == "@" })
		assertTrue(tokens.any { it.type == ThtrTypes.IDENTIFIER && it.text == "request" })
	}

	@Test
	fun emitsBadIndentForIndentedRootForms() {
		val tokens = lex(
			"""
			stage main
			  scenario login
			""".trimIndent(),
		)

		assertTrue(tokens.any { it.type == ThtrTypes.BAD_INDENT && it.text == "\n  " })
		assertTrue(tokens.any { it.type == ThtrTypes.SCENARIO })
	}

	@Test
	fun emitsAcceptedSurfaceTokens() {
		val tokens = lex(
			"""
			http
			  session browser = http.session.browser()
			state
			  backend local = state.backend.file(root: "/tmp")
			scenario auth/register(email: string!)
			  preflight recipient-test-domain: ${'$'}email matches r"^[^@]+@example\.test$" override ${'$'}allow_non_test_recipient
			  act submit
			    eventually 30s every 1s
			    do repeatable action.http
			    log response = object { status: field(status_code), id: ${'$'}id }
			    export id = field(body) | decode(json) | path("/id")
			    dependency prepare when done
			    on pass -> done
			call run = auth/register(email: generate.email())
			""".trimIndent(),
		).filter { it.type != TokenType.WHITE_SPACE }

		assertTrue(tokens.any { it.type == ThtrTypes.HTTP })
		assertTrue(tokens.any { it.type == ThtrTypes.SESSION })
		assertTrue(tokens.any { it.type == ThtrTypes.STATE })
		assertTrue(tokens.any { it.type == ThtrTypes.BACKEND })
		assertTrue(tokens.any { it.type == ThtrTypes.DURATION && it.text == "30s" })
		assertTrue(tokens.any { it.type == ThtrTypes.EVERY })
		assertTrue(tokens.any { it.type == ThtrTypes.DURATION && it.text == "1s" })
		assertTrue(tokens.any { it.type == ThtrTypes.REPEATABLE })
		assertTrue(tokens.any { it.type == ThtrTypes.PREFLIGHT })
		assertTrue(tokens.any { it.type == ThtrTypes.OVERRIDE })
		assertTrue(tokens.any { it.type == ThtrTypes.LOG })
		assertTrue(tokens.any { it.type == ThtrTypes.OBJECT })
		assertTrue(tokens.any { it.type == ThtrTypes.EXPORT })
		assertTrue(tokens.any { it.type == ThtrTypes.FIELD })
		assertTrue(tokens.any { it.type == ThtrTypes.DECODE })
		assertTrue(tokens.any { it.type == ThtrTypes.PATH })
		assertTrue(tokens.any { it.type == ThtrTypes.DEPENDENCY })
		assertTrue(tokens.any { it.type == ThtrTypes.WHEN })
		assertTrue(tokens.any { it.type == ThtrTypes.ON })
		assertTrue(tokens.any { it.type == ThtrTypes.ARROW })
		assertTrue(tokens.any { it.type == ThtrTypes.GENERATE_REF })
		assertTrue(tokens.any { it.type == ThtrTypes.IDENTIFIER && it.text == "auth/register" })
	}
}

private data class LexedToken(
	val type: IElementType,
	val text: String,
)

private fun lex(source: String): List<LexedToken> {
	val lexer = ThtrLexer()
	val tokens = mutableListOf<LexedToken>()
	lexer.start(source)
	while (true) {
		val tokenType = lexer.tokenType ?: break
		tokens += LexedToken(
			type = tokenType,
			text = source.substring(lexer.tokenStart, lexer.tokenEnd),
		)
		lexer.advance()
	}
	return tokens
}
