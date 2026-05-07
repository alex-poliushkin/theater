package com.github.alexpoliushkin.theater.thtrij

import com.intellij.lang.Language
import com.intellij.openapi.fileTypes.FileType
import com.intellij.psi.PsiFile
import com.intellij.psi.codeStyle.CommonCodeStyleSettings
import com.intellij.psi.codeStyle.FileTypeIndentOptionsProvider
import com.intellij.psi.codeStyle.LanguageCodeStyleSettingsProvider
import com.intellij.psi.codeStyle.LanguageCodeStyleSettingsProvider.SettingsType

private const val DEFAULT_INDENT_SIZE = 2
private const val DEFAULT_CONTINUATION_INDENT_SIZE = 4
private const val CODE_STYLE_SAMPLE = """
stage smoke
scenario api
  act get
    do action.http(method: "GET", url: "/health")
    expect ok: field(status_code) == 200
call run = api()
"""

class ThtrLanguageCodeStyleSettingsProvider : LanguageCodeStyleSettingsProvider() {
	override fun getLanguage(): Language = ThtrLanguage.INSTANCE

	override fun customizeDefaults(
		commonSettings: CommonCodeStyleSettings,
		indentOptions: CommonCodeStyleSettings.IndentOptions,
	) {
		indentOptions.INDENT_SIZE = DEFAULT_INDENT_SIZE
		indentOptions.CONTINUATION_INDENT_SIZE = DEFAULT_CONTINUATION_INDENT_SIZE
		indentOptions.TAB_SIZE = DEFAULT_INDENT_SIZE
		indentOptions.USE_TAB_CHARACTER = false
	}

	override fun getCodeSample(settingsType: SettingsType): String = CODE_STYLE_SAMPLE.trimIndent()
}

class ThtrFileIndentOptionsProvider : FileTypeIndentOptionsProvider {
	override fun createIndentOptions(): CommonCodeStyleSettings.IndentOptions {
		return CommonCodeStyleSettings.IndentOptions().apply {
			INDENT_SIZE = DEFAULT_INDENT_SIZE
			CONTINUATION_INDENT_SIZE = DEFAULT_CONTINUATION_INDENT_SIZE
			TAB_SIZE = DEFAULT_INDENT_SIZE
			USE_TAB_CHARACTER = false
		}
	}

	override fun getFileType(): FileType = ThtrFileType.INSTANCE

	override fun getPreviewText(): String = CODE_STYLE_SAMPLE.trimIndent()

	override fun prepareForReformat(file: PsiFile) {}
}
