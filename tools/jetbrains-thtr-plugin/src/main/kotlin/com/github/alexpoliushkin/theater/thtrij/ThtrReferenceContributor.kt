package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.patterns.PlatformPatterns
import com.intellij.psi.PsiReferenceContributor
import com.intellij.psi.PsiReferenceRegistrar

class ThtrReferenceContributor : PsiReferenceContributor() {
	override fun registerReferenceProviders(registrar: PsiReferenceRegistrar) {
		val provider = ThtrReferenceProvider()
		registrar.registerReferenceProvider(
			PlatformPatterns.psiElement(ThtrTypes.IDENTIFIER).withLanguage(ThtrLanguage.INSTANCE),
			provider,
		)
		registrar.registerReferenceProvider(
			PlatformPatterns.psiElement(ThtrTypes.DOTTED_REF).withLanguage(ThtrLanguage.INSTANCE),
			provider,
		)
		registrar.registerReferenceProvider(
			PlatformPatterns.psiElement(ThtrTypes.DOLLAR_REF).withLanguage(ThtrLanguage.INSTANCE),
			provider,
		)
	}
}
