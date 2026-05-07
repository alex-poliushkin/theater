package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.codeInspection.LocalInspectionTool
import com.intellij.codeInspection.LocalQuickFix
import com.intellij.codeInspection.ProblemsHolder
import com.intellij.codeInspection.ProblemDescriptor
import com.intellij.openapi.project.Project
import com.intellij.psi.PsiDocumentManager
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiElementVisitor

class ThtrRemovedStateSyntaxInspection : LocalInspectionTool() {
	override fun buildVisitor(holder: ProblemsHolder, isOnTheFly: Boolean): PsiElementVisitor {
		return object : PsiElementVisitor() {
			override fun visitElement(element: PsiElement) {
				if (element.node?.elementType == ThtrTypes.DOTTED_REF && element.text == "state.cas") {
					holder.registerProblem(
						element,
						"state.cas has been removed; use state.update(... if_version: ...)",
						ReplaceStateCasQuickFix(),
					)
				}
			}
		}
	}
}

private class ReplaceStateCasQuickFix : LocalQuickFix {
	override fun getFamilyName(): String = "Replace state.cas with state.update"

	override fun applyFix(project: Project, descriptor: ProblemDescriptor) {
		val element = descriptor.psiElement ?: return
		val file = element.containingFile ?: return
		val document = PsiDocumentManager.getInstance(project).getDocument(file) ?: return
		val range = element.textRange
		document.replaceString(range.startOffset, range.endOffset, "state.update")
		PsiDocumentManager.getInstance(project).commitDocument(document)
	}
}
