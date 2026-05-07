package com.github.alexpoliushkin.theater.thtrij

import com.intellij.openapi.util.TextRange
import com.intellij.psi.PsiDocumentManager
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiReferenceBase

class ThtrReference(
	element: PsiElement,
	private val target: ThtrReferenceTarget,
) : PsiReferenceBase<PsiElement>(
	element,
	TextRange(target.rangeStart, element.textLength),
	false,
) {
	override fun resolve(): PsiElement? = ThtrSymbols.resolve(element, target)

	fun resolve(useProjectIndex: Boolean): PsiElement? {
		return ThtrSymbols.resolve(element, target, useProjectIndex)
	}

	override fun getVariants(): Array<Any> = emptyArray()

	override fun handleElementRename(newElementName: String): PsiElement {
		val replacement = when (target.kind) {
			ThtrReferenceKind.VALUE -> "$" + newElementName
			else -> newElementName
		}
		val file = element.containingFile
		val documentManager = PsiDocumentManager.getInstance(element.project)
		val document = documentManager.getDocument(file) ?: return element
		document.replaceString(element.textRange.startOffset, element.textRange.endOffset, replacement)
		documentManager.commitDocument(document)
		return element
	}

	fun unresolvedMessage(): String {
		return target.unresolvedMessage()
	}
}
