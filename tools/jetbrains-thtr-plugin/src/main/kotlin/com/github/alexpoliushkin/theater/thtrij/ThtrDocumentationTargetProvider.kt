package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrActDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrBindStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCallDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCaptureAuthStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrExportStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrLogStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrNamedElement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPropStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrScenarioDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrStageDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.model.Pointer
import com.intellij.platform.backend.documentation.DocumentationResult
import com.intellij.platform.backend.documentation.DocumentationTarget
import com.intellij.platform.backend.documentation.PsiDocumentationTargetProvider
import com.intellij.platform.backend.presentation.TargetPresentation
import com.intellij.psi.PsiElement

class ThtrDocumentationTargetProvider : PsiDocumentationTargetProvider {
	override fun documentationTarget(element: PsiElement, originalElement: PsiElement?): DocumentationTarget? {
		val targetElement = originalElement ?: element
		val tokenType = targetElement.node?.elementType ?: return null
		if (tokenType == ThtrTypes.DOTTED_REF || tokenType == ThtrTypes.GENERATE_REF) {
			val capability = ThtrCapabilities.byLabel(targetElement.project, targetElement.text)
			if (capability != null) {
				return ThtrCapabilityDocumentationTarget(capability)
			}
		}

		val referencedDeclaration = ThtrSymbols.referenceTarget(targetElement)
			?.let { ThtrSymbols.resolve(targetElement, it) }
		val construct = constructDocumentation(referencedDeclaration)
			?: constructDocumentation(targetElement)
			?: constructDocumentation(element)
			?: return null
		return ThtrConstructDocumentationTarget(construct)
	}
}

private class ThtrCapabilityDocumentationTarget(
	private val capability: ThtrCapability,
) : DocumentationTarget {
	override fun createPointer(): Pointer<out DocumentationTarget> = Pointer.hardPointer(this)

	override fun computePresentation(): TargetPresentation =
		TargetPresentation.builder(capability.completionLabel).presentation()

	override fun computeDocumentationHint(): String = capability.detail()

	override fun computeDocumentation(): DocumentationResult =
		DocumentationResult.documentation(ThtrCapabilities.renderDocumentation(capability))
}

private class ThtrConstructDocumentationTarget(
	private val documentation: ThtrConstructDocumentation,
) : DocumentationTarget {
	override fun createPointer(): Pointer<out DocumentationTarget> = Pointer.hardPointer(this)

	override fun computePresentation(): TargetPresentation =
		TargetPresentation.builder(documentation.title).presentation()

	override fun computeDocumentationHint(): String = "${documentation.kind}: ${documentation.summary}"

	override fun computeDocumentation(): DocumentationResult =
		DocumentationResult.documentation(renderConstructDocumentation(documentation))
}

private data class ThtrConstructDocumentation(
	val title: String,
	val kind: String,
	val summary: String,
	val sections: List<Pair<String, String>> = emptyList(),
)

private fun constructDocumentation(element: PsiElement?): ThtrConstructDocumentation? {
	val construct = constructElement(element) ?: return null
	return when (construct) {
		is ThtrStageDeclaration -> ThtrConstructDocumentation(
			title = "stage ${identifierText(construct)}",
			kind = ".thtr stage",
			summary = "Declares the stage name for the scenario file.",
		)
		is ThtrScenarioDeclaration -> ThtrConstructDocumentation(
			title = "scenario ${identifierText(construct)}",
			kind = ".thtr scenario",
			summary = "Defines a reusable flow made of ordered acts.",
		)
		is ThtrActDeclaration -> ThtrConstructDocumentation(
			title = "act ${identifierText(construct)}",
			kind = ".thtr act",
			summary = "Groups actions, scenario-authored logs, expectations, exports and transitions inside a scenario.",
		)
		is ThtrCallDeclaration -> callDocumentation(construct)
		is ThtrBindStatement -> ThtrConstructDocumentation(
			title = "bind ${construct.text.removePrefix("bind ").lineSequence().firstOrNull()?.trim().orEmpty()}",
			kind = ".thtr auth binding",
			summary = "Initializes scenario-local HTTP auth slots before the first act runs.",
		)
		is ThtrPropStatement -> ThtrConstructDocumentation(
			title = "prop ${identifierText(construct)}",
			kind = ".thtr prop",
			summary = "Defines an act-local value that can be referenced with a dollar-prefixed name.",
		)
		is ThtrExportStatement -> ThtrConstructDocumentation(
			title = "export ${identifierText(construct)}",
			kind = ".thtr export",
			summary = "Publishes a value from the current act for later references and scenario outputs.",
		)
		is ThtrLogStatement -> ThtrConstructDocumentation(
			title = "log ${identifierText(construct)}",
			kind = ".thtr log",
			summary = "Emits a report-safe act-local observation without writing to scope or affecting transitions.",
		)
		is ThtrCaptureAuthStatement -> ThtrConstructDocumentation(
			title = "capture_auth ${identifierText(construct)}",
			kind = ".thtr auth capture",
			summary = "Captures authentication data from an action output for later authenticated calls.",
		)
		else -> null
	}
}

private fun callDocumentation(call: ThtrCallDeclaration): ThtrConstructDocumentation {
	val target = callTargetText(call)
	val sections = if (target == null) emptyList() else listOf("Target scenario" to target)
	return ThtrConstructDocumentation(
		title = "call ${identifierText(call)}",
		kind = ".thtr call",
		summary = "Invokes another scenario and binds its result to a call name.",
		sections = sections,
	)
}

private fun constructElement(element: PsiElement?): PsiElement? {
	if (element == null) {
		return null
	}
	return when (element) {
		is ThtrStageDeclaration,
		is ThtrScenarioDeclaration,
		is ThtrActDeclaration,
		is ThtrCallDeclaration,
		is ThtrPropStatement,
		is ThtrExportStatement,
		is ThtrLogStatement,
		is ThtrCaptureAuthStatement -> element
		is ThtrNamedElement -> element
		else -> constructElement(element.parent)
	}
}

private fun renderConstructDocumentation(documentation: ThtrConstructDocumentation): String {
	return buildString {
		append("<b><code>")
		append(escapeDocHtml(documentation.title))
		append("</code></b>")
		append("<br/>")
		append(escapeDocHtml(documentation.kind))
		append("<br/><br/>")
		append(escapeDocHtml(documentation.summary))
		if (documentation.sections.isNotEmpty()) {
			append("<br/><br/><b>Details</b><ul>")
			for ((name, value) in documentation.sections) {
				append("<li><b>")
				append(escapeDocHtml(name))
				append(":</b> <code>")
				append(escapeDocHtml(value))
				append("</code></li>")
			}
			append("</ul>")
		}
	}
}

private fun callTargetText(call: ThtrCallDeclaration): String? {
	var afterEquals = false
	var child = call.firstChild
	while (child != null) {
		val tokenType = child.node?.elementType
		if (tokenType == ThtrTypes.EQUALS) {
			afterEquals = true
		} else if (afterEquals && isIdentifierLike(child)) {
			return child.text
		}
		child = child.nextSibling
	}
	return null
}

private fun identifierText(element: PsiElement): String {
	if (element is ThtrNamedElement) {
		return element.name ?: "<unnamed>"
	}
	var child = element.firstChild
	while (child != null) {
		if (isIdentifierLike(child)) {
			return child.text
		}
		child = child.nextSibling
	}
	return "<unnamed>"
}

private fun isIdentifierLike(element: PsiElement): Boolean {
	val tokenType = element.node?.elementType
	return tokenType == ThtrTypes.IDENTIFIER || tokenType == ThtrTypes.DOTTED_REF
}

private fun escapeDocHtml(value: String): String {
	return value
		.replace("&", "&amp;")
		.replace("<", "&lt;")
		.replace(">", "&gt;")
}
