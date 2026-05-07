package com.github.alexpoliushkin.theater.thtrij

import com.intellij.psi.PsiElement
import com.intellij.psi.PsiReference
import com.intellij.psi.PsiReferenceProvider
import com.intellij.util.ProcessingContext

class ThtrReferenceProvider : PsiReferenceProvider() {
	override fun getReferencesByElement(element: PsiElement, context: ProcessingContext): Array<PsiReference> {
		val target = ThtrSymbols.referenceTarget(element) ?: return PsiReference.EMPTY_ARRAY
		return arrayOf(ThtrReference(element, target))
	}
}
