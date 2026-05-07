package com.github.alexpoliushkin.theater.thtrij

import com.intellij.codeInsight.navigation.actions.GotoDeclarationHandler
import com.intellij.openapi.editor.Editor
import com.intellij.psi.PsiElement

class ThtrGotoDeclarationHandler : GotoDeclarationHandler {
	override fun getGotoDeclarationTargets(
		sourceElement: PsiElement?,
		offset: Int,
		editor: Editor,
	): Array<PsiElement>? {
		if (sourceElement == null || sourceElement.language != ThtrLanguage.INSTANCE) {
			return null
		}
		val target = ThtrSymbols.referenceTarget(sourceElement)
			?.let { ThtrSymbols.resolve(sourceElement, it) }
			?: return null
		return arrayOf(target)
	}
}
