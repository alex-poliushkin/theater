package com.github.alexpoliushkin.theater.thtrij

import com.intellij.icons.AllIcons
import com.intellij.openapi.fileTypes.LanguageFileType
import javax.swing.Icon

class ThtrFileType private constructor() : LanguageFileType(ThtrLanguage.INSTANCE) {
	override fun getName(): String = "THTR"

	override fun getDescription(): String = "Theater scenario language"

	override fun getDefaultExtension(): String = "thtr"

	override fun getIcon(): Icon = AllIcons.FileTypes.Text

	companion object {
		@JvmField
		val INSTANCE: ThtrFileType = ThtrFileType()
	}
}
