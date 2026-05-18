package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrActDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrAuthDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrBackendDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrBindStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCallDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCaptureAuthStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrDependencyStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrDescriptorRef
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrDoStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrEventuallyStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrExpectStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrExportStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrFile
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrGeneratorCall
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrHttpBlock
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrIdentityDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrListValue
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrLogStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrObjectValue
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPoolDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPreflightStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPropStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrRecordDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrScenarioDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrSelectorCall
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrSessionDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrStageDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrStateBlock
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTransitionStatement
import com.intellij.psi.PsiComment
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiErrorElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiFileFactory
import com.intellij.psi.util.PsiTreeUtil
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrParserDefinitionTest : BasePlatformTestCase() {
	fun testRepresentativeFileProducesStablePsiDeclarations() {
		val fixtures = listOf(
			FixtureExpectation("testdata/thtr-tooling-contract/success-input.thtr", stages = 1, scenarios = 1, acts = 1, calls = 1, expects = 1),
			FixtureExpectation("testdata/thtr-expectation-sugar/success-input.thtr", stages = 1, scenarios = 1, acts = 1, expects = 8),
			FixtureExpectation("testdata/thtr-state-ergonomics/success-input.thtr", stages = 1, scenarios = 1, acts = 7),
		)

		for (fixture in fixtures) {
			val file = parse(readFixture(fixture.path))

			assertInstanceOf(file, ThtrFile::class.java)
			assertSame(ThtrFileType.INSTANCE, file.fileType)
			assertSame(ThtrLanguage.INSTANCE, file.language)
			assertEquals(fixture.path, fixture.stages, count<ThtrStageDeclaration>(file))
			assertEquals(fixture.path, fixture.scenarios, count<ThtrScenarioDeclaration>(file))
			assertEquals(fixture.path, fixture.acts, count<ThtrActDeclaration>(file))
			assertEquals(fixture.path, fixture.calls, count<ThtrCallDeclaration>(file))
			assertEquals(fixture.path, fixture.acts, count<ThtrDoStatement>(file))
			assertEquals(fixture.path, fixture.expects, count<ThtrExpectStatement>(file))
			assertNoErrors(file, fixture.path)
		}
	}

	fun testAcceptedSyntaxFamiliesProducePsiAnchors() {
		val coverage = SyntaxCoverage()
		val fixtures = listOf(
			"testdata/thtr-tooling-contract/success-input.thtr",
			"testdata/thtr-tooling-contract/success-formatted.thtr",
			"testdata/thtr-expectation-sugar/success-input.thtr",
			"testdata/thtr-state-ergonomics/success-input.thtr",
			"testdata/thtr-expressiveness/success-input.thtr",
			"testdata/thtr-expressiveness/success-formatted.thtr",
			"testdata/thtr-expressiveness/repo/theater/lib/web/check-page-text.thtr",
			"testdata/thtr-expressiveness/repo/theater/flows/http/page-text-stress.thtr",
			"testdata/workflows/command/command-generated.thtr",
			"testdata/workflows/irs/individual-registration-email-v1.thtr",
			"testdata/workflows/state/file-backend-lifecycle.thtr",
			"testdata/docs/valid/docs/examples/first-stage/stage.thtr",
		)

		for (fixture in fixtures) {
			val file = parse(readFixture(fixture))
			assertNoErrors(file, fixture)
			coverage.add(file)
		}

		val multilineFile = parse(
			"stage multiline\n" +
				"scenario text\n" +
				"  act emit\n" +
				"    do action.command\n" +
				"      stdin: \"\"\"hello\nworld\"\"\"\n" +
				"    log response = object {\n" +
				"      status: field(exit_code),\n" +
				"      stdout: field(stdout),\n" +
				"      attempts: list [ field(exit_code), field(stdout) ]\n" +
				"    }\n" +
				"    log request = ${'$'}request_id\n" +
				"    log attempts = list [ field(exit_code), ${'$'}request_id ]\n" +
				"    expect ok: field(exit_code) == 0\n",
		)
		assertNoErrors(multilineFile, "inline multiline string")
			coverage.add(multilineFile)

			val authBindingFile = parse(
				"stage dynamic-auth\n" +
					"scenario mobile/dashboard-ready(access_token: string!)\n" +
					"  bind auth mobile_api\n" +
					"    access_token: ${'$'}access_token\n" +
					"  act wait-customer\n" +
					"    do action.http\n" +
					"      auth: \"mobile_api\"\n",
			)
			assertNoErrors(authBindingFile, "inline auth binding")
			assertEquals("inline auth binding", 1, count<ThtrBindStatement>(authBindingFile))
			coverage.add(authBindingFile)

			val preflightFile = parse(
				"stage safety\n" +
					"scenario send-email(recipient_email: string!, allow_non_test_recipient: bool)\n" +
					"  preflight recipient-test-domain: ${'$'}recipient_email matches r\"^[^@]+@example\\.test$\" override ${'$'}allow_non_test_recipient\n" +
					"  act send\n" +
					"    do action.http\n",
			)
			assertNoErrors(preflightFile, "inline preflight")
			assertEquals("inline preflight", 1, count<ThtrPreflightStatement>(preflightFile))
			coverage.add(preflightFile)

			assertTrue("stages", coverage.stages > 0)
		assertTrue("scenarios", coverage.scenarios > 0)
		assertTrue("calls", coverage.calls > 0)
		assertTrue("acts", coverage.acts > 0)
		assertTrue("actions", coverage.actions > 0)
		assertTrue("expectations", coverage.expectations > 0)
		assertTrue("logs", coverage.logs > 0)
		assertTrue("properties", coverage.properties > 0)
		assertTrue("exports", coverage.exports > 0)
			assertTrue("transitions", coverage.transitions > 0)
			assertTrue("auth bindings", coverage.authBindings > 0)
			assertTrue("preflight", coverage.preflight > 0)
			assertTrue("dependencies", coverage.dependencies > 0)
		assertTrue("http blocks", coverage.httpBlocks > 0)
		assertTrue("http session declarations", coverage.sessions > 0)
		assertTrue("http auth declarations", coverage.auths > 0)
		assertTrue("http identity declarations", coverage.identities > 0)
		assertTrue("state blocks", coverage.stateBlocks > 0)
		assertTrue("state backend declarations", coverage.backends > 0)
		assertTrue("state record declarations", coverage.records > 0)
		assertTrue("state pool declarations", coverage.pools > 0)
		assertTrue("eventually", coverage.eventually > 0)
		assertTrue("capture auth", coverage.captureAuth > 0)
		assertTrue("selectors", coverage.selectors > 0)
		assertTrue("object values", coverage.objects > 0)
		assertTrue("list values", coverage.lists > 0)
		assertTrue("generator calls", coverage.generators > 0)
		assertTrue("descriptor refs", coverage.descriptorRefs > 0)
		assertTrue("comments", coverage.comments > 0)
	}

	fun testMalformedInputProducesBoundedErrorsAndPreservesText() {
		val source = """
			stage @
			scenario smoke
			  act before-error
			    do action.http(
			  @
			  act after-error
			    do action.http
		""".trimIndent()
		val file = parse(source)
		val errors = PsiTreeUtil.findChildrenOfType(file, PsiErrorElement::class.java)

		assertInstanceOf(file, ThtrFile::class.java)
		assertEquals(source, file.text)
		assertTrue(errors.isNotEmpty())
		assertTrue(errors.size <= 4)
		assertEquals(2, count<ThtrActDeclaration>(file))
		assertEquals(2, count<ThtrDoStatement>(file))
	}

	fun testMalformedToolingFixturesStayBounded() {
		val fixtures = listOf(
			"testdata/thtr-tooling-contract/parse-error-bad-indentation.thtr",
			"testdata/thtr-tooling-contract/parse-error-incomplete-paren.thtr",
			"testdata/thtr-tooling-contract/parse-error-malformed-clause.thtr",
			"testdata/thtr-tooling-contract/parse-error-quoted-core-id.thtr",
		)

		for (fixture in fixtures) {
			val source = readFixture(fixture)
			val file = parse(source)
			val errors = PsiTreeUtil.findChildrenOfType(file, PsiErrorElement::class.java)

			assertEquals(fixture, source, file.text)
			if (fixture.endsWith("parse-error-bad-indentation.thtr")) {
				assertTrue(fixture, errors.isNotEmpty())
			}
			assertTrue(fixture, errors.size <= 6)
			assertTrue(fixture, count<ThtrDoStatement>(file) > 0)
		}
	}

	private fun parse(source: String) =
		PsiFileFactory.getInstance(project).createFileFromText("stage.thtr", ThtrFileType.INSTANCE, source)

	private fun readFixture(relativePath: String): String {
		return Files.readString(repoRoot().resolve(relativePath))
	}

	private fun repoRoot(): Path {
		return Paths.get("..", "..").toAbsolutePath().normalize()
	}

	private inline fun <reified T : PsiElement> count(file: PsiFile): Int {
		return PsiTreeUtil.findChildrenOfType(file, T::class.java).size
	}

	private fun assertNoErrors(file: PsiFile, label: String) {
		assertEmpty(label, PsiTreeUtil.findChildrenOfType(file, PsiErrorElement::class.java))
	}
}

private data class FixtureExpectation(
	val path: String,
	val stages: Int,
	val scenarios: Int,
	val acts: Int,
	val calls: Int = 0,
	val expects: Int = 0,
)

private class SyntaxCoverage {
	var stages = 0
	var scenarios = 0
	var calls = 0
	var acts = 0
	var actions = 0
	var expectations = 0
	var logs = 0
	var properties = 0
	var exports = 0
	var transitions = 0
	var authBindings = 0
	var preflight = 0
	var dependencies = 0
	var httpBlocks = 0
	var sessions = 0
	var auths = 0
	var identities = 0
	var stateBlocks = 0
	var backends = 0
	var records = 0
	var pools = 0
	var eventually = 0
	var captureAuth = 0
	var selectors = 0
	var objects = 0
	var lists = 0
	var generators = 0
	var descriptorRefs = 0
	var comments = 0

	fun add(file: PsiFile) {
		stages += count<ThtrStageDeclaration>(file)
		scenarios += count<ThtrScenarioDeclaration>(file)
		calls += count<ThtrCallDeclaration>(file)
		acts += count<ThtrActDeclaration>(file)
		actions += count<ThtrDoStatement>(file)
		expectations += count<ThtrExpectStatement>(file)
		logs += count<ThtrLogStatement>(file)
		properties += count<ThtrPropStatement>(file)
		exports += count<ThtrExportStatement>(file)
			transitions += count<ThtrTransitionStatement>(file)
			authBindings += count<ThtrBindStatement>(file)
			preflight += count<ThtrPreflightStatement>(file)
			dependencies += count<ThtrDependencyStatement>(file)
		httpBlocks += count<ThtrHttpBlock>(file)
		sessions += count<ThtrSessionDeclaration>(file)
		auths += count<ThtrAuthDeclaration>(file)
		identities += count<ThtrIdentityDeclaration>(file)
		stateBlocks += count<ThtrStateBlock>(file)
		backends += count<ThtrBackendDeclaration>(file)
		records += count<ThtrRecordDeclaration>(file)
		pools += count<ThtrPoolDeclaration>(file)
		eventually += count<ThtrEventuallyStatement>(file)
		captureAuth += count<ThtrCaptureAuthStatement>(file)
		selectors += count<ThtrSelectorCall>(file)
		objects += count<ThtrObjectValue>(file)
		lists += count<ThtrListValue>(file)
		generators += count<ThtrGeneratorCall>(file)
		descriptorRefs += count<ThtrDescriptorRef>(file)
		comments += count<PsiComment>(file)
	}

	private inline fun <reified T : PsiElement> count(file: PsiFile): Int {
		return PsiTreeUtil.findChildrenOfType(file, T::class.java).size
	}
}
