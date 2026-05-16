package com.github.alexpoliushkin.theater.thtrij

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.intellij.openapi.project.DumbService
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.vfs.VfsUtilCore
import com.intellij.psi.search.FilenameIndex
import com.intellij.psi.search.GlobalSearchScope
import java.security.MessageDigest

private const val REGISTRY_SCHEMA_VERSION = "theater.plugin.registry/v1alpha1"
private const val LOCK_SCHEMA_VERSION = "theater.plugin.lock/v1alpha1"

object ThtrCapabilities {
	fun all(project: Project, useProjectIndex: Boolean = !DumbService.isDumb(project)): List<ThtrCapability> {
		return builtinCapabilities() + pluginCapabilities(project, useProjectIndex)
	}

	fun byLabel(project: Project, label: String, useProjectIndex: Boolean = !DumbService.isDumb(project)): ThtrCapability? {
		return all(project, useProjectIndex).firstOrNull { it.completionLabel == label || it.ref == label }
	}

	fun forKinds(project: Project, vararg kinds: ThtrCapabilityKind): List<ThtrCapability> {
		val allowed = kinds.toSet()
		return all(project).filter { it.kind in allowed }
	}

	fun renderDocumentation(capability: ThtrCapability): String {
		val signature = capability.signature()
		return buildString {
			append("<b><code>")
			append(escapeHtml(capability.completionLabel))
			append("</code></b>")
			append("<br/>")
			append(escapeHtml(capability.detail()))
			if (signature.isNotEmpty()) {
				append("<br/><br/><b>Signature:</b> <code>")
				append(escapeHtml(signature))
				append("</code>")
			}
			if (capability.parameters.isNotEmpty()) {
				append("<br/><br/><b>Parameters</b><ul>")
				for (parameter in capability.parameters) {
					append("<li><code>")
					append(escapeHtml(parameter.name))
					append("</code>: <code>")
					append(escapeHtml(parameter.valueType))
					append("</code>")
					if (parameter.required) {
						append(" required")
					}
					if (parameter.description.isNotBlank()) {
						append(" - ")
						append(escapeHtml(parameter.description))
					}
					append("</li>")
				}
				append("</ul>")
			}
		}
	}
}

enum class ThtrCapabilityKind(val familyLabel: String) {
	ACTION("action"),
	GENERATOR("generator"),
	INVENTORY("inventory"),
	MATCHER("matcher"),
	STATE_BACKEND("state backend"),
	STATE_VERB("state verb"),
	TRANSFORM("transform"),
}

data class ThtrCapability(
	val kind: ThtrCapabilityKind,
	val ref: String,
	val summary: String,
	val provider: ThtrCapabilityProvider,
	val parameters: List<ThtrCapabilityParameter> = emptyList(),
) {
	val completionLabel: String
		get() = if (kind == ThtrCapabilityKind.GENERATOR && !ref.startsWith("generate.")) "generate.$ref" else ref

	fun detail(): String {
		val providerText = when (provider) {
			ThtrCapabilityProvider.BUILTIN -> "built-in ${kind.familyLabel}"
			is ThtrCapabilityProvider.Plugin -> "plugin ${kind.familyLabel} from ${provider.id}@${provider.version}"
		}
		if (summary.isBlank()) {
			return providerText
		}
		return "$providerText: $summary"
	}

	fun signature(): String {
		val labels = parameters.joinToString(", ") { it.label() }
		return "$completionLabel($labels)"
	}
}

sealed interface ThtrCapabilityProvider {
	data object BUILTIN : ThtrCapabilityProvider
	data class Plugin(val id: String, val version: String) : ThtrCapabilityProvider
}

data class ThtrCapabilityParameter(
	val name: String,
	val valueType: String,
	val required: Boolean,
	val description: String = "",
) {
	fun label(): String {
		val suffix = if (required) "" else "?"
		return "$name$suffix: $valueType"
	}
}

object ThtrDescriptorMetadata {
	fun load(project: Project, useProjectIndex: Boolean = !DumbService.isDumb(project)): ThtrDescriptorMetadataResult {
		if (!useProjectIndex) {
			return ThtrDescriptorMetadataResult(emptyList())
		}
		val settings = ThtrProjectSettings.getInstance(project)
		if (settings.pluginsConfigPath.isNotBlank()) {
			return loadRegistry(project, settings.pluginsConfigPath, settings.pluginsLockPath)
		}
		return ThtrDescriptorMetadataResult(scanProjectManifests(project))
	}
}

data class ThtrDescriptorMetadataResult(
	val capabilities: List<ThtrCapability>,
	val issues: List<ThtrDescriptorMetadataIssue> = emptyList(),
)

data class ThtrDescriptorMetadataIssue(
	val message: String,
)

private fun pluginCapabilities(project: Project, useProjectIndex: Boolean): List<ThtrCapability> {
	return ThtrDescriptorMetadata.load(project, useProjectIndex).capabilities
}

private fun scanProjectManifests(project: Project): List<ThtrCapability> {
	val scope = GlobalSearchScope.projectScope(project)
	val capabilities = mutableListOf<ThtrCapability>()
	for (file in FilenameIndex.getAllFilesByExt(project, "json", scope)) {
		if (!file.path.endsWith("/manifest.json") || !file.path.contains("/plugins/")) {
			continue
		}
		val text = runCatching { VfsUtilCore.loadText(file) }.getOrNull() ?: continue
		capabilities += parsePluginManifest(text)
	}
	return capabilities.sortedWith(compareBy({ it.kind.ordinal }, { it.completionLabel }))
}

private fun loadRegistry(project: Project, configPath: String, lockPath: String): ThtrDescriptorMetadataResult {
	val issues = mutableListOf<ThtrDescriptorMetadataIssue>()
	val capabilities = mutableListOf<ThtrCapability>()
	val configFile = resolveSettingsFile(project, configPath)
	if (configFile == null) {
		return ThtrDescriptorMetadataResult(
			emptyList(),
			listOf(ThtrDescriptorMetadataIssue("Theater plugin registry config does not exist: $configPath")),
		)
	}
	val config = readJsonObject(configFile)
	if (config == null) {
		return ThtrDescriptorMetadataResult(
			emptyList(),
			listOf(ThtrDescriptorMetadataIssue("Theater plugin registry config is invalid JSON: $configPath")),
		)
	}
	if (config.stringProperty("schema") != REGISTRY_SCHEMA_VERSION) {
		return ThtrDescriptorMetadataResult(
			emptyList(),
			listOf(ThtrDescriptorMetadataIssue("Theater plugin registry config has unsupported schema: $configPath")),
		)
	}
	val plugins = config.objectProperty("plugins")
	if (plugins == null || plugins.entrySet().isEmpty()) {
		return ThtrDescriptorMetadataResult(
			emptyList(),
			listOf(ThtrDescriptorMetadataIssue("Theater plugin registry config must declare at least one plugin: $configPath")),
		)
	}
	val lock = loadLock(project, lockPath)
	if (lock.issues.isNotEmpty()) {
		return ThtrDescriptorMetadataResult(emptyList(), lock.issues)
	}

	for (entry in plugins.entrySet().sortedBy { it.key }) {
		val id = entry.key
		val plugin = entry.value.asJsonObjectOrNull()
		if (plugin == null) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin registry entry is invalid: $id")
			continue
		}
		val manifestPath = plugin.stringProperty("manifest")
		if (manifestPath == null) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin registry entry $id is missing manifest")
			continue
		}
		if (!hasCommand(plugin)) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin registry entry $id is missing exec.command")
			continue
		}
		if (!hasAllowedCapabilities(plugin)) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin registry entry $id is missing allow_capabilities")
			continue
		}
		val manifestFile = resolveRelativeFile(configFile.parent, manifestPath)
		if (manifestFile == null) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin manifest does not exist for $id: $manifestPath")
			continue
		}
		val manifestText = runCatching { VfsUtilCore.loadText(manifestFile) }.getOrNull()
		if (manifestText == null) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin manifest is unreadable for $id: $manifestPath")
			continue
		}
		val manifestRoot = runCatching { JsonParser.parseString(manifestText).asJsonObject }.getOrNull()
		if (manifestRoot == null) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin manifest is invalid JSON for $id: $manifestPath")
			continue
		}
		val manifestID = manifestRoot.objectProperty("plugin")?.stringProperty("id")
		if (manifestID != id) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin manifest id does not match registry entry: $id")
			continue
		}
		val expectedChecksum = lock.plugins?.get(id)?.asJsonObjectOrNull()?.stringProperty("manifest_sha256")
		if (lock.plugins != null) {
			if (expectedChecksum == null) {
				issues += ThtrDescriptorMetadataIssue("Theater plugin lock entry is missing for $id")
				continue
			}
			if (expectedChecksum != sha256(manifestText)) {
				issues += ThtrDescriptorMetadataIssue("Theater plugin manifest checksum mismatch for $id")
				continue
			}
		}
		val pluginCapabilities = runCatching { parsePluginManifest(manifestText) }.getOrDefault(emptyList())
		if (pluginCapabilities.isEmpty()) {
			issues += ThtrDescriptorMetadataIssue("Theater plugin manifest declares no usable capabilities for $id")
			continue
		}
		capabilities += pluginCapabilities
	}

	return ThtrDescriptorMetadataResult(
		capabilities.sortedWith(compareBy({ it.kind.ordinal }, { it.completionLabel })),
		issues,
	)
}

private data class DescriptorLock(
	val plugins: JsonObject?,
	val issues: List<ThtrDescriptorMetadataIssue>,
)

private fun loadLock(project: Project, lockPath: String): DescriptorLock {
	if (lockPath.isBlank()) {
		return DescriptorLock(null, emptyList())
	}
	val file = resolveSettingsFile(project, lockPath)
	if (file == null) {
		return DescriptorLock(
			null,
			listOf(ThtrDescriptorMetadataIssue("Theater plugin lock file does not exist: $lockPath")),
		)
	}
	val lock = readJsonObject(file)
	if (lock == null) {
		return DescriptorLock(
			null,
			listOf(ThtrDescriptorMetadataIssue("Theater plugin lock file is invalid JSON: $lockPath")),
		)
	}
	if (lock.stringProperty("schema") != LOCK_SCHEMA_VERSION) {
		return DescriptorLock(
			null,
			listOf(ThtrDescriptorMetadataIssue("Theater plugin lock file has unsupported schema: $lockPath")),
		)
	}
	val plugins = lock.objectProperty("plugins")
	if (plugins == null || plugins.entrySet().isEmpty()) {
		return DescriptorLock(
			null,
			listOf(ThtrDescriptorMetadataIssue("Theater plugin lock file must declare at least one plugin: $lockPath")),
		)
	}
	return DescriptorLock(plugins, emptyList())
}

private fun readJsonObject(file: VirtualFile): JsonObject? {
	val text = runCatching { VfsUtilCore.loadText(file) }.getOrNull() ?: return null
	return runCatching { JsonParser.parseString(text).asJsonObject }.getOrNull()
}

private fun resolveSettingsFile(project: Project, configuredPath: String): VirtualFile? {
	val root = project.basePath?.let { LocalFileSystem.getInstance().findFileByPath(it) }
	return resolveRelativeFile(root, configuredPath) ?: findProjectFileByPath(project, configuredPath)
}

private fun resolveRelativeFile(root: VirtualFile?, configuredPath: String): VirtualFile? {
	val path = configuredPath.trim()
	if (path.isEmpty()) {
		return null
	}
	if (path.startsWith("/")) {
		return LocalFileSystem.getInstance().findFileByPath(path)
	}
	return root?.findFileByRelativePath(path)
}

private fun findProjectFileByPath(project: Project, configuredPath: String): VirtualFile? {
	val path = configuredPath.trim().removePrefix("/")
	if (path.isEmpty() || !path.endsWith(".json")) {
		return null
	}
	val scope = GlobalSearchScope.projectScope(project)
	return FilenameIndex.getAllFilesByExt(project, "json", scope)
		.firstOrNull { file -> file.path.endsWith("/$path") }
}

private fun hasCommand(plugin: JsonObject): Boolean {
	val command = plugin.objectProperty("exec")?.arrayProperty("command") ?: return false
	return command.any { it.asStringOrNull()?.isNotBlank() == true }
}

private fun hasAllowedCapabilities(plugin: JsonObject): Boolean {
	val allowed = plugin.arrayProperty("allow_capabilities") ?: return false
	return allowed.any { it.asStringOrNull()?.isNotBlank() == true }
}

private fun sha256(text: String): String {
	val digest = MessageDigest.getInstance("SHA-256").digest(text.toByteArray(Charsets.UTF_8))
	return "sha256:" + digest.joinToString("") { "%02x".format(it) }
}

private fun parsePluginManifest(text: String): List<ThtrCapability> {
	val root = runCatching { JsonParser.parseString(text).asJsonObject }.getOrNull() ?: return emptyList()
	val plugin = root.getAsJsonObject("plugin") ?: return emptyList()
	val provider = ThtrCapabilityProvider.Plugin(
		id = plugin.stringProperty("id") ?: return emptyList(),
		version = plugin.stringProperty("version") ?: "",
	)
	val items = root.getAsJsonArray("capabilities") ?: return emptyList()
	val capabilities = mutableListOf<ThtrCapability>()
	for (item in items) {
		val capability = item.asJsonObjectOrNull() ?: continue
		val kind = pluginCapabilityKind(capability.stringProperty("kind")) ?: continue
		val ref = capability.stringProperty("name") ?: continue
		capabilities += ThtrCapability(
			kind = kind,
			ref = ref,
			summary = capability.stringProperty("summary") ?: "",
			provider = provider,
			parameters = schemaParameters(capability.getAsJsonObject("property_schema")),
		)
	}
	return capabilities
}

private fun pluginCapabilityKind(kind: String?): ThtrCapabilityKind? {
	return when (kind) {
		"action" -> ThtrCapabilityKind.ACTION
		"generator" -> ThtrCapabilityKind.GENERATOR
		"inventory" -> ThtrCapabilityKind.INVENTORY
		"matcher" -> ThtrCapabilityKind.MATCHER
		"state_backend" -> ThtrCapabilityKind.STATE_BACKEND
		"transform" -> ThtrCapabilityKind.TRANSFORM
		else -> null
	}
}

private fun schemaParameters(schema: JsonObject?): List<ThtrCapabilityParameter> {
	if (schema == null) {
		return emptyList()
	}
	val required = schema.getAsJsonArray("required")
		?.mapNotNull { it.asStringOrNull() }
		?.toSet()
		?: emptySet()
	val properties = schema.getAsJsonObject("properties") ?: return emptyList()
	return properties.entrySet()
		.sortedBy { it.key }
		.map { entry ->
			val property = entry.value.asJsonObjectOrNull()
			ThtrCapabilityParameter(
				name = entry.key,
				valueType = schemaType(property),
				required = entry.key in required,
				description = property?.stringProperty("description") ?: "",
			)
		}
}

private fun schemaType(schema: JsonObject?): String {
	val type = schema?.get("type") ?: return "any"
	if (type.isJsonArray) {
		return type.asJsonArray.mapNotNull { it.asStringOrNull() }.sorted().joinToString("|").ifEmpty { "any" }
	}
	return type.asStringOrNull() ?: "any"
}

private fun JsonObject.stringProperty(name: String): String? {
	return get(name)?.asStringOrNull()?.trim()?.takeIf { it.isNotEmpty() }
}

private fun JsonObject.objectProperty(name: String): JsonObject? {
	return get(name)?.asJsonObjectOrNull()
}

private fun JsonObject.arrayProperty(name: String): com.google.gson.JsonArray? {
	return get(name)?.asJsonArrayOrNull()
}

private fun com.google.gson.JsonElement.asStringOrNull(): String? {
	if (!isJsonPrimitive || !asJsonPrimitive.isString) {
		return null
	}
	return asString
}

private fun com.google.gson.JsonElement.asJsonObjectOrNull(): JsonObject? {
	return if (isJsonObject) asJsonObject else null
}

private fun com.google.gson.JsonElement.asJsonArrayOrNull(): com.google.gson.JsonArray? {
	return if (isJsonArray) asJsonArray else null
}

private fun builtinCapabilities(): List<ThtrCapability> {
	val builtins = listOf(
		ThtrCapability(
			ThtrCapabilityKind.ACTION,
			"action.http",
			"Perform an HTTP request.",
			ThtrCapabilityProvider.BUILTIN,
			listOf(
				ThtrCapabilityParameter("method", "string", true),
				ThtrCapabilityParameter("url", "string", true),
				ThtrCapabilityParameter("timeout", "duration", false),
			),
		),
		ThtrCapability(
			ThtrCapabilityKind.ACTION,
			"action.command",
			"Run a local command.",
			ThtrCapabilityProvider.BUILTIN,
			listOf(
				ThtrCapabilityParameter("executable", "string", true),
				ThtrCapabilityParameter("args", "list<string>", false),
				ThtrCapabilityParameter("env", "object<string>", false),
				ThtrCapabilityParameter("stdin", "string", false),
				ThtrCapabilityParameter("timeout", "duration", false),
				ThtrCapabilityParameter("working_dir", "string", false),
			),
		),
		ThtrCapability(ThtrCapabilityKind.ACTION, "action.generate", "Run generator-backed action output.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.STATE_VERB, "state.read", "Read a persistent state record.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.STATE_VERB, "state.update", "Update a persistent state record.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.STATE_VERB, "state.claim", "Claim an item from a persistent state pool.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.STATE_VERB, "state.renew", "Renew a persistent state lease.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.STATE_VERB, "state.release", "Release a persistent state lease.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.STATE_VERB, "state.consume", "Consume a persistent state lease.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.INVENTORY, "inventory.env", "Read a value from the process environment.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.INVENTORY, "inventory.file", "Read a value from a file.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(
			ThtrCapabilityKind.INVENTORY,
			"inventory.http.get",
			"Fetch data with an HTTP GET request.",
			ThtrCapabilityProvider.BUILTIN,
			listOf(ThtrCapabilityParameter("url", "string", true)),
		),
		ThtrCapability(ThtrCapabilityKind.INVENTORY, "inventory.state.record", "Use a state record as inventory.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.INVENTORY, "inventory.state.pool", "Use a state pool as inventory.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(
			ThtrCapabilityKind.STATE_BACKEND,
			"state.backend.file",
			"Store state in a local file backend.",
			ThtrCapabilityProvider.BUILTIN,
			listOf(ThtrCapabilityParameter("root", "string", true)),
		),
		ThtrCapability(ThtrCapabilityKind.TRANSFORM, "json.decode", "Decode JSON text into a value.", ThtrCapabilityProvider.BUILTIN),
		ThtrCapability(ThtrCapabilityKind.TRANSFORM, "csv.decode", "Decode CSV text into a value.", ThtrCapabilityProvider.BUILTIN),
	)
	val matchers = listOf(
		"expectation.equal",
		"expectation.contains",
		"expectation.matches",
		"expectation.not",
		"expectation.present",
		"expectation.null",
		"expectation.not_null",
		"expectation.gt",
		"expectation.gte",
		"expectation.lt",
		"expectation.lte",
		"expectation.between",
		"expectation.has_item",
		"expectation.all_items",
		"expectation.has_key",
		"expectation.lacks_key",
		"expectation.has_entry",
	).map {
		ThtrCapability(ThtrCapabilityKind.MATCHER, it, "Evaluate a Theater expectation.", ThtrCapabilityProvider.BUILTIN)
	}
	val generators = listOf(
		ThtrCapability(
			ThtrCapabilityKind.GENERATOR,
			"date",
			"Generate a UTC date string.",
			ThtrCapabilityProvider.BUILTIN,
			listOf(
				ThtrCapabilityParameter("format", "iso|basic", false),
				ThtrCapabilityParameter("offset", "duration", false),
			),
		),
	) + listOf("sequence", "uuid", "timestamp", "string", "digits", "email", "phone", "slug").map {
		ThtrCapability(ThtrCapabilityKind.GENERATOR, it, "Generate a value for authoring-time data.", ThtrCapabilityProvider.BUILTIN)
	}
	return (builtins + matchers + generators).sortedWith(compareBy({ it.kind.ordinal }, { it.completionLabel }))
}

private fun escapeHtml(value: String): String {
	return value
		.replace("&", "&amp;")
		.replace("<", "&lt;")
		.replace(">", "&gt;")
		.replace("\"", "&quot;")
}
