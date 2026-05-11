package com.github.alexpoliushkin.theater.thtrij

import com.intellij.codeInsight.daemon.impl.HighlightInfo
import com.intellij.lang.annotation.HighlightSeverity
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrDiagnosticsTest : BasePlatformTestCase() {
	fun testStaticDiagnosticsReportMessagesOnStableRanges() {
		addSmokePluginManifest()
		val file = myFixture.configureByText(
			ThtrFileType.INSTANCE,
			Files.readString(inspectionDataRoot().resolve("static-diagnostics.thtr")),
		)

		val diagnostics = errorDiagnostics()

		assertDiagnostic(diagnostics, "action.missing", "Unknown .thtr capability reference: action.missing")
		assertDiagnostic(
			diagnostics,
			"action.http",
			""".thtr capability "action.http" is missing required argument: method""",
		)
		assertDiagnostic(
			diagnostics,
			"action.smoke.echo",
			""".thtr capability "action.smoke.echo" is missing required argument: value""",
		)
		assertDiagnostic(diagnostics, "field", """.thtr selector "field" requires arguments""")
		assertDiagnostic(diagnostics, "field", """.thtr selector "field" requires an argument""")
		assertDiagnostic(diagnostics, "transform.missing", "Unknown .thtr capability reference: transform.missing")
		assertDiagnostic(diagnostics, "missing-act", "Unresolved .thtr act reference: missing-act")
		assertDiagnostic(diagnostics, "${'$'}missing", "Unresolved .thtr value reference: missing")

		val validRanges = diagnostics.filter { it.rangeText(file.text) == "inventory.http.get" }
		assertTrue("valid inventory capability must not be highlighted as an error", validRanges.isEmpty())
	}

	fun testMalformedSyntaxReportsParserError() {
		myFixture.configureByText(ThtrFileType.INSTANCE, "stage\n")

		val diagnostic = errorDiagnostics()
			.single { it.description?.startsWith("Malformed .thtr syntax:") == true }

		assertEquals(5, diagnostic.startOffset)
		assertEquals(5, diagnostic.endOffset)
		assertEquals("", diagnostic.rangeText(myFixture.file.text))
	}

	fun testScenarioLogDiagnosticsTrackShippedTheaterDslContract() {
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage smoke
			scenario api
			  act read
			    log before = field(status_code)
			    do action.http
			    log with_modifier required = field(status_code)
			    log bad_root = "status"
			    log bad_pipe = field(body) | action.vendor.redact()
			    expect ok: field(status_code) == 200
			    log after = field(status_code)
			""".trimIndent(),
		)

		val diagnostics = errorDiagnostics()

		assertDiagnostic(diagnostics, "log before = field(status_code)", ".thtr logs must appear after do or capture_auth")
		assertDiagnostic(
			diagnostics,
			"required",
			"Theater DSL logs use `log <id> = <log-value>`; use YAML for capture, sensitivity, required, message or fields.",
		)
		assertDiagnostic(diagnostics, "\"status\"", ".thtr log value must start with field(...), ${'$'}ref, object, or list")
		assertDiagnostic(
			diagnostics,
			"action.vendor.redact",
			".thtr log pipelines support selector steps only: decode, path, pick, regexp or transform",
		)
		assertDiagnostic(diagnostics, "log after = field(status_code)", ".thtr logs must appear after do or capture_auth and before expect, export or on")
	}

	fun testCommandActionIndentedArgumentsTrackShippedTheaterDslContract() {
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage check-values
			scenario check-profile
			  act read-profile
			    do action.command
			      executable: "printf"
			      args: list [
			        "%s",
			        "{\"data\":{\"id\":\"user-123\",\"status\":\"active\",\"email\":\"demo@example.test\"}}"
			      ]
			      timeout: "5s"
			    expect command-ok: field(exit_code) == 0
			""".trimIndent(),
		)

		val diagnostics = errorDiagnostics()
		assertTrue(
			"valid command action block must not be highlighted as an error\n${renderDiagnostics(diagnostics)}",
			diagnostics.isEmpty(),
		)

		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage check-values
			scenario check-profile
			  act invalid-command
			    do action.command
			      args: list ["emit"]
			""".trimIndent(),
		)

		val invalidDiagnostics = errorDiagnostics()
		val missingExecutable = invalidDiagnostics.filter {
			it.description == """.thtr capability "action.command" is missing required argument: executable""" &&
				it.rangeText(myFixture.file.text) == "action.command"
		}

		assertEquals(renderDiagnostics(invalidDiagnostics), 1, missingExecutable.size)
	}

	fun testScenarioInputsAndCallExportsDoNotProduceUnresolvedValueDiagnostics() {
		myFixture.addFileToProject(
			"theater/lib/messages/make.thtr",
			"""
			stage message-library

			scenario messages/make(text: string!)
			  act create
			    do action.generate
			      outputs:
			        message: ${'$'}text
			    expect message: field(values) | path("/message") == ${'$'}text
			    export message = field(values) | path("/message")
			""".trimIndent(),
		)
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage reusable-message-flow

			scenario verify-message(message: string!, expected: string!, actual: string!)
			  act check
			    do action.generate
			      outputs:
			        actual: ${'$'}actual
			    expect message: field(values) | path("/actual") == ${'$'}expected

			call make-message = messages/make(
			  text: "hello from Theater"
			)
			  export shared_message = ${'$'}message

			call check-message = verify-message(message: "caller-local", expected: "hello from Theater", actual: ${'$'}shared_message)
			  dependency make-message
			""".trimIndent(),
		)

		val diagnostics = errorDiagnostics()
		assertTrue(
			"scenario inputs and scenario-call exports must resolve\n${renderDiagnostics(diagnostics)}",
			diagnostics.none { it.description?.startsWith("Unresolved .thtr value reference:") == true },
		)
	}

	fun testRemovedStateSyntaxQuickFixIsMechanical() {
		myFixture.enableInspections(ThtrRemovedStateSyntaxInspection())
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage smoke
			scenario api
			  act update
			    do state.cas<caret>(expected_version: "1")
			""".trimIndent(),
		)

		val quickFix = myFixture.filterAvailableIntentions("Replace state.cas with state.update").single()
		myFixture.launchAction(quickFix)

		myFixture.checkResult(
			"""
			stage smoke
			scenario api
			  act update
			    do state.update(expected_version: "1")
			""".trimIndent(),
		)
	}

	private fun errorDiagnostics(): List<HighlightInfo> {
		return myFixture.doHighlighting()
			.filter { it.severity == HighlightSeverity.ERROR }
	}

	private fun assertDiagnostic(diagnostics: List<HighlightInfo>, rangeText: String, message: String) {
		assertTrue(
			"missing diagnostic on \"$rangeText\": $message\n${renderDiagnostics(diagnostics)}",
			diagnostics.any { it.description == message && it.rangeText(myFixture.file.text) == rangeText },
		)
	}

	private fun renderDiagnostics(diagnostics: List<HighlightInfo>): String {
		return diagnostics.joinToString("\n") { diagnostic ->
			"${diagnostic.startOffset}-${diagnostic.endOffset} ${diagnostic.rangeText(myFixture.file.text)}: ${diagnostic.description}"
		}
	}

	private fun HighlightInfo.rangeText(source: String): String {
		return source.substring(startOffset, endOffset)
	}

	private fun addSmokePluginManifest() {
		myFixture.addFileToProject(
			"plugins/smoke/manifest.json",
			Files.readString(completionDataRoot().resolve("smoke-manifest.json")),
		)
	}

	private fun completionDataRoot(): Path {
		return Paths.get("src", "test", "testData", "completion").toAbsolutePath().normalize()
	}

	private fun inspectionDataRoot(): Path {
		return Paths.get("src", "test", "testData", "inspections").toAbsolutePath().normalize()
	}
}
