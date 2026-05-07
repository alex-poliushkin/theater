package com.github.alexpoliushkin.theater.thtrij

import com.intellij.lang.LanguageCommenters
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.psi.codeStyle.CodeStyleManager
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

class ThtrFormatterTest : BasePlatformTestCase() {
	fun testFormatterCanonicalizesRepresentativeNativeSubset() {
		val before = Files.readString(formatterDataRoot().resolve("native-formatting.before.thtr"))
		val expected = Files.readString(formatterDataRoot().resolve("native-formatting.after.thtr"))
		myFixture.configureByText(ThtrFileType.INSTANCE, before)

		WriteCommandAction.runWriteCommandAction(project) {
			CodeStyleManager.getInstance(project).reformat(myFixture.file)
		}

		assertEquals(expected, myFixture.file.text)
		assertGoGoldenContains(
			"thtr-expressiveness/success-formatted.thtr",
			listOf(
				"scenario plugins/echo-check",
				"  act echo",
				"    do action.smoke.echo(value: \"hello\")",
				"    expect echoed: field(echo) assert matcher.smoke.equal(expected: \"hello\")",
				"object { owner: \"expressiveness-stress\" }",
				"list [",
				"      | path(\"/items\")",
			),
		)
		assertGoGoldenContains(
			"thtr-state-ergonomics/success-formatted.thtr",
			listOf(
				"state",
				"  record shared_meta = state.record",
				"    backend: local",
				"    record: \"env/shared-meta\"",
				"    min_guarantee: local-atomic",
			),
		)
	}

	fun testCodeStyleDefaultsUseTwoSpaceIndent() {
		val options = ThtrFileIndentOptionsProvider().createIndentOptions()

		assertEquals(2, options.INDENT_SIZE)
		assertEquals(4, options.CONTINUATION_INDENT_SIZE)
		assertEquals(2, options.TAB_SIZE)
		assertFalse(options.USE_TAB_CHARACTER)
	}

	fun testLineCommentBehaviorIsRegistered() {
		val commenter = LanguageCommenters.INSTANCE.forLanguage(ThtrLanguage.INSTANCE)
		assertEquals("#", commenter.lineCommentPrefix)
		assertNull(commenter.blockCommentPrefix)
		assertNull(commenter.blockCommentSuffix)
	}

	private fun formatterDataRoot(): Path {
		return Paths.get("src", "test", "testData", "formatter").toAbsolutePath().normalize()
	}

	private fun goGoldenRoot(): Path {
		return Paths.get("..", "..", "testdata").toAbsolutePath().normalize()
	}

	private fun assertGoGoldenContains(path: String, lines: List<String>) {
		val golden = Files.readString(goGoldenRoot().resolve(path))
		for (line in lines) {
			assertTrue("Go formatter golden $path must contain: $line", golden.contains(line))
			assertTrue("native formatter output must contain: $line", myFixture.file.text.contains(line))
		}
	}
}
