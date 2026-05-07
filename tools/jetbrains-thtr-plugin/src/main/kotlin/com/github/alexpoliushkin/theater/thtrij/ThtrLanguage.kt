package com.github.alexpoliushkin.theater.thtrij

import com.intellij.lang.Language

class ThtrLanguage private constructor() : Language("thtr") {
	companion object {
		@JvmField
		val INSTANCE: ThtrLanguage = ThtrLanguage()
	}
}
