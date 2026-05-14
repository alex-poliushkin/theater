package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTokenSets
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.lang.cacheBuilder.DefaultWordsScanner
import com.intellij.lang.cacheBuilder.WordsScanner
import com.intellij.lang.findUsages.FindUsagesProvider
import com.intellij.lang.refactoring.NamesValidator
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Condition
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiManager
import com.intellij.psi.PsiReference
import com.intellij.psi.search.FilenameIndex
import com.intellij.psi.search.GlobalSearchScope
import com.intellij.psi.search.LocalSearchScope
import com.intellij.psi.search.SearchScope
import com.intellij.psi.search.searches.ReferencesSearch
import com.intellij.psi.tree.TokenSet
import com.intellij.psi.util.PsiTreeUtil
import com.intellij.util.ProcessingContext
import com.intellij.util.Processor
import com.intellij.util.QueryExecutor

class ThtrFindUsagesProvider : FindUsagesProvider {
	override fun getWordsScanner(): WordsScanner {
		return DefaultWordsScanner(
			ThtrLexer(),
			TokenSet.create(ThtrTypes.IDENTIFIER, ThtrTypes.DOTTED_REF, ThtrTypes.DOLLAR_REF),
			ThtrTokenSets.COMMENTS,
			ThtrTokenSets.STRINGS,
		)
	}

	override fun canFindUsagesFor(psiElement: PsiElement): Boolean {
		return ThtrSymbols.declarationKind(psiElement) != null
	}

	override fun getHelpId(psiElement: PsiElement): String? = null

	override fun getType(element: PsiElement): String {
		return ThtrSymbols.declarationKind(element)?.label ?: ".thtr symbol"
	}

	override fun getDescriptiveName(element: PsiElement): String {
		return ThtrSymbols.declarationName(element) ?: element.text
	}

	override fun getNodeText(element: PsiElement, useFullName: Boolean): String {
		return getDescriptiveName(element)
	}
}

class ThtrNamesValidator : NamesValidator {
	override fun isKeyword(name: String, project: Project): Boolean {
		return name in KEYWORDS
	}

	override fun isIdentifier(name: String, project: Project): Boolean {
		return ThtrSymbols.isIdentifierLikeName(name) && !isKeyword(name, project)
	}
}

class ThtrRenameVetoCondition : Condition<PsiElement> {
	override fun value(element: PsiElement): Boolean {
		return ThtrSymbols.declarationKind(element) != null && !ThtrSymbols.canRenameDeclaration(element)
	}
}

class ThtrReferencesSearchExecutor : QueryExecutor<PsiReference, ReferencesSearch.SearchParameters> {
	override fun execute(queryParameters: ReferencesSearch.SearchParameters, consumer: Processor<in PsiReference>): Boolean {
		val target = queryParameters.elementToSearch
		if (ThtrSymbols.declarationKind(target) == null) {
			return true
		}
		for (file in thtrFiles(target, queryParameters.effectiveSearchScope)) {
			val completed = PsiTreeUtil.processElements(file) { element ->
				for (reference in referencesFrom(element)) {
					if (reference.resolve() == target && !consumer.process(reference)) {
						return@processElements false
					}
				}
				true
			}
			if (!completed) {
				return false
			}
		}
		return true
	}

	private fun thtrFiles(target: PsiElement, scope: SearchScope): List<PsiFile> {
		val project = target.project
		if (scope is LocalSearchScope) {
			return scope.scope
				.mapNotNull { it.containingFile }
				.distinct()
				.filter { it.fileType == ThtrFileType.INSTANCE }
		}
		val psiManager = PsiManager.getInstance(project)
		val globalScope = scope as? GlobalSearchScope ?: GlobalSearchScope.projectScope(project)
		return FilenameIndex.getAllFilesByExt(project, "thtr", globalScope)
			.mapNotNull { psiManager.findFile(it) }
	}

	private fun referencesFrom(element: PsiElement): List<ThtrReference> {
		return ThtrReferenceProvider()
			.getReferencesByElement(element, ProcessingContext())
			.filterIsInstance<ThtrReference>()
	}
}

private val KEYWORDS = setOf(
	"stage",
	"http",
	"state",
	"session",
	"auth",
	"identity",
	"scenario",
	"act",
	"bind",
	"call",
	"name",
	"do",
	"log",
	"expect",
	"eventually",
	"prop",
	"export",
	"on",
	"dependency",
	"when",
	"every",
	"capture_auth",
	"backend",
	"record",
	"pool",
	"repeatable",
	"object",
	"list",
	"field",
	"decode",
	"path",
	"pick",
	"regexp",
	"true",
	"false",
	"null",
)
