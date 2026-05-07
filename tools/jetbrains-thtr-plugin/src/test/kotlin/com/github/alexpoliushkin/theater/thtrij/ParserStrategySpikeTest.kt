package com.github.alexpoliushkin.theater.thtrij

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ParserStrategySpikeTest {
	@Test
	fun grammarKitStrategyAcceptsRepresentativeFixtures() {
		val parser = SpikeParser()
		val fixtures = listOf(
			"testdata/thtr-tooling-contract/success-input.thtr",
			"testdata/thtr-expectation-sugar/success-input.thtr",
			"testdata/thtr-state-ergonomics/success-input.thtr",
			"testdata/thtr-expressiveness/success-input.thtr",
			"testdata/workflows/command/command-generated.thtr",
		)

		val facts = mutableSetOf<SpikeFact>()
		for (fixture in fixtures) {
			val result = parser.parse(readFixture(fixture))
			assertTrue("$fixture produced spike errors: ${result.errors}", result.errors.isEmpty())
			facts += result.facts
		}

		assertTrue(facts.contains(SpikeFact.Stage))
		assertTrue(facts.contains(SpikeFact.Scenario))
		assertTrue(facts.contains(SpikeFact.Act))
		assertTrue(facts.contains(SpikeFact.Expectation))
		assertTrue(facts.contains(SpikeFact.Selector))
		assertTrue(facts.contains(SpikeFact.ObjectValue))
		assertTrue(facts.contains(SpikeFact.ListValue))
		assertTrue(facts.contains(SpikeFact.StateSugar))
		assertTrue(facts.contains(SpikeFact.GeneratorCall))
		assertTrue(facts.contains(SpikeFact.PluginRef))
	}

	@Test
	fun grammarKitStrategyBoundsMalformedInput() {
		val parser = SpikeParser()

		val badIndentation = parser.parse(readFixture("testdata/thtr-tooling-contract/parse-error-bad-indentation.thtr"))
		assertEquals(listOf(SpikeErrorKind.BadIndentation), badIndentation.errors.map { it.kind })
		assertEquals(2, badIndentation.errors.single().line)

		val incompleteParen = parser.parse(readFixture("testdata/thtr-tooling-contract/parse-error-incomplete-paren.thtr"))
		assertEquals(listOf(SpikeErrorKind.UnclosedDelimiter), incompleteParen.errors.map { it.kind })
		assertEquals(4, incompleteParen.errors.single().line)
	}

	private fun readFixture(relativePath: String): String {
		return Files.readString(repoRoot().resolve(relativePath))
	}

	private fun repoRoot(): Path {
		return Paths.get("..", "..").toAbsolutePath().normalize()
	}
}

private class SpikeParser {
	fun parse(source: String): SpikeResult {
		val facts = mutableSetOf<SpikeFact>()
		val errors = mutableListOf<SpikeError>()
		val delimiterStack = mutableListOf<Delimiter>()

		source.lineSequence().forEachIndexed { index, rawLine ->
			val lineNumber = index + 1
			val indent = rawLine.takeWhile { it == ' ' }.length
			val line = rawLine.trim()
			if (line.isEmpty() || line.startsWith("#")) {
				return@forEachIndexed
			}
			if (rawLine.takeWhile { it.isWhitespace() }.contains('\t')) {
				errors += SpikeError(SpikeErrorKind.BadIndentation, lineNumber)
				return@forEachIndexed
			}
			if (isTopLevelForm(line) && indent != 0) {
				errors += SpikeError(SpikeErrorKind.BadIndentation, lineNumber)
			}
			recordFacts(line, facts)
			scanDelimiters(line, lineNumber, delimiterStack, errors)
		}

		if (delimiterStack.isNotEmpty()) {
			val unclosed = delimiterStack.last()
			errors += SpikeError(SpikeErrorKind.UnclosedDelimiter, unclosed.line)
		}

		return SpikeResult(facts, errors)
	}

	private fun isTopLevelForm(line: String): Boolean {
		return line.startsWith("stage ") ||
			line == "http" ||
			line == "state" ||
			line.startsWith("scenario ") ||
			line.startsWith("call ") ||
			line.startsWith("name ")
	}

	private fun recordFacts(line: String, facts: MutableSet<SpikeFact>) {
		when {
			line.startsWith("stage ") -> facts += SpikeFact.Stage
			line.startsWith("scenario ") -> facts += SpikeFact.Scenario
			line.startsWith("act ") -> facts += SpikeFact.Act
			line.startsWith("expect ") -> facts += SpikeFact.Expectation
		}
		if (line.startsWith("do ") || line.contains(" do ")) {
			facts += SpikeFact.Action
		}
		if (line.contains("field(") || line.contains("path(") || line.contains("decode(") || line.contains("pick where")) {
			facts += SpikeFact.Selector
		}
		if (line.contains("object {") || line.endsWith("object {")) {
			facts += SpikeFact.ObjectValue
		}
		if (line.contains("list [")) {
			facts += SpikeFact.ListValue
		}
		if (line == "state" || line.contains("state.") || line.startsWith("backend ") || line.startsWith("record ") || line.startsWith("pool ")) {
			facts += SpikeFact.StateSugar
		}
		if (line.contains("generate.")) {
			facts += SpikeFact.GeneratorCall
		}
		if (line.contains("action.smoke.") || line.contains("matcher.smoke.") || line.contains("plugin.")) {
			facts += SpikeFact.PluginRef
		}
	}

	private fun scanDelimiters(
		line: String,
		lineNumber: Int,
		stack: MutableList<Delimiter>,
		errors: MutableList<SpikeError>,
	) {
		var quote: Char? = null
		var escaped = false
		for (char in line) {
			val activeQuote = quote
			if (activeQuote != null) {
				when {
					escaped -> escaped = false
					char == '\\' -> escaped = true
					char == activeQuote -> quote = null
				}
				continue
			}
			when (char) {
				'"', '\'' -> quote = char
				'(' -> stack += Delimiter(')', lineNumber)
				'{' -> stack += Delimiter('}', lineNumber)
				'[' -> stack += Delimiter(']', lineNumber)
				')', '}', ']' -> {
					if (stack.lastOrNull()?.expectedClosing == char) {
						stack.removeAt(stack.lastIndex)
					} else {
						errors += SpikeError(SpikeErrorKind.UnmatchedDelimiter, lineNumber)
					}
				}
			}
		}
	}
}

private data class SpikeResult(
	val facts: Set<SpikeFact>,
	val errors: List<SpikeError>,
)

private data class SpikeError(
	val kind: SpikeErrorKind,
	val line: Int,
)

private data class Delimiter(
	val expectedClosing: Char,
	val line: Int,
)

private enum class SpikeFact {
	Stage,
	Scenario,
	Act,
	Action,
	Expectation,
	Selector,
	ObjectValue,
	ListValue,
	StateSugar,
	GeneratorCall,
	PluginRef,
}

private enum class SpikeErrorKind {
	BadIndentation,
	UnclosedDelimiter,
	UnmatchedDelimiter,
}
