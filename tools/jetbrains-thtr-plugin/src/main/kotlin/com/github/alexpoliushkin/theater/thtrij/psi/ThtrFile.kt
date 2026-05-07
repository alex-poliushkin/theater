package com.github.alexpoliushkin.theater.thtrij.psi

import com.github.alexpoliushkin.theater.thtrij.ThtrFileType
import com.github.alexpoliushkin.theater.thtrij.ThtrLanguage
import com.intellij.extapi.psi.PsiFileBase
import com.intellij.openapi.fileTypes.FileType
import com.intellij.psi.FileViewProvider

class ThtrFile(viewProvider: FileViewProvider) : PsiFileBase(viewProvider, ThtrLanguage.INSTANCE) {
	override fun getFileType(): FileType = ThtrFileType.INSTANCE

	override fun toString(): String = "Theater DSL File"
}
