package com.github.alexpoliushkin.theater.thtrij

import com.intellij.codeInsight.lookup.Lookup
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrCompletionTest : BasePlatformTestCase() {
	fun testCompletionContributorIsRegisteredForThtrFiles() {
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			<caret>
			""".trimIndent(),
		)

		val labels = myFixture.completeBasic().map { it.lookupString }.toSet()

		assertTrue(labels.contains("stage"))
		assertTrue(labels.contains("scenario"))
		assertFalse(labels.contains("act"))
	}

	fun testStructuralKeywordCompletionIsContextAware() {
		val scenarioLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  <caret>
			""",
			)
			assertTrue(scenarioLabels.contains("act"))
			assertTrue(scenarioLabels.contains("bind"))
			assertTrue(scenarioLabels.contains("preflight"))
			assertFalse(scenarioLabels.contains("call"))
			assertFalse(scenarioLabels.contains("stage"))

		val actLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    <caret>
			""",
		)
		assertTrue(actLabels.contains("do"))
		assertTrue(actLabels.contains("expect"))
		assertFalse(actLabels.contains("log"))
		assertFalse(actLabels.contains("scenario"))

		val afterDoLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    do action.http
			    <caret>
			""",
		)
		assertTrue(afterDoLabels.contains("log"))

		val afterExpectLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    do action.http
			    expect ok: field(status_code) == 200
			    <caret>
			""",
		)
		assertFalse(afterExpectLabels.contains("log"))
	}

	fun testLocalAndRepoScenarioIdsCompleteFromPsi() {
		myFixture.addFileToProject(
			"theater/lib/web/check-page.thtr",
			"""
			scenario web/check-page
			  act open
			    export status = field(status_code)
			""".trimIndent(),
		)

		val labels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    do action.http()

			call run = <caret>
			""",
		)

		assertTrue(labels.contains("login"))
		assertTrue(labels.contains("web/check-page"))
	}

	fun testLocalActValueAndStateBackendIdsCompleteFromPsi() {
		val actLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    export id = field(body)
			    on pass -> <caret>
			  act verify
			    expect has-id: ${'$'}id == "42"
			""",
		)
		assertTrue(actLabels.contains("verify"))

		val valueLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    export id = field(body)
			    expect has-id: ${'$'}<caret> == "42"
			""",
		)
		assertTrue(valueLabels.contains("${'$'}id"))

		val backendLabels = completionLabels(
			"""
			stage smoke
			state
			  backend local = state.backend.file(root: "/tmp/theater-state")
			  record shared_meta = state.record(backend: l<caret>
			""",
		)
		assertTrue(backendLabels.contains("local"))
	}

	fun testScenarioLogCompletionsCoverStatementAndValueRootsWithoutCreatingValueRefs() {
		val valueRootLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    do action.http
			    log response = f<caret>
			""",
		)
		assertTrue(valueRootLabels.contains("field"))
		assertFalse(valueRootLabels.contains("decode"))

		val selectorLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    do action.http
			    log response = field(body) | d<caret>
			""",
		)
		assertTrue(selectorLabels.contains("decode"))

		val valueLabels = completionLabels(
			"""
			stage smoke
			scenario login
			  act submit
			    prop request_id = generate.uuid()
			    log response = field(body)
			    expect has-id: ${'$'}<caret> == "42"
			""",
		)
		assertTrue(valueLabels.contains("${'$'}request_id"))
		assertFalse(valueLabels.contains("${'$'}response"))
	}

	fun testBuiltInAndDescriptorBackedCapabilityRefsCompleteInSupportedPositions() {
		addSmokePluginManifest()

		assertTrue(completionLabels("stage smoke\nscenario api\n  act get\n    do action.<caret>").contains("action.http"))
		assertTrue(completionLabels("stage smoke\nscenario api\n  act get\n    do action.sm<caret>").contains("action.smoke.echo"))
		assertTrue(completionLabels("stage smoke\nstate\n  backend smoke = state_backend.sm<caret>").contains("state_backend.smoke.file"))
		assertTrue(
			completionLabels(
				"""
				stage smoke
				scenario plugin
				  act load
				    prop wrapped = inventory.http.get(url: "/payload") | transform.sm<caret>
				""",
			).contains("transform.smoke.wrap"),
		)
		assertTrue(
			completionLabels(
				"""
				stage smoke
				scenario plugin
				  act load
				    expect wrapped: field(body) | transform.sm<caret>
				""",
			).contains("transform.smoke.wrap"),
		)
		assertTrue(
			completionLabels(
				"""
				stage smoke
				scenario plugin
				  act echo
				    expect echoed: field(echo) assert matcher.sm<caret>
				""",
			).contains("matcher.smoke.equal"),
		)
		assertTrue(completionLabels("stage smoke\nscenario gen\n  act create\n    prop email = generate.e<caret>").contains("generate.email"))
		assertTrue(completionLabels("stage smoke\nscenario gen\n  act create\n    prop start_date = generate.d<caret>").contains("generate.date"))
		assertTrue(completionLabels("stage smoke\nscenario gen\n  act create\n    prop email = c<caret>").contains("coalesce"))
		assertTrue(completionLabels("stage smoke\nscenario gen\n  act create\n    prop email = coalesce(e<caret>").contains("env"))
	}

	fun testKeywordCompletionReplacesTypedPrefix() {
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage smoke
			scenario login
			  act submit
			    ex<caret>
			""".trimIndent(),
		)

		val export = myFixture.completeBasic().first { it.lookupString == "export" }
		myFixture.lookup.currentItem = export
		myFixture.finishLookup(Lookup.NORMAL_SELECT_CHAR)

		assertTrue(myFixture.file.text.contains("    export"))
		assertFalse(myFixture.file.text.contains("exexport"))
	}

	fun testDateFormatCompletionInsertsValueInsideExistingQuotes() {
		myFixture.configureByText(
			ThtrFileType.INSTANCE,
			"""
			stage smoke
			scenario gen
			  act create
			    prop start_date = generate.date(format: "<caret>")
			""".trimIndent(),
		)

		val iso = myFixture.completeBasic().first { it.lookupString == "iso" }
		myFixture.lookup.currentItem = iso
		myFixture.finishLookup(Lookup.NORMAL_SELECT_CHAR)

		assertTrue(myFixture.file.text.contains("""generate.date(format: "iso")"""))
	}

	fun testDescriptorArgumentNamesCompleteFromCapabilityContracts() {
		addSmokePluginManifest()

		val actionLabels = completionLabels(
			"""
			stage smoke
			scenario plugin
			  act echo
			    do action.smoke.echo(v<caret>
			""",
		)
		assertTrue(actionLabels.contains("value"))

		val transformLabels = completionLabels(
			"""
			stage smoke
			scenario plugin
			  act load
			    prop wrapped = inventory.http.get(url: "/payload") | transform.smoke.wrap(p<caret>
			""",
		)
		assertTrue(transformLabels.contains("prefix"))

		val selectorTransformLabels = completionLabels(
			"""
			stage smoke
			scenario plugin
			  act load
			    expect wrapped: field(body) | transform.smoke.wrap(p<caret>
			""",
		)
		assertTrue(selectorTransformLabels.contains("prefix"))

		val dateArgLabels = completionLabels(
			"""
			stage smoke
			scenario gen
			  act create
			    prop start_date = generate.date(<caret>
			""",
		)
		assertTrue(dateArgLabels.contains("format"))
		assertTrue(dateArgLabels.contains("offset"))

		val dateFormatLabels = completionLabels(
			"""
			stage smoke
			scenario gen
			  act create
			    prop start_date = generate.date(format: "<caret>
			""",
		)
		assertTrue(dateFormatLabels.contains("iso"))
		assertTrue(dateFormatLabels.contains("basic"))
	}

	private fun completionLabels(source: String): Set<String> {
		val file = myFixture.configureByText(ThtrFileType.INSTANCE, source.trimIndent())
		return ThtrCompletions.items(file, myFixture.caretOffset).map { it.label }.toSet()
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
}
