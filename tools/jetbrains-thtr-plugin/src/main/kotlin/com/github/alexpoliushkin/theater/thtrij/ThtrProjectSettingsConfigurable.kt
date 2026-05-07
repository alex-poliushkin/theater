package com.github.alexpoliushkin.theater.thtrij

import com.intellij.codeInsight.daemon.DaemonCodeAnalyzer
import com.intellij.openapi.options.Configurable
import com.intellij.openapi.project.Project
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBTextField
import com.intellij.util.ui.FormBuilder
import javax.swing.JComponent
import javax.swing.JPanel

class ThtrProjectSettingsConfigurable(private val project: Project) : Configurable {
	private var panel: JPanel? = null
	private var pluginsConfigField: JBTextField? = null
	private var pluginsLockField: JBTextField? = null

	override fun getDisplayName(): String = "Theater"

	override fun createComponent(): JComponent {
		if (panel == null) {
			pluginsConfigField = JBTextField()
			pluginsLockField = JBTextField()
			panel = FormBuilder.createFormBuilder()
				.addLabeledComponent("Plugins config path:", pluginsConfigField!!)
				.addLabeledComponent("Plugins lock path:", pluginsLockField!!)
				.addComponent(
					JBLabel(
						"<html>Relative paths resolve from the project root. Leave both empty to use built-ins and project plugin manifests.</html>",
					),
				)
				.addComponent(
					JBLabel(
						"<html>Apply refreshes native descriptor-backed diagnostics and completion.</html>",
					),
				)
				.addComponentFillVertically(JPanel(), 0)
				.panel
			reset()
		}

		return panel!!
	}

	override fun isModified(): Boolean {
		val settings = ThtrProjectSettings.getInstance(project)
		val configField = pluginsConfigField ?: return false
		val lockField = pluginsLockField ?: return false
		return normalizePath(configField.text) != settings.pluginsConfigPath ||
			normalizePath(lockField.text) != settings.pluginsLockPath
	}

	override fun apply() {
		val settings = ThtrProjectSettings.getInstance(project)
		val previousConfigPath = settings.pluginsConfigPath
		val previousLockPath = settings.pluginsLockPath
		val nextConfigPath = normalizePath(pluginsConfigField?.text)
		val nextLockPath = normalizePath(pluginsLockField?.text)
		settings.pluginsConfigPath = nextConfigPath
		settings.pluginsLockPath = nextLockPath
		if (previousConfigPath != nextConfigPath || previousLockPath != nextLockPath) {
			DaemonCodeAnalyzer.getInstance(project).restart()
		}
	}

	override fun reset() {
		val settings = ThtrProjectSettings.getInstance(project)
		pluginsConfigField?.text = settings.pluginsConfigPath
		pluginsLockField?.text = settings.pluginsLockPath
	}

	override fun disposeUIResources() {
		panel = null
		pluginsConfigField = null
		pluginsLockField = null
	}

	private fun normalizePath(value: String?): String = value?.trim() ?: ""
}
