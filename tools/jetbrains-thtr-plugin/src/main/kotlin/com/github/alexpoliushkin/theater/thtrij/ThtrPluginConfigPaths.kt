package com.github.alexpoliushkin.theater.thtrij

import com.intellij.openapi.project.Project
import java.nio.file.Files
import java.nio.file.InvalidPathException
import java.nio.file.Path
import java.nio.file.Paths

object ThtrPluginConfigPaths {
	const val ENV_PLUGINS_CONFIG = "THEATER_PLUGINS_CONFIG"
	const val ENV_PLUGINS_LOCK = "THEATER_PLUGINS_LOCK"

	data class Resolution(
		val environment: Map<String, String>,
		val problem: String?,
	)

	fun resolve(project: Project): Resolution {
		val settings = ThtrProjectSettings.getInstance(project)
		val configRaw = settings.pluginsConfigPath.trim()
		val lockRaw = settings.pluginsLockPath.trim()
		if (configRaw.isEmpty() && lockRaw.isEmpty()) {
			return Resolution(emptyMap(), null)
		}
		if (configRaw.isEmpty()) {
			return Resolution(emptyMap(), "Set a Theater plugins config path or clear the plugins lock path.")
		}
		if (lockRaw.isEmpty()) {
			return Resolution(emptyMap(), "Set a Theater plugins lock path or clear the plugins config path.")
		}

		val configPath = normalizeReadableFile(project, configRaw)
			?: return Resolution(emptyMap(), "Theater plugins config path is not a readable file.")
		val lockPath = normalizeReadableFile(project, lockRaw)
			?: return Resolution(emptyMap(), "Theater plugins lock path is not a readable file.")

		return Resolution(
			mapOf(
				ENV_PLUGINS_CONFIG to configPath.toString(),
				ENV_PLUGINS_LOCK to lockPath.toString(),
			),
			null,
		)
	}

	private fun normalizeReadableFile(project: Project, value: String): Path? {
		val path = try {
			Paths.get(value.trim())
		} catch (_: InvalidPathException) {
			return null
		}

		if (!path.isAbsolute && project.basePath == null) {
			return null
		}

		val resolved = if (path.isAbsolute) path else Paths.get(project.basePath!!).resolve(path)
		val normalized = resolved.normalize()

		if (!Files.isRegularFile(normalized) || !Files.isReadable(normalized)) {
			return null
		}
		return normalized
	}
}
