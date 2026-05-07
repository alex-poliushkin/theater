package thtr

import (
	"fmt"
	"time"
	"unicode/utf8"
)

func lex(data []byte) ([]token, error) {
	scanner := lexer{
		source:      string(data),
		line:        1,
		column:      1,
		indentStack: []int{0},
		atLineStart: true,
	}

	return scanner.scan()
}

type lexer struct {
	source string

	offset int
	line   int
	column int

	bracketDepth int
	atLineStart  bool
	indentStack  []int
	tokens       []token
}

func (l *lexer) scan() ([]token, error) {
	for {
		if l.atLineStart && l.bracketDepth == 0 {
			if err := l.scanIndentation(); err != nil {
				return nil, err
			}
		}

		if l.eof() {
			break
		}

		if l.atLineStart && l.bracketDepth == 0 {
			// comment-only and blank lines keep the line-start state until the
			// newline token is emitted.
			switch l.peek() {
			case '\n', '\r', '#':
				// handled below
			default:
				l.atLineStart = false
			}
		}

		if err := l.scanToken(); err != nil {
			return nil, err
		}
	}

	if l.tokensLen() != 0 && l.tokens[l.tokensLen()-1].Kind != tokenNewline {
		position := l.position()
		l.emitToken(tokenNewline, "", position, position)
	}

	for len(l.indentStack) > 1 {
		position := l.position()
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		l.emitToken(tokenDedent, "", position, position)
	}

	position := l.position()
	l.emitToken(tokenEOF, "", position, position)

	return l.tokens, nil
}

func (l *lexer) scanIndentation() error {
	start := l.position()
	indent := 0
	index := l.offset
	column := l.column

	for index < len(l.source) {
		r, size := utf8.DecodeRuneInString(l.source[index:])
		switch r {
		case ' ':
			indent++
			index += size
			column++
		case '\t':
			return &lexerError{
				span: sourceSpan{
					Start: start,
					End: sourcePosition{
						Offset: index + size,
						Line:   l.line,
						Column: column + 1,
					},
				},
				message: "leading tabs are not allowed in indentation",
			}
		default:
			goto resolved
		}
	}

resolved:
	if index >= len(l.source) {
		l.offset = index
		l.column = column
		return nil
	}

	switch l.source[index] {
	case '#', '\n', '\r':
		l.offset = index
		l.column = column
		return nil
	}

	l.offset = index
	l.column = column

	current := l.indentStack[len(l.indentStack)-1]
	if indent == current {
		return nil
	}
	if indent > current {
		l.indentStack = append(l.indentStack, indent)
		l.emitToken(tokenIndent, "", start, start)
		return nil
	}

	for len(l.indentStack) > 1 && indent < l.indentStack[len(l.indentStack)-1] {
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		l.emitToken(tokenDedent, "", start, start)
	}
	if indent != l.indentStack[len(l.indentStack)-1] {
		return &lexerError{
			span:    sourceSpan{Start: start, End: start},
			message: fmt.Sprintf("inconsistent indentation: got %d spaces", indent),
		}
	}

	return nil
}

func (l *lexer) scanToken() error {
	if l.eof() {
		return nil
	}

	start := l.position()
	r, size := utf8.DecodeRuneInString(l.source[l.offset:])

	if l.scanLayoutToken(start, r, size) {
		return nil
	}
	if handled, err := l.scanStructuralToken(start, r, size); handled || err != nil {
		return err
	}
	if handled, err := l.scanStringLikeToken(r); handled || err != nil {
		return err
	}

	if isIdentifierStart(r) {
		return l.scanIdentifier()
	}
	if isDigit(r) || l.hasNumericStart() {
		return l.scanNumberOrDuration()
	}

	return &lexerError{
		span: sourceSpan{
			Start: start,
			End:   l.positionAfter(size, r),
		},
		message: fmt.Sprintf("unexpected character %q", r),
	}
}

func (l *lexer) scanLayoutToken(start sourcePosition, r rune, size int) bool {
	switch r {
	case ' ', '\t':
		l.advanceRune(r, size)
		return true
	case '\n':
		l.advanceRune(r, size)
		if l.bracketDepth == 0 {
			l.emitToken(tokenNewline, "", start, l.position())
			l.atLineStart = true
		}
		return true
	case '\r':
		l.advanceRune(r, size)
		if !l.eof() && l.peek() == '\n' {
			next, nextSize := utf8.DecodeRuneInString(l.source[l.offset:])
			l.advanceRune(next, nextSize)
		}
		if l.bracketDepth == 0 {
			l.emitToken(tokenNewline, "", start, l.position())
			l.atLineStart = true
		}
		return true
	case '#':
		l.scanComment()
		return true
	default:
		return false
	}
}

func (l *lexer) scanStructuralToken(start sourcePosition, r rune, size int) (bool, error) {
	switch r {
	case '(':
		return true, l.emitBracketToken(tokenLParen, "(", 1, start, size)
	case ')':
		return true, l.emitClosingBracketToken(tokenRParen, ")", start, size)
	case '{':
		return true, l.emitBracketToken(tokenLBrace, "{", 1, start, size)
	case '}':
		return true, l.emitClosingBracketToken(tokenRBrace, "}", start, size)
	case '[':
		return true, l.emitBracketToken(tokenLBracket, "[", 1, start, size)
	case ']':
		return true, l.emitClosingBracketToken(tokenRBracket, "]", start, size)
	case ',':
		return true, l.emitSimpleToken(tokenComma, ",", start, size)
	case ':':
		return true, l.emitSimpleToken(tokenColon, ":", start, size)
	case '|':
		return true, l.emitSimpleToken(tokenPipe, "|", start, size)
	case '=':
		return true, l.emitSimpleToken(tokenEqual, "=", start, size)
	case '>':
		return true, l.emitSimpleToken(tokenGreater, ">", start, size)
	case '<':
		return true, l.emitSimpleToken(tokenLess, "<", start, size)
	case '!':
		return true, l.emitSimpleToken(tokenBang, "!", start, size)
	case '$':
		return true, l.emitSimpleToken(tokenDollar, "$", start, size)
	case '.':
		return true, l.emitSimpleToken(tokenDot, ".", start, size)
	case '/':
		return true, l.emitSimpleToken(tokenSlash, "/", start, size)
	case '-':
		if l.hasPrefix("->") {
			l.advanceBytes(2)
			l.emitToken(tokenArrow, "->", start, l.position())
			return true, nil
		}
		return false, nil
	default:
		return false, nil
	}
}

func (l *lexer) scanStringLikeToken(r rune) (bool, error) {
	switch r {
	case '"':
		if l.hasPrefix(`"""`) {
			return true, l.scanMultilineString()
		}
		return true, l.scanString()
	case 'r':
		if l.hasPrefix(`r"`) {
			return true, l.scanRawString()
		}
		return false, nil
	default:
		return false, nil
	}
}

func (l *lexer) scanComment() {
	startOffset := l.offset
	start := l.position()
	for !l.eof() {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if r == '\n' || r == '\r' {
			break
		}
		l.advanceRune(r, size)
	}

	l.emitToken(tokenComment, l.source[startOffset:l.offset], start, l.position())
}

func (l *lexer) scanIdentifier() error {
	startOffset := l.offset
	start := l.position()
	for !l.eof() {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if !isIdentifierPart(r) {
			break
		}
		l.advanceRune(r, size)
	}

	l.emitToken(tokenIdentifier, l.source[startOffset:l.offset], start, l.position())
	return nil
}

func (l *lexer) scanNumberOrDuration() error {
	startOffset := l.offset
	start := l.position()

	if l.peek() == '-' {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		l.advanceRune(r, size)
	}

	l.consumeDigits()
	if l.peek() == '.' {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		l.advanceRune(r, size)
		l.consumeDigits()
	}

	kind := tokenNumber
	for !l.eof() {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if isDurationRune(r) {
			kind = tokenDuration
			l.advanceRune(r, size)
			continue
		}
		break
	}

	text := l.source[startOffset:l.offset]
	if kind == tokenDuration {
		if _, err := time.ParseDuration(text); err != nil {
			return &lexerError{
				span:    sourceSpan{Start: start, End: l.position()},
				message: fmt.Sprintf("invalid duration literal %q", text),
			}
		}
	}

	l.emitToken(kind, text, start, l.position())
	return nil
}

func (l *lexer) scanString() error {
	return l.scanQuotedString(tokenString, `"`, false)
}

func (l *lexer) scanRawString() error {
	start := l.position()
	startOffset := l.offset
	l.advanceBytes(2) // r"
	return l.scanQuotedStringFrom(start, startOffset, tokenRawString, `"`, true)
}

func (l *lexer) scanMultilineString() error {
	start := l.position()
	startOffset := l.offset
	l.advanceBytes(3)
	return l.scanQuotedStringFrom(start, startOffset, tokenMultilineString, `"""`, true)
}

func (l *lexer) scanQuotedString(kind tokenKind, quote string, raw bool) error {
	start := l.position()
	startOffset := l.offset
	l.advanceBytes(len(quote))
	return l.scanQuotedStringFrom(start, startOffset, kind, quote, raw)
}

func (l *lexer) scanQuotedStringFrom(start sourcePosition, startOffset int, kind tokenKind, quote string, raw bool) error {
	for !l.eof() {
		if l.hasPrefix(quote) {
			l.advanceBytes(len(quote))
			l.emitToken(kind, l.source[startOffset:l.offset], start, l.position())
			return nil
		}

		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if kind != tokenMultilineString && (r == '\n' || r == '\r') {
			return &lexerError{
				span:    sourceSpan{Start: start, End: l.position()},
				message: "unterminated string literal",
			}
		}

		if !raw && r == '\\' {
			l.advanceRune(r, size)
			if l.eof() {
				break
			}
			next, nextSize := utf8.DecodeRuneInString(l.source[l.offset:])
			l.advanceRune(next, nextSize)
			continue
		}

		l.advanceRune(r, size)
	}

	return &lexerError{
		span:    sourceSpan{Start: start, End: l.position()},
		message: "unterminated string literal",
	}
}

func (l *lexer) consumeDigits() {
	for !l.eof() {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if !isDigit(r) {
			return
		}
		l.advanceRune(r, size)
	}
}

func (l *lexer) emitToken(kind tokenKind, text string, start, end sourcePosition) {
	l.tokens = append(l.tokens, token{
		Kind: kind,
		Text: text,
		Span: sourceSpan{Start: start, End: end},
	})
}

func (l *lexer) emitSimpleToken(kind tokenKind, text string, start sourcePosition, size int) error {
	r, width := utf8.DecodeRuneInString(l.source[l.offset:])
	if size != width {
		size = width
	}
	l.advanceRune(r, size)
	l.emitToken(kind, text, start, l.position())
	return nil
}

func (l *lexer) emitBracketToken(kind tokenKind, text string, depthDelta int, start sourcePosition, size int) error {
	l.bracketDepth += depthDelta
	return l.emitSimpleToken(kind, text, start, size)
}

func (l *lexer) emitClosingBracketToken(kind tokenKind, text string, start sourcePosition, size int) error {
	if l.bracketDepth == 0 {
		return &lexerError{
			span:    sourceSpan{Start: start, End: l.positionAfter(size, rune(text[0]))},
			message: fmt.Sprintf("unexpected closing bracket %q", text),
		}
	}
	l.bracketDepth--
	return l.emitSimpleToken(kind, text, start, size)
}

func (l *lexer) tokensLen() int {
	return len(l.tokens)
}

func (l *lexer) eof() bool {
	return l.offset >= len(l.source)
}

func (l *lexer) peek() byte {
	return l.source[l.offset]
}

func (l *lexer) hasPrefix(prefix string) bool {
	return len(l.source[l.offset:]) >= len(prefix) && l.source[l.offset:l.offset+len(prefix)] == prefix
}

func (l *lexer) hasNumericStart() bool {
	if l.eof() || l.source[l.offset] != '-' {
		return false
	}
	if l.offset+1 >= len(l.source) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(l.source[l.offset+1:])
	return isDigit(r)
}

func (l *lexer) advanceBytes(count int) {
	for range count {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		l.advanceRune(r, size)
	}
}

func (l *lexer) advanceRune(r rune, size int) {
	l.offset += size
	switch r {
	case '\n':
		l.line++
		l.column = 1
	default:
		l.column++
	}
}

func (l *lexer) position() sourcePosition {
	return sourcePosition{
		Offset: l.offset,
		Line:   l.line,
		Column: l.column,
	}
}

func (l *lexer) positionAfter(size int, r rune) sourcePosition {
	position := l.position()
	position.Offset += size
	if r == '\n' {
		position.Line++
		position.Column = 1
		return position
	}
	position.Column++
	return position
}

func isIdentifierStart(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || r >= '0' && r <= '9' || r == '_' || r == '-'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isDurationRune(r rune) bool {
	return isDigit(r) || isIdentifierStart(r) || r == 'µ'
}
