package com.github.alexpoliushkin.theater.thtrij

import com.intellij.codeInsight.completion.CompletionContributor
import com.intellij.codeInsight.completion.CompletionParameters
import com.intellij.codeInsight.completion.CompletionProvider
import com.intellij.codeInsight.completion.CompletionResultSet
import com.intellij.codeInsight.completion.CompletionType
import com.intellij.codeInsight.completion.CompletionUtilCore
import com.intellij.codeInsight.completion.InsertionContext
import com.intellij.codeInsight.lookup.LookupElement
import com.intellij.codeInsight.lookup.LookupElementBuilder
import com.intellij.openapi.editor.Document
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.DumbService
import com.intellij.patterns.PlatformPatterns
import com.intellij.psi.PsiFile
import com.intellij.util.ProcessingContext

class ThtrCompletionContributor : CompletionContributor(), DumbAware {
	init {
		extend(
			CompletionType.BASIC,
			PlatformPatterns.psiElement().withLanguage(ThtrLanguage.INSTANCE),
			object : CompletionProvider<CompletionParameters>() {
				override fun addCompletions(
					parameters: CompletionParameters,
					context: ProcessingContext,
					result: CompletionResultSet,
				) {
					val items = ThtrCompletions.items(parameters.originalFile, parameters.offset)
					val completionResult = result.withPrefixMatcher("")
					for (item in items) {
						completionResult.addElement(item.toLookupElement())
					}
				}
			},
		)
	}
}

object ThtrCompletions {
	fun items(file: PsiFile, offset: Int): List<ThtrCompletionItem> {
		val context = completionContext(file, offset)
		val symbols = ThtrSymbols.collect(file)
		val capabilities = ThtrCapabilities.all(file.project)
		val candidates = mutableListOf<ThtrCompletionItem>()

		candidates += stateBackendIDCompletions(context, symbols)
		if (candidates.isEmpty()) {
			candidates += argumentCompletions(context, capabilities)
		}
		if (candidates.isEmpty()) {
			candidates += referenceCompletions(context, file, symbols)
		}
		if (candidates.isEmpty()) {
			candidates += capabilityCompletions(context, capabilities)
		}
		if (candidates.isEmpty()) {
			candidates += structuralKeywordCompletions(context)
		}

		return filterAndDedupe(candidates, context.fragment)
	}
}

data class ThtrCompletionItem(
	val label: String,
	val typeText: String,
	val detail: String = "",
) {
	fun toLookupElement() =
		LookupElementBuilder.create(label)
			.withTypeText(typeText, true)
			.withTailText(if (detail.isBlank()) null else " $detail", true)
			.withInsertHandler(::replaceCompletionFragment)
}

private data class CompletionContext(
	val file: PsiFile,
	val text: String,
	val offset: Int,
	val lines: List<String>,
	val lineIndex: Int,
	val linePrefix: String,
	val trimmedPrefix: String,
	val statementPrefix: String,
	val fragment: String,
	val indent: Int,
)

private fun completionContext(file: PsiFile, rawOffset: Int): CompletionContext {
	val rawText = file.text
	val offset = rawOffset.coerceIn(0, rawText.length)
	val cleanText = rawText.replace(CompletionUtilCore.DUMMY_IDENTIFIER_TRIMMED, "")
	val lineStart = rawText.lastIndexOf('\n', (offset - 1).coerceAtLeast(0)).let { if (it < 0) 0 else it + 1 }
	val rawPrefix = rawText.substring(lineStart, offset)
	val prefix = rawPrefix.replace(CompletionUtilCore.DUMMY_IDENTIFIER_TRIMMED, "")
	val fragment = currentFragment(prefix)
	val statementPrefix = prefix.dropLast(fragment.length)
	val lineIndex = rawText.substring(0, offset).count { it == '\n' }
	return CompletionContext(
		file = file,
		text = cleanText,
		offset = offset.coerceAtMost(cleanText.length),
		lines = cleanText.split('\n'),
		lineIndex = lineIndex,
		linePrefix = prefix,
		trimmedPrefix = prefix.trim(),
		statementPrefix = statementPrefix.trim(),
		fragment = fragment,
		indent = prefix.takeWhile { it == ' ' }.length,
	)
}

private fun stateBackendIDCompletions(context: CompletionContext, symbols: ThtrFileSymbols): List<ThtrCompletionItem> {
	if (!Regex("""backend:\s*[\w./-]*$""").containsMatchIn(context.linePrefix)) {
		return emptyList()
	}
	return symbols.stateBackends.keys.map { ThtrCompletionItem(it, "state backend id") }
}

private fun argumentCompletions(
	context: CompletionContext,
	capabilities: List<ThtrCapability>,
): List<ThtrCompletionItem> {
	if (isArgumentValuePrefix(context.linePrefix)) {
		return emptyList()
	}
	val ref = callRefBeforePrefix(context.linePrefix) ?: return emptyList()
	val capability = capabilities.firstOrNull { it.completionLabel == ref || it.ref == ref } ?: return emptyList()
	return capability.parameters.map { parameter ->
		val type = if (parameter.required) "required argument" else "optional argument"
		ThtrCompletionItem(parameter.name, type, parameter.valueType)
	}
}

private fun referenceCompletions(
	context: CompletionContext,
	file: PsiFile,
	symbols: ThtrFileSymbols,
): List<ThtrCompletionItem> {
	return when {
		context.trimmedPrefix.startsWith("call ") && context.trimmedPrefix.contains("=") ->
			ThtrSymbols.scenarioTargets(file, useProjectIndex = !DumbService.isDumb(file.project)).keys
				.map { ThtrCompletionItem(it, "scenario") }
		context.trimmedPrefix.startsWith("on ") && context.trimmedPrefix.contains("->") ->
			symbols.acts.keys.map { ThtrCompletionItem(it, "act") }
		context.fragment.startsWith("$") || context.linePrefix.contains("$") ->
			symbols.values.keys.map { ThtrCompletionItem("$$it", "value reference") }
		else -> emptyList()
	}
}

private fun capabilityCompletions(
	context: CompletionContext,
	capabilities: List<ThtrCapability>,
): List<ThtrCompletionItem> {
	if (context.fragment.startsWith("generate.")) {
		return capabilityItems(capabilities, ThtrCapabilityKind.GENERATOR)
	}
	return when {
		context.trimmedPrefix.startsWith("do repeatable ") || context.trimmedPrefix.startsWith("do ") ->
			capabilityItems(capabilities, ThtrCapabilityKind.ACTION, ThtrCapabilityKind.STATE_VERB)
		isStateBackendAssignmentPrefix(context.trimmedPrefix) ->
			capabilityItems(capabilities, ThtrCapabilityKind.STATE_BACKEND)
		isStateAliasAssignmentPrefix(context.trimmedPrefix, "record") ->
			listOf(ThtrCompletionItem("state.record", "state alias constructor"))
		isStateAliasAssignmentPrefix(context.trimmedPrefix, "pool") ->
			listOf(ThtrCompletionItem("state.pool", "state alias constructor"))
		isLogValuePrefix(context.trimmedPrefix) ->
			logValueCompletions(context.trimmedPrefix)
		hasTransformPrefix(context.trimmedPrefix) ->
			capabilityItems(capabilities, ThtrCapabilityKind.TRANSFORM) + selectorCompletions()
		context.trimmedPrefix.startsWith("prop ") && context.trimmedPrefix.contains("=") ->
			capabilityItems(capabilities, ThtrCapabilityKind.INVENTORY, ThtrCapabilityKind.GENERATOR)
		containsAssertPrefix(context.trimmedPrefix) ->
			capabilityItems(capabilities, ThtrCapabilityKind.MATCHER)
		else -> emptyList()
	}
}

private fun capabilityItems(capabilities: List<ThtrCapability>, vararg kinds: ThtrCapabilityKind): List<ThtrCompletionItem> {
	val allowed = kinds.toSet()
	return capabilities
		.filter { it.kind in allowed }
		.map { ThtrCompletionItem(it.completionLabel, it.kind.familyLabel, it.detail()) }
}

private fun selectorCompletions(): List<ThtrCompletionItem> {
	return listOf("field", "decode", "path", "pick", "regexp").map { ThtrCompletionItem(it, "selector") }
}

private fun logValueCompletions(prefix: String): List<ThtrCompletionItem> {
	val afterPipe = prefix.substringAfterLast("|", missingDelimiterValue = "").trimStart()
	if (prefix.contains("|") && !afterPipe.contains(" ")) {
		return selectorCompletions()
	}
	return listOf(
		ThtrCompletionItem("field", "log value"),
		ThtrCompletionItem("object", "log value"),
		ThtrCompletionItem("list", "log value"),
	)
}

private fun structuralKeywordCompletions(context: CompletionContext): List<ThtrCompletionItem> {
	if (context.statementPrefix.isNotBlank()) {
		return emptyList()
	}
	val keywords = when {
		context.indent == 0 -> listOf("stage", "http", "state", "scenario", "call")
		nearestBlockKind(context.lines, context.lineIndex) == "state" -> listOf("backend", "record", "pool")
		nearestBlockKind(context.lines, context.lineIndex) == "scenario" && context.indent <= 2 -> listOf("act")
		nearestBlockKind(context.lines, context.lineIndex) == "act" -> actStructuralKeywords(context)
		else -> emptyList()
	}
	return keywords.map { ThtrCompletionItem(it, "keyword") }
}

private fun actStructuralKeywords(context: CompletionContext): List<String> {
	val keywords = mutableListOf("do", "expect", "eventually", "prop", "export", "on", "capture_auth")
	if (actCanAcceptLog(context)) {
		keywords += "log"
	}
	return keywords
}

private fun filterAndDedupe(items: List<ThtrCompletionItem>, fragment: String): List<ThtrCompletionItem> {
	val normalizedFragment = fragment.removePrefix("$")
	val filtered = if (normalizedFragment.isBlank()) {
		items
	} else {
		items.filter { item ->
			item.label.startsWith(fragment) || item.label.removePrefix("$").startsWith(normalizedFragment)
		}
	}
	return filtered.distinctBy { it.label }.sortedBy { it.label }
}

private fun replaceCompletionFragment(context: InsertionContext, item: LookupElement) {
	val document = context.document
	val replacement = item.lookupString
	val start = completionFragmentStart(document, context.startOffset)
	val tail = context.tailOffset.coerceIn(context.startOffset, document.textLength)
	document.replaceString(start, tail, replacement)
	val caretOffset = start + replacement.length
	context.editor.caretModel.moveToOffset(caretOffset)
	context.tailOffset = caretOffset
	context.commitDocument()
}

private fun completionFragmentStart(document: Document, offset: Int): Int {
	val text = document.charsSequence
	var start = offset.coerceIn(0, document.textLength)
	while (start > 0 && isCompletionChar(text[start - 1])) {
		start--
	}
	return start
}

private fun currentFragment(prefix: String): String {
	var start = prefix.length
	while (start > 0 && isCompletionChar(prefix[start - 1])) {
		start--
	}
	return prefix.substring(start)
}

private fun isCompletionChar(char: Char): Boolean {
	return char.isLetterOrDigit() || char == '_' || char == '-' || char == '.' || char == '$' || char == '/'
}

private fun isArgumentValuePrefix(prefix: String): Boolean {
	val fragment = prefix.substringAfterLast('(').substringAfterLast(',')
	return fragment.contains(":")
}

private fun callRefBeforePrefix(prefix: String): String? {
	val open = prefix.lastIndexOf('(')
	if (open < 1) {
		return null
	}
	var start = open
	while (start > 0 && isCompletionChar(prefix[start - 1])) {
		start--
	}
	return prefix.substring(start, open).takeIf { it.isNotBlank() }
}

private fun isStateBackendAssignmentPrefix(prefix: String): Boolean {
	return prefix.startsWith("backend ") && prefix.contains("=")
}

private fun isStateAliasAssignmentPrefix(prefix: String, sectionKind: String): Boolean {
	return prefix.startsWith("$sectionKind ") && prefix.contains("=")
}

private fun hasTransformPrefix(prefix: String): Boolean {
	if (!prefix.contains("|")) {
		return false
	}
	val afterPipe = prefix.substringAfterLast("|").trimStart()
	return afterPipe.isNotEmpty() && !afterPipe.contains(" ")
}

private fun isLogValuePrefix(prefix: String): Boolean {
	return prefix.startsWith("log ") && prefix.contains("=")
}

private fun actCanAcceptLog(context: CompletionContext): Boolean {
	for (index in (context.lineIndex - 1) downTo 0) {
		val line = context.lines[index]
		val trimmed = line.trim()
		if (trimmed.isEmpty() || trimmed.startsWith("#")) {
			continue
		}
		val indent = line.takeWhile { it == ' ' }.length
		if (indent < context.indent) {
			break
		}
		if (indent > context.indent) {
			continue
		}
		when {
			trimmed.startsWith("expect ") ||
				trimmed.startsWith("export ") ||
				trimmed.startsWith("on ") ->
				return false
			trimmed.startsWith("do ") || trimmed.startsWith("capture_auth ") -> return true
		}
	}
	return false
}

private fun containsAssertPrefix(prefix: String): Boolean {
	return Regex("""\bassert\s+[\w.-]*$""").containsMatchIn(prefix)
}

private fun nearestBlockKind(lines: List<String>, lineIndex: Int): String? {
	for (index in (lineIndex - 1) downTo 0) {
		val line = lines[index]
		val trimmed = line.trim()
		if (trimmed.isEmpty() || trimmed.startsWith("#")) {
			continue
		}
		val indent = line.takeWhile { it == ' ' }.length
		when {
			indent == 0 && trimmed.startsWith("state") -> return "state"
			indent == 0 && trimmed.startsWith("scenario ") -> return "scenario"
			indent == 0 && trimmed.startsWith("stage ") -> return null
			indent <= 2 && trimmed.startsWith("act ") -> return "act"
		}
	}
	return null
}
