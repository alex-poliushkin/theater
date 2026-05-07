package com.github.alexpoliushkin.theater.thtrij

import com.intellij.lang.annotation.AnnotationHolder
import com.intellij.lang.annotation.Annotator
import com.intellij.lang.annotation.HighlightSeverity
import com.intellij.psi.PsiElement

class ThtrSemanticAnnotator : Annotator {
	override fun annotate(element: PsiElement, holder: AnnotationHolder) {
		val key = ThtrHighlighting.semanticKey(element)
		if (key != null) {
			holder.newSilentAnnotation(HighlightSeverity.INFORMATION)
				.range(element)
				.textAttributes(key)
				.create()
		}

		val target = ThtrSymbols.referenceTarget(element)
		if (target != null && ThtrSymbols.resolve(element, target) == null) {
			holder.newAnnotation(HighlightSeverity.ERROR, target.unresolvedMessage())
				.range(element)
				.create()
		}

		for (diagnostic in ThtrDiagnostics.diagnostics(element)) {
			holder.newAnnotation(HighlightSeverity.ERROR, diagnostic.message)
				.range(diagnostic.element)
				.create()
		}
	}
}
