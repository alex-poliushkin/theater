package com.github.alexpoliushkin.theater.thtrij.psi

import com.intellij.psi.tree.TokenSet

object ThtrTokenSets {
	@JvmField
	val COMMENTS: TokenSet = TokenSet.create(ThtrTypes.LINE_COMMENT)

	@JvmField
	val STRINGS: TokenSet = TokenSet.create(ThtrTypes.STRING)
}
