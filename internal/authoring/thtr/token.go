package thtr

type tokenKind string

const (
	tokenEOF             tokenKind = "eof"
	tokenNewline         tokenKind = "newline"
	tokenIndent          tokenKind = "indent"
	tokenDedent          tokenKind = "dedent"
	tokenComment         tokenKind = "comment"
	tokenIdentifier      tokenKind = "identifier"
	tokenNumber          tokenKind = "number"
	tokenDuration        tokenKind = "duration"
	tokenString          tokenKind = "string"
	tokenRawString       tokenKind = "raw_string"
	tokenMultilineString tokenKind = "multiline_string"
	tokenLParen          tokenKind = "("
	tokenRParen          tokenKind = ")"
	tokenLBrace          tokenKind = "{"
	tokenRBrace          tokenKind = "}"
	tokenLBracket        tokenKind = "["
	tokenRBracket        tokenKind = "]"
	tokenComma           tokenKind = ","
	tokenColon           tokenKind = ":"
	tokenPipe            tokenKind = "|"
	tokenEqual           tokenKind = "="
	tokenGreater         tokenKind = ">"
	tokenLess            tokenKind = "<"
	tokenBang            tokenKind = "!"
	tokenArrow           tokenKind = "->"
	tokenDollar          tokenKind = "$"
	tokenDot             tokenKind = "."
	tokenSlash           tokenKind = "/"
)

type sourcePosition struct {
	Offset int
	Line   int
	Column int
}

type sourceSpan struct {
	Start sourcePosition
	End   sourcePosition
}

type token struct {
	Kind tokenKind
	Text string
	Span sourceSpan
}

type lexerError struct {
	span    sourceSpan
	message string
}

func (e *lexerError) Error() string {
	return e.message
}

func (e *lexerError) Span() sourceSpan {
	return e.span
}
