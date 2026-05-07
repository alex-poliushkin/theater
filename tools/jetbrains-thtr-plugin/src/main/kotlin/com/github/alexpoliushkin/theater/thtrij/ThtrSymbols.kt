package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrActDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrBackendDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCallDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCaptureAuthStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrExportStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrFile
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrNamedElement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrPropStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrScenarioDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTransitionStatement
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.openapi.project.DumbService
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.PsiManager
import com.intellij.psi.search.FilenameIndex
import com.intellij.psi.search.GlobalSearchScope
import com.intellij.psi.util.PsiTreeUtil
import com.intellij.psi.tree.TokenSet

private val REFERENCE_TEXT_TOKENS: TokenSet = TokenSet.create(
	ThtrTypes.IDENTIFIER,
	ThtrTypes.DOTTED_REF,
)

private val SCENARIO_INPUT_REGEX = Regex("""(?:^|,)\s*([A-Za-z][A-Za-z0-9_-]*)\s*:""")

private val CALL_TARGET_REGEX = Regex(
	"""^call\s+[A-Za-z][A-Za-z0-9_-]*(?:[./][A-Za-z][A-Za-z0-9_-]*)*\s*=\s*([A-Za-z][A-Za-z0-9_-]*(?:[./][A-Za-z][A-Za-z0-9_-]*)*)""",
)

object ThtrSymbols {
	fun referenceTarget(element: PsiElement): ThtrReferenceTarget? {
		val tokenType = element.node?.elementType ?: return null
		if (tokenType == ThtrTypes.DOLLAR_REF && element.text.length > 1) {
			return ThtrReferenceTarget(ThtrReferenceKind.VALUE, element.text.removePrefix("$"), 1)
		}
		if (!REFERENCE_TEXT_TOKENS.contains(tokenType)) {
			return null
		}
		val parent = element.parent
		return when {
			parent is ThtrCallDeclaration && previousSignificantSibling(element)?.node?.elementType == ThtrTypes.EQUALS ->
				ThtrReferenceTarget(ThtrReferenceKind.SCENARIO, element.text, 0)
			parent is ThtrTransitionStatement && previousSignificantSibling(element)?.node?.elementType == ThtrTypes.ARROW ->
				ThtrReferenceTarget(ThtrReferenceKind.ACT, element.text, 0)
			else -> null
		}
	}

	fun resolve(element: PsiElement, target: ThtrReferenceTarget, useProjectIndex: Boolean = !DumbService.isDumb(element.project)): PsiElement? {
		val file = element.containingFile ?: return null
		val symbols = collect(file)
		return when (target.kind) {
			ThtrReferenceKind.SCENARIO -> symbols.scenarios[target.name] ?: findRepoScenario(file, target.name, useProjectIndex)
			ThtrReferenceKind.ACT -> symbols.acts[target.name]
			ThtrReferenceKind.VALUE -> {
				val callScenarioID = enclosingCallTargetID(element)
				if (callScenarioID != null) {
					return resolveScenarioCallValue(file, callScenarioID, target.name, symbols, useProjectIndex)
				}
				val scenario = enclosingScenario(element, symbols)
				if (scenario != null) {
					return scenarioValueDeclaration(scenario, target.name, includeActLocals = true)
				}
				symbols.values[target.name]
			}
		}
	}

	fun scenarioTargets(file: PsiFile, useProjectIndex: Boolean = !DumbService.isDumb(file.project)): Map<String, PsiElement> {
		val targets = linkedMapOf<String, PsiElement>()
		targets.putAll(collect(file).scenarios)
		if (!useProjectIndex || file.virtualFile == null) {
			return targets
		}
		for (candidate in repoThtrFiles(file)) {
			if (candidate.virtualFile == file.virtualFile) {
				continue
			}
			targets.putAll(collect(candidate).scenarios)
		}
		return targets
	}

	fun declarationKind(element: PsiElement): ThtrReferenceKind? {
		if (element is ThtrNamedElement) {
			return namedDeclarationKind(element)
		}
		if (!REFERENCE_TEXT_TOKENS.contains(element.node?.elementType)) {
			return null
		}
		val parent = element.parent
		if (isScenarioInputNameElement(element)) {
			return ThtrReferenceKind.VALUE
		}
		if (firstIdentifierLikeChild(parent) != element) {
			return null
		}
		return namedDeclarationKind(parent)
	}

	fun declarationName(element: PsiElement): String? {
		return when {
			declarationKind(element) == null -> null
			element is ThtrNamedElement -> element.name
			else -> element.text
		}
	}

	fun canRenameDeclaration(element: PsiElement): Boolean {
		return declarationKind(element) != null && !isRepoLibraryScenarioDeclaration(element)
	}

	fun isIdentifierLikeName(name: String): Boolean {
		return name.matches(Regex("[A-Za-z][A-Za-z0-9_-]*(?:[./][A-Za-z][A-Za-z0-9_-]*)*"))
	}

	fun collect(file: PsiFile): ThtrFileSymbols {
		val symbols = ThtrFileSymbols()
		for (scenario in PsiTreeUtil.findChildrenOfType(file, ThtrScenarioDeclaration::class.java)) {
			firstIdentifierLikeChild(scenario)?.let { symbols.scenarios.putIfAbsent(it.text, scenario) }
		}
		for (act in PsiTreeUtil.findChildrenOfType(file, ThtrActDeclaration::class.java)) {
			firstIdentifierLikeChild(act)?.let { symbols.acts.putIfAbsent(it.text, act) }
		}
		for (export in PsiTreeUtil.findChildrenOfType(file, ThtrExportStatement::class.java)) {
			firstIdentifierLikeChild(export)?.let { symbols.values.putIfAbsent(it.text, export) }
		}
		for (prop in PsiTreeUtil.findChildrenOfType(file, ThtrPropStatement::class.java)) {
			firstIdentifierLikeChild(prop)?.let { symbols.values.putIfAbsent(it.text, prop) }
		}
		for (captureAuth in PsiTreeUtil.findChildrenOfType(file, ThtrCaptureAuthStatement::class.java)) {
			firstIdentifierLikeChild(captureAuth)?.let { symbols.values.putIfAbsent(it.text, captureAuth) }
		}
		for (backend in PsiTreeUtil.findChildrenOfType(file, ThtrBackendDeclaration::class.java)) {
			firstIdentifierLikeChild(backend)?.let { symbols.stateBackends.putIfAbsent(it.text, it) }
		}
		return symbols
	}

	private fun resolveScenarioCallValue(
		file: PsiFile,
		scenarioID: String,
		name: String,
		symbols: ThtrFileSymbols,
		useProjectIndex: Boolean,
	): PsiElement? {
		val scenario = symbols.scenarios[scenarioID] ?: findRepoScenario(file, scenarioID, useProjectIndex) ?: return null
		return scenarioValueDeclaration(scenario, name, includeActLocals = false)
	}

	private fun findRepoScenario(file: PsiFile, name: String, useProjectIndex: Boolean): PsiElement? {
		if (!useProjectIndex || file.virtualFile == null) {
			return null
		}
		for (candidate in repoThtrFiles(file)) {
			val target = collect(candidate).scenarios[name]
			if (target != null) {
				return target
			}
		}
		return null
	}

	private fun scenarioValueDeclaration(scenario: PsiElement, name: String, includeActLocals: Boolean): PsiElement? {
		if (scenario is ThtrScenarioDeclaration) {
			scenarioInputNameChildren(scenario).firstOrNull { it.text == name }?.let {
				return it
			}
		}
		return scenarioValueDeclarations(scenario, includeActLocals)
			.firstOrNull { declaration -> firstIdentifierLikeChild(declaration)?.text == name }
	}

	private fun isRepoLibraryScenarioDeclaration(element: PsiElement): Boolean {
		if (declarationKind(element) != ThtrReferenceKind.SCENARIO) {
			return false
		}
		val path = element.containingFile?.virtualFile?.path ?: return false
		return path.contains("/theater/lib/")
	}

	private fun repoThtrFiles(file: PsiFile): Sequence<ThtrFile> {
		val project = file.project
		val psiManager = PsiManager.getInstance(project)
		val scope = GlobalSearchScope.projectScope(project)
		return FilenameIndex.getAllFilesByExt(project, "thtr", scope)
			.asSequence()
			.filter { it.path.contains("/theater/lib/") }
			.mapNotNull { psiManager.findFile(it) as? ThtrFile }
	}

	private fun namedDeclarationKind(element: PsiElement): ThtrReferenceKind? {
		return when (element) {
			is ThtrScenarioDeclaration -> ThtrReferenceKind.SCENARIO
			is ThtrActDeclaration -> ThtrReferenceKind.ACT
			is ThtrExportStatement, is ThtrPropStatement, is ThtrCaptureAuthStatement -> ThtrReferenceKind.VALUE
			else -> null
		}
	}

	private fun firstIdentifierLikeChild(parent: PsiElement): PsiElement? {
		var child = parent.firstChild
		while (child != null) {
			if (child.node != null && REFERENCE_TEXT_TOKENS.contains(child.node.elementType)) {
				return child
			}
			child = child.nextSibling
		}
		return null
	}

	private fun scenarioInputNameChildren(scenario: ThtrScenarioDeclaration): List<PsiElement> {
		val text = scenario.text
		val open = text.indexOf('(')
		if (open < 0) {
			return emptyList()
		}
		val close = text.indexOf(')', open + 1)
		if (close < 0 || close <= open) {
			return emptyList()
		}

		val names = SCENARIO_INPUT_REGEX.findAll(text.substring(open + 1, close))
			.map { match ->
				val group = match.groups[1] ?: return@map null
				InputName(group.value, open + 1 + group.range.first, open + 1 + group.range.last + 1)
			}
			.filterNotNull()
			.toList()
		if (names.isEmpty()) {
			return emptyList()
		}

		val leaves = leafChildren(scenario)
		val scenarioStart = scenario.textRange.startOffset
		return names.mapNotNull { input ->
			val absoluteStart = scenarioStart + input.start
			val absoluteEnd = scenarioStart + input.end
			leaves.firstOrNull { leaf ->
				leaf.text == input.name &&
					leaf.textRange.startOffset >= absoluteStart &&
					leaf.textRange.endOffset <= absoluteEnd
			}
		}
	}

	private fun enclosingScenario(element: PsiElement, symbols: ThtrFileSymbols): PsiElement? {
		val file = element.containingFile ?: return null
		val text = file.text
		val lines = text.split('\n')
		val elementLine = lineIndexForOffset(text, element.textRange.startOffset)
		return symbols.scenarios.values
			.filter { scenario ->
				val scenarioLine = lineIndexForOffset(text, scenario.textRange.startOffset)
				val scenarioIndent = lines.getOrNull(scenarioLine)?.let(::leadingSpaceCount) ?: return@filter false
				scenarioLine <= elementLine && elementLine < scenarioEndLine(lines, scenarioLine, scenarioIndent)
			}
			.maxByOrNull { scenario -> scenario.textRange.startOffset }
	}

	private fun scenarioValueDeclarations(scenario: PsiElement, includeActLocals: Boolean): List<PsiElement> {
		val file = scenario.containingFile ?: return emptyList()
		val text = file.text
		val lines = text.split('\n')
		val scenarioLine = lineIndexForOffset(text, scenario.textRange.startOffset)
		val scenarioIndent = lines.getOrNull(scenarioLine)?.let(::leadingSpaceCount) ?: return emptyList()
		val endLine = scenarioEndLine(lines, scenarioLine, scenarioIndent)

		val values = mutableListOf<PsiElement>()
		values += declarationsInLineRange(
			PsiTreeUtil.findChildrenOfType(file, ThtrExportStatement::class.java),
			text,
			scenarioLine,
			endLine,
		)
		if (includeActLocals) {
			values += declarationsInLineRange(
				PsiTreeUtil.findChildrenOfType(file, ThtrPropStatement::class.java),
				text,
				scenarioLine,
				endLine,
			)
			values += declarationsInLineRange(
				PsiTreeUtil.findChildrenOfType(file, ThtrCaptureAuthStatement::class.java),
				text,
				scenarioLine,
				endLine,
			)
		}
		return values
	}

	private fun declarationsInLineRange(
		declarations: Collection<PsiElement>,
		text: String,
		startLine: Int,
		endLine: Int,
	): List<PsiElement> {
		return declarations.mapNotNull { declaration ->
			val line = lineIndexForOffset(text, declaration.textRange.startOffset)
			if (line <= startLine || line >= endLine) {
				return@mapNotNull null
			}
			if (firstIdentifierLikeChild(declaration) == null) {
				return@mapNotNull null
			}
			declaration
		}
	}

	private fun scenarioEndLine(lines: List<String>, scenarioLine: Int, scenarioIndent: Int): Int {
		for (index in scenarioLine + 1 until lines.size) {
			val trimmed = lines[index].trim()
			if (trimmed.isEmpty() || trimmed.startsWith("#")) {
				continue
			}
			val indent = leadingSpaceCount(lines[index])
			if (indent <= scenarioIndent) {
				return index
			}
		}
		return lines.size
	}

	private fun enclosingCallTargetID(element: PsiElement): String? {
		val export = PsiTreeUtil.getParentOfType(element, ThtrExportStatement::class.java) ?: return null
		val fileText = export.containingFile?.text ?: return null
		val lines = fileText.split('\n')
		val exportLine = lineIndexForOffset(fileText, export.textRange.startOffset)
		val exportIndent = lines.getOrNull(exportLine)?.let(::leadingSpaceCount) ?: return null

		for (index in exportLine - 1 downTo 0) {
			val line = lines[index]
			val trimmed = line.trim()
			if (trimmed.isEmpty() || trimmed.startsWith("#")) {
				continue
			}
			val indent = leadingSpaceCount(line)
			if (indent >= exportIndent) {
				continue
			}
			CALL_TARGET_REGEX.find(trimmed)?.groupValues?.get(1)?.let {
				return it
			}
			if (trimmed != ")" && isDeclarationBoundary(trimmed)) {
				return null
			}
		}
		return null
	}

	private fun isDeclarationBoundary(trimmedLine: String): Boolean {
		return trimmedLine.startsWith("stage ") ||
			trimmedLine == "http" ||
			trimmedLine == "state" ||
			trimmedLine.startsWith("scenario ") ||
			trimmedLine.startsWith("act ") ||
			trimmedLine.startsWith("name ") ||
			trimmedLine.startsWith("do ") ||
			trimmedLine.startsWith("log ") ||
			trimmedLine.startsWith("expect ") ||
			trimmedLine.startsWith("eventually ") ||
			trimmedLine.startsWith("prop ") ||
			trimmedLine.startsWith("export ") ||
			trimmedLine.startsWith("on ") ||
			trimmedLine.startsWith("dependency ") ||
			trimmedLine.startsWith("capture_auth ") ||
			trimmedLine.startsWith("backend ") ||
			trimmedLine.startsWith("record ") ||
			trimmedLine.startsWith("pool ")
	}

	private fun leafChildren(element: PsiElement): List<PsiElement> {
		val leaves = mutableListOf<PsiElement>()
		collectLeafChildren(element, leaves)
		return leaves
	}

	private fun collectLeafChildren(element: PsiElement, leaves: MutableList<PsiElement>) {
		var child = element.firstChild
		if (child == null) {
			leaves += element
			return
		}
		while (child != null) {
			collectLeafChildren(child, leaves)
			child = child.nextSibling
		}
	}

	private fun lineIndexForOffset(text: String, offset: Int): Int {
		return text.substring(0, offset.coerceIn(0, text.length)).count { it == '\n' }
	}

	private fun leadingSpaceCount(line: String): Int {
		return line.takeWhile { it == ' ' }.length
	}

	private fun isScenarioInputNameElement(element: PsiElement): Boolean {
		val scenario = PsiTreeUtil.getParentOfType(element, ThtrScenarioDeclaration::class.java) ?: return false
		return scenarioInputNameChildren(scenario).any { it == element }
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
}

private data class InputName(
	val name: String,
	val start: Int,
	val end: Int,
)

enum class ThtrReferenceKind(val label: String) {
	SCENARIO("scenario"),
	ACT("act"),
	VALUE("value"),
}

data class ThtrReferenceTarget(
	val kind: ThtrReferenceKind,
	val name: String,
	val rangeStart: Int,
) {
	fun unresolvedMessage(): String {
		return "Unresolved .thtr ${kind.label} reference: $name"
	}
}

class ThtrFileSymbols {
	val scenarios: MutableMap<String, PsiElement> = linkedMapOf()
	val acts: MutableMap<String, PsiElement> = linkedMapOf()
	val values: MutableMap<String, PsiElement> = linkedMapOf()
	val stateBackends: MutableMap<String, PsiElement> = linkedMapOf()
}
