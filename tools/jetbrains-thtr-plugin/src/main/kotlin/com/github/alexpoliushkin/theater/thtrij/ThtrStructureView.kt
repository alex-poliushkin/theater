package com.github.alexpoliushkin.theater.thtrij

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrActDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrCallDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrFile
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrScenarioDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrStageDeclaration
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes
import com.intellij.ide.structureView.StructureViewBuilder
import com.intellij.ide.structureView.StructureViewModel
import com.intellij.ide.structureView.StructureViewTreeElement
import com.intellij.ide.structureView.TextEditorBasedStructureViewModel
import com.intellij.ide.structureView.TreeBasedStructureViewBuilder
import com.intellij.ide.util.treeView.smartTree.TreeElement
import com.intellij.lang.PsiStructureViewFactory
import com.intellij.navigation.ItemPresentation
import com.intellij.openapi.editor.Editor
import com.intellij.pom.Navigatable
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import com.intellij.psi.util.PsiTreeUtil
import javax.swing.Icon

class ThtrStructureViewFactory : PsiStructureViewFactory {
	override fun getStructureViewBuilder(psiFile: PsiFile): StructureViewBuilder? {
		if (psiFile !is ThtrFile) {
			return null
		}
		return object : TreeBasedStructureViewBuilder() {
			override fun createStructureViewModel(editor: Editor?): StructureViewModel {
				return ThtrStructureViewModel(editor, psiFile)
			}
		}
	}
}

private class ThtrStructureViewModel(
	editor: Editor?,
	file: PsiFile,
) : TextEditorBasedStructureViewModel(editor, file) {
	override fun getRoot(): StructureViewTreeElement = ThtrStructureElement(psiFile)

	override fun getSuitableClasses(): Array<Class<*>> = arrayOf(
		ThtrStageDeclaration::class.java,
		ThtrScenarioDeclaration::class.java,
		ThtrCallDeclaration::class.java,
		ThtrActDeclaration::class.java,
	)
}

private class ThtrStructureElement(
	private val element: PsiElement,
) : StructureViewTreeElement {
	override fun getValue(): Any = element

	override fun getPresentation(): ItemPresentation = ThtrStructurePresentation(structureLabel(element), structureLocation(element))

	override fun getChildren(): Array<TreeElement> {
		return structureChildren(element)
			.map { ThtrStructureElement(it) }
			.toTypedArray()
	}

	override fun navigate(requestFocus: Boolean) {
		(element as? Navigatable)?.navigate(requestFocus)
	}

	override fun canNavigate(): Boolean = (element as? Navigatable)?.canNavigate() == true

	override fun canNavigateToSource(): Boolean = (element as? Navigatable)?.canNavigateToSource() == true
}

private class ThtrStructurePresentation(
	private val text: String,
	private val location: String?,
) : ItemPresentation {
	override fun getPresentableText(): String = text

	override fun getLocationString(): String? = location

	override fun getIcon(unused: Boolean): Icon? = null
}

private fun structureChildren(element: PsiElement): List<PsiElement> {
	return when (element) {
		is ThtrFile -> fileStructureChildren(element)
		is ThtrScenarioDeclaration -> scenarioActs(element)
		else -> emptyList()
	}
}

private fun fileStructureChildren(file: PsiFile): List<PsiElement> {
	return PsiTreeUtil.findChildrenOfAnyType(
		file,
		ThtrStageDeclaration::class.java,
		ThtrScenarioDeclaration::class.java,
		ThtrCallDeclaration::class.java,
	).sortedBy { it.textRange.startOffset }
}

private fun scenarioActs(scenario: ThtrScenarioDeclaration): List<PsiElement> {
	val file = scenario.containingFile
	val endOffset = nextTopLevelOffset(file, scenario.textRange.startOffset)
	return PsiTreeUtil.findChildrenOfType(file, ThtrActDeclaration::class.java)
		.filter { it.textRange.startOffset > scenario.textRange.startOffset && it.textRange.startOffset < endOffset }
		.sortedBy { it.textRange.startOffset }
}

private fun nextTopLevelOffset(file: PsiFile, startOffset: Int): Int {
	return PsiTreeUtil.findChildrenOfAnyType(
		file,
		ThtrStageDeclaration::class.java,
		ThtrScenarioDeclaration::class.java,
		ThtrCallDeclaration::class.java,
	).map { it.textRange.startOffset }
		.filter { it > startOffset }
		.minOrNull()
		?: file.textLength
}

private fun structureLabel(element: PsiElement): String {
	return when (element) {
		is ThtrFile -> element.name
		is ThtrStageDeclaration -> "stage ${identifierText(element)}"
		is ThtrScenarioDeclaration -> "scenario ${identifierText(element)}"
		is ThtrCallDeclaration -> callLabel(element)
		is ThtrActDeclaration -> "act ${identifierText(element)}"
		else -> element.text.lines().firstOrNull()?.trim().orEmpty()
	}
}

private fun structureLocation(element: PsiElement): String? {
	return when (element) {
		is ThtrActDeclaration -> "act"
		is ThtrCallDeclaration -> "call"
		is ThtrScenarioDeclaration -> "scenario"
		is ThtrStageDeclaration -> "stage"
		else -> null
	}
}

private fun callLabel(call: ThtrCallDeclaration): String {
	val name = identifierText(call)
	val target = callTargetText(call)
	return if (target == null) "call $name" else "call $name -> $target"
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
	return firstIdentifierLikeChild(element)?.text ?: "<unnamed>"
}

private fun firstIdentifierLikeChild(element: PsiElement): PsiElement? {
	var child = element.firstChild
	while (child != null) {
		if (isIdentifierLike(child)) {
			return child
		}
		child = child.nextSibling
	}
	return null
}

private fun isIdentifierLike(element: PsiElement): Boolean {
	val tokenType = element.node?.elementType
	return tokenType == ThtrTypes.IDENTIFIER || tokenType == ThtrTypes.DOTTED_REF
}
