package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrBackendDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrDescriptorRef
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrDoStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrExpectStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrFile
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrGeneratorCall
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrListValue
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrLogStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrObjectValue
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPropStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrSelectorCall
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiErrorElement
import com.intellij.psi.util.PsiTreeUtil

private val SELECTOR_TOKENS = setOf(
	ThtrTypes.FIELD,
	ThtrTypes.DECODE,
	ThtrTypes.PATH,
	ThtrTypes.PICK,
	ThtrTypes.REGEXP,
)

private val LOG_VALUE_ROOT_TOKENS = setOf(
	ThtrTypes.FIELD,
	ThtrTypes.DOLLAR_REF,
	ThtrTypes.OBJECT,
	ThtrTypes.LIST,
)

private val LOG_VALUE_PIPELINE_TOKENS = setOf(
	ThtrTypes.DECODE,
	ThtrTypes.PATH,
	ThtrTypes.PICK,
	ThtrTypes.REGEXP,
)

object ThtrDiagnostics {
	fun diagnostics(element: PsiElement): List<ThtrDiagnostic> {
		val diagnostics = mutableListOf<ThtrDiagnostic>()
		val tokenType = element.node?.elementType
		when {
			element is ThtrFile ->
				diagnostics += ThtrDescriptorMetadata.load(element.project).issues.map { issue ->
					ThtrDiagnostic(element, issue.message)
				}
			element is PsiErrorElement ->
				diagnostics += ThtrDiagnostic(element, "Malformed .thtr syntax: ${element.errorDescription}")
			tokenType == ThtrTypes.BAD_INDENT ->
				diagnostics += ThtrDiagnostic(element, "Top-level .thtr declarations must not be indented")
			tokenType == ThtrTypes.BAD_CHARACTER ->
				diagnostics += ThtrDiagnostic(element, "Unexpected .thtr character")
		}

		if (element is ThtrLogStatement) {
			diagnostics += logDiagnostics(element)
		}
		selectorDiagnostic(element)?.let { diagnostics += it }
		capabilityDiagnostics(element).let { diagnostics += it }
		return diagnostics
	}
}

data class ThtrDiagnostic(
	val element: PsiElement,
	val message: String,
)

private fun logDiagnostics(log: ThtrLogStatement): List<ThtrDiagnostic> {
	val diagnostics = mutableListOf<ThtrDiagnostic>()
	logPlacementDiagnostic(log)?.let { diagnostics += it }

	val children = significantChildren(log)
	val idIndex = children.indexOfFirst { child ->
		val type = child.node?.elementType
		type == ThtrTypes.IDENTIFIER || type == ThtrTypes.DOTTED_REF || type == ThtrTypes.DOLLAR_REF
	}
	if (idIndex < 0) {
		return diagnostics
	}

	val afterID = children.getOrNull(idIndex + 1)
	if (afterID?.node?.elementType != ThtrTypes.EQUALS) {
		diagnostics += ThtrDiagnostic(
			afterID ?: log,
			"Theater DSL logs use `log <id> = <log-value>`; use YAML for capture, sensitivity, required, message or fields.",
		)
		return diagnostics
	}

	val valueRoot = children.getOrNull(idIndex + 2)
	if (valueRoot == null) {
		diagnostics += ThtrDiagnostic(log, ".thtr log value must start with field(...), ${'$'}ref, object, or list")
		return diagnostics
	}
	if (logValueRootToken(valueRoot) !in LOG_VALUE_ROOT_TOKENS) {
		diagnostics += ThtrDiagnostic(valueRoot, ".thtr log value must start with field(...), ${'$'}ref, object, or list")
	}

	for (index in children.indices) {
		if (children[index].node?.elementType != ThtrTypes.PIPE) {
			continue
		}
		val next = children.getOrNull(index + 1) ?: continue
		if (logValueRootToken(next) !in LOG_VALUE_PIPELINE_TOKENS) {
			diagnostics += ThtrDiagnostic(next, ".thtr log pipelines support selector steps only: decode, path, pick or regexp")
		}
	}

	return diagnostics
}

private fun logPlacementDiagnostic(log: ThtrLogStatement): ThtrDiagnostic? {
	val fileText = log.containingFile?.text ?: return null
	val lineStart = fileText.lastIndexOf('\n', (log.textRange.startOffset - 1).coerceAtLeast(0)).let {
		if (it < 0) 0 else it + 1
	}
	val currentLineIndex = fileText.substring(0, lineStart).count { it == '\n' }
	val lines = fileText.split('\n')
	val currentLine = lines.getOrNull(currentLineIndex) ?: return null
	val currentIndent = currentLine.takeWhile { it == ' ' }.length
	var sawActionBoundary = false

	for (index in currentLineIndex - 1 downTo 0) {
		val line = lines[index]
		val trimmed = line.trim()
		if (trimmed.isEmpty() || trimmed.startsWith("#")) {
			continue
		}
		val indent = line.takeWhile { it == ' ' }.length
		if (indent < currentIndent) {
			break
		}
		if (indent > currentIndent) {
			continue
		}
		when {
			trimmed.startsWith("expect ") ||
				trimmed.startsWith("export ") ||
				trimmed.startsWith("on ") ->
				return ThtrDiagnostic(log, ".thtr logs must appear after do or capture_auth and before expect, export or on")
			trimmed.startsWith("do ") || trimmed.startsWith("capture_auth ") -> {
				sawActionBoundary = true
				break
			}
		}
	}

	return if (sawActionBoundary) {
		null
	} else {
		ThtrDiagnostic(log, ".thtr logs must appear after do or capture_auth")
	}
}

private fun logValueRootToken(element: PsiElement): com.intellij.psi.tree.IElementType? {
	val host = syntaxHost(element)
	return when (host) {
		is ThtrSelectorCall, is ThtrObjectValue, is ThtrListValue -> host.firstChild?.node?.elementType
		else -> host.node?.elementType
	}
}

private fun significantChildren(element: PsiElement): List<PsiElement> {
	val children = mutableListOf<PsiElement>()
	var child = element.firstChild
	while (child != null) {
		if (child.node != null && child.text.isNotBlank()) {
			children += child
		}
		child = child.nextSibling
	}
	return children
}

private fun selectorDiagnostic(element: PsiElement): ThtrDiagnostic? {
	val tokenType = element.node?.elementType ?: return null
	if (tokenType !in SELECTOR_TOKENS) {
		return null
	}
	val host = syntaxHost(element)
	val open = nextSignificantSibling(host)
	if (open?.node?.elementType != ThtrTypes.L_PAREN) {
		return ThtrDiagnostic(element, ".thtr selector \"${element.text}\" requires arguments")
	}
	val firstArg = nextSignificantSibling(open)
	if (firstArg == null || firstArg.node?.elementType == ThtrTypes.R_PAREN) {
		return ThtrDiagnostic(element, ".thtr selector \"${element.text}\" requires an argument")
	}
	return null
}

private fun capabilityDiagnostics(element: PsiElement): List<ThtrDiagnostic> {
	val label = capabilityLabel(element) ?: return emptyList()
	if (label == "state.cas") {
		return emptyList()
	}
	val capability = ThtrCapabilities.byLabel(element.project, label)
		?: return listOf(ThtrDiagnostic(element, "Unknown .thtr capability reference: $label"))

	val provided = argumentNames(element)
	val missing = capability.parameters.filter { it.required && it.name !in provided }
	if (missing.isEmpty()) {
		return emptyList()
	}
	return missing.map { parameter ->
		ThtrDiagnostic(element, ".thtr capability \"$label\" is missing required argument: ${parameter.name}")
	}
}

private fun capabilityLabel(element: PsiElement): String? {
	val tokenType = element.node?.elementType ?: return null
	if (tokenType == ThtrTypes.GENERATE_REF) {
		return element.text
	}
	if (tokenType != ThtrTypes.DOTTED_REF) {
		return null
	}
	return when {
		isActionRef(element) -> element.text
		isStateBackendRef(element) -> element.text
		isInventoryRef(element) -> element.text
		isTransformRef(element) -> element.text
		isMatcherRef(element) -> element.text
		else -> null
	}
}

private fun isActionRef(element: PsiElement): Boolean {
	val host = syntaxHost(element)
	if (PsiTreeUtil.getParentOfType(host, ThtrDoStatement::class.java) == null) {
		return false
	}
	val previous = previousSignificantSibling(host)?.node?.elementType
	return previous == ThtrTypes.DO || previous == ThtrTypes.REPEATABLE
}

private fun isStateBackendRef(element: PsiElement): Boolean {
	val host = syntaxHost(element)
	if (PsiTreeUtil.getParentOfType(host, ThtrBackendDeclaration::class.java) == null) {
		return false
	}
	return previousSignificantSibling(host)?.node?.elementType == ThtrTypes.EQUALS
}

private fun isInventoryRef(element: PsiElement): Boolean {
	val host = syntaxHost(element)
	if (PsiTreeUtil.getParentOfType(host, ThtrPropStatement::class.java) == null) {
		return false
	}
	return previousSignificantSibling(host)?.node?.elementType == ThtrTypes.EQUALS
}

private fun isTransformRef(element: PsiElement): Boolean {
	val host = syntaxHost(element)
	if (PsiTreeUtil.getParentOfType(host, ThtrPropStatement::class.java) == null &&
		PsiTreeUtil.getParentOfType(host, ThtrExpectStatement::class.java) == null
	) {
		return false
	}
	return previousSignificantSibling(host)?.node?.elementType == ThtrTypes.PIPE
}

private fun isMatcherRef(element: PsiElement): Boolean {
	val host = syntaxHost(element)
	if (PsiTreeUtil.getParentOfType(host, ThtrExpectStatement::class.java) == null) {
		return false
	}
	return previousSignificantSibling(host)?.node?.elementType == ThtrTypes.ASSERT
}

private fun argumentNames(element: PsiElement): Set<String> {
	val statement = containingCapabilityStatement(element) ?: return emptySet()
	val inline = inlineArgumentNames(element, statement.text)
	if (inline.isNotEmpty() || statement.text.contains(element.text + "(")) {
		return inline
	}
	if (!supportsIndentedArgumentBlock(element)) {
		return emptySet()
	}
	return indentedArgumentNames(element, statement)
}

private fun inlineArgumentNames(element: PsiElement, text: String): Set<String> {
	val start = text.indexOf(element.text + "(")
	if (start < 0) {
		return emptySet()
	}
	val args = text.substring(start + element.text.length + 1)
		.substringBefore(")")
	return Regex("""\b([A-Za-z_][A-Za-z0-9_-]*)\s*:""")
		.findAll(args)
		.map { it.groupValues[1] }
		.toSet()
}

private fun indentedArgumentNames(element: PsiElement, statement: PsiElement): Set<String> {
	val text = statement.text
	val relativeOffset = (element.textRange.startOffset - statement.textRange.startOffset).coerceAtLeast(0)
	val refLineIndex = text.substring(0, relativeOffset.coerceAtMost(text.length)).count { it == '\n' }
	val lines = text.split('\n')
	val refLine = lines.getOrNull(refLineIndex) ?: return emptySet()
	val refIndent = leadingSpaceCount(refLine)
	var argumentIndent: Int? = null
	val names = mutableSetOf<String>()
	val argument = Regex("""^([A-Za-z_][A-Za-z0-9_-]*)\s*:""")

	for (line in lines.drop(refLineIndex + 1)) {
		val trimmed = line.trim()
		if (trimmed.isEmpty() || trimmed.startsWith("#")) {
			continue
		}
		val indent = leadingSpaceCount(line)
		if (indent <= refIndent) {
			break
		}
		val currentArgumentIndent = argumentIndent ?: indent.also { argumentIndent = it }
		if (indent == currentArgumentIndent) {
			argument.find(trimmed)?.let { names += it.groupValues[1] }
		}
	}

	return names
}

private fun syntaxHost(element: PsiElement): PsiElement {
	return when (element.parent) {
		is ThtrDescriptorRef, is ThtrGeneratorCall, is ThtrSelectorCall -> element.parent
		else -> element
	}
}

private fun supportsIndentedArgumentBlock(element: PsiElement): Boolean {
	return isActionRef(element) || isStateBackendRef(element)
}

private fun containingCapabilityStatement(element: PsiElement): PsiElement? {
	var current: PsiElement? = syntaxHost(element)
	while (current != null) {
		if (current is ThtrDoStatement ||
			current is ThtrBackendDeclaration ||
			current is ThtrPropStatement ||
			current is ThtrExpectStatement ||
			current is ThtrLogStatement
		) {
			return current
		}
		current = current.parent
	}
	return null
}

private fun leadingSpaceCount(line: String): Int {
	return line.takeWhile { it == ' ' }.length
}

private fun previousSignificantSibling(element: PsiElement): PsiElement? {
	var sibling = element.prevSibling
	while (sibling != null) {
		if (sibling.node != null && sibling.text.isNotBlank()) {
			return sibling
		}
		sibling = sibling.prevSibling
	}
	return null
}

private fun nextSignificantSibling(element: PsiElement): PsiElement? {
	var sibling = element.nextSibling
	while (sibling != null) {
		if (sibling.node != null && sibling.text.isNotBlank()) {
			return sibling
		}
		sibling = sibling.nextSibling
	}
	return null
}
