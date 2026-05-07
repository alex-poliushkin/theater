package com.github.alexpoliushkin.theater.thtrij

import com.intellij.openapi.components.SerializablePersistentStateComponent
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage
import com.intellij.openapi.project.Project

@State(
	name = "TheaterThtrProjectSettings",
	storages = [Storage("theater-thtr.xml")],
)
class ThtrProjectSettings : SerializablePersistentStateComponent<ThtrProjectSettings.State>(State()) {
	var pluginsConfigPath: String
		get() = state.pluginsConfigPath
		set(value) {
			updateState { current ->
				current.copy(pluginsConfigPath = value.trim())
			}
		}

	var pluginsLockPath: String
		get() = state.pluginsLockPath
		set(value) {
			updateState { current ->
				current.copy(pluginsLockPath = value.trim())
			}
		}

	data class State(
		@JvmField val pluginsConfigPath: String = "",
		@JvmField val pluginsLockPath: String = "",
	)

	companion object {
		fun getInstance(project: Project): ThtrProjectSettings =
			project.getService(ThtrProjectSettings::class.java)
	}
}
