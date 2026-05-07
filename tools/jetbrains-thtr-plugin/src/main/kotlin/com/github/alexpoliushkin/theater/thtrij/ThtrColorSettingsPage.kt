package com.github.alexpoliushkin.theater.thtrij

import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.fileTypes.SyntaxHighlighter
import com.intellij.openapi.options.colors.AttributesDescriptor
import com.intellij.openapi.options.colors.ColorDescriptor
import com.intellij.openapi.options.colors.ColorSettingsPage
import javax.swing.Icon

class ThtrColorSettingsPage : ColorSettingsPage {
	override fun getIcon(): Icon? = ThtrFileType.INSTANCE.icon

	override fun getHighlighter(): SyntaxHighlighter = ThtrSyntaxHighlighter()

	override fun getDemoText(): String {
		return """
			# Theater DSL
			stage <decl>checkout</decl>
			scenario <decl>auth/register</decl>(email: string!)
			  act <decl>submit</decl>
			    eventually 30s every 1s
			    do <cap>action.http</cap>
			      <key>method</key>: "POST"
			    log <decl>response</decl> = object { <key>status</key>: field(status_code), <key>id</key>: <ref>${'$'}id</ref> }
			    expect ok: field(body) | decode(json) | path("/id") == <ref>${'$'}id</ref>
			    export <decl>id</decl> = <cap>generate.email</cap>()
			call <decl>run</decl> = auth/register(email: "demo@example.test")
		""".trimIndent()
	}

	override fun getAdditionalHighlightingTagToDescriptorMap(): Map<String, TextAttributesKey> {
		return mapOf(
			"decl" to ThtrHighlighting.DECLARATION_ID,
			"key" to ThtrHighlighting.DATA_KEY,
			"ref" to ThtrHighlighting.REFERENCE,
			"cap" to ThtrHighlighting.CAPABILITY_REF,
		)
	}

	override fun getAttributeDescriptors(): Array<AttributesDescriptor> = ATTRIBUTE_DESCRIPTORS

	override fun getColorDescriptors(): Array<ColorDescriptor> = ColorDescriptor.EMPTY_ARRAY

	override fun getDisplayName(): String = "Theater"
}

private val ATTRIBUTE_DESCRIPTORS = arrayOf(
	AttributesDescriptor("Keyword", ThtrHighlighting.KEYWORD),
	AttributesDescriptor("Identifier", ThtrHighlighting.IDENTIFIER),
	AttributesDescriptor("Declaration", ThtrHighlighting.DECLARATION_ID),
	AttributesDescriptor("Data key", ThtrHighlighting.DATA_KEY),
	AttributesDescriptor("Reference", ThtrHighlighting.REFERENCE),
	AttributesDescriptor("Capability reference", ThtrHighlighting.CAPABILITY_REF),
	AttributesDescriptor("Selector", ThtrHighlighting.SELECTOR),
	AttributesDescriptor("Generator reference", ThtrHighlighting.GENERATOR_REF),
	AttributesDescriptor("String", ThtrHighlighting.STRING),
	AttributesDescriptor("Number", ThtrHighlighting.NUMBER),
	AttributesDescriptor("Duration", ThtrHighlighting.DURATION),
	AttributesDescriptor("Comment", ThtrHighlighting.COMMENT),
	AttributesDescriptor("Operator", ThtrHighlighting.OPERATOR),
	AttributesDescriptor("Bad character", ThtrHighlighting.BAD_CHARACTER),
)
