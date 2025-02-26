/*
	Copyright (c) 2022 Tomas Glozar

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.

	Copyright (c) 2013, Daniel C. Jones <dcjones@cs.washington.edu>
	All rights reserved.

	Redistribution and use in source and binary forms, with or without
	modification, are permitted provided that the following conditions are met:

	1. Redistributions of source code must retain the above copyright notice, this
	   list of conditions and the following disclaimer.
	2. Redistributions in binary form must reproduce the above copyright notice,
	   this list of conditions and the following disclaimer in the documentation
	   and/or other materials provided with the distribution.

	THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
	ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
	WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
	DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
	ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
	(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
	LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
	ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
	(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
	SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

	The views and conclusions contained in the software and documentation are those
	of the authors and should not be interpreted as representing official policies,
	either expressed or implied, of the FreeBSD Project.
*/

package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type tokenType int

const eof rune = '\000'

// Rune's that cannot be part of a bare (unquoted) string.
const nonBareRunes = " \t\n\r\\=:#'\"$"

// Return true if the string contains whitespace only.
func onlyWhitespace(s string) bool {
	return strings.IndexAny(s, " \t\r\n") < 0
}

const (
	tokenError tokenType = iota
	tokenNewline
	tokenWord
	tokenPipeInclude
	tokenRedirInclude
	tokenColon
	tokenAssign
	tokenRecipe
)

func (typ tokenType) String() string {
	switch typ {
	case tokenError:
		return "[Error]"
	case tokenNewline:
		return "[Newline]"
	case tokenWord:
		return "[Word]"
	case tokenPipeInclude:
		return "[PipeInclude]"
	case tokenRedirInclude:
		return "[RedirInclude]"
	case tokenColon:
		return "[Colon]"
	case tokenAssign:
		return "[Assign]"
	case tokenRecipe:
		return "[Recipe]"
	}
	return "[MysteryToken]"
}

type token struct {
	typ  tokenType // token type
	val  string    // token string
	line int       // line where it was found
	col  int       // column on which the token began
}

func (t *token) String() string {
	if t.typ == tokenError {
		return t.val
	} else if t.typ == tokenNewline {
		return "\\n"
	}

	return t.val
}

type lexer struct {
	input     string     // input string to be lexed
	output    chan token // channel on which tokens are sent
	start     int        // token beginning
	startCol  int        // column on which the token begins
	pos       int        // position within input
	line      int        // line within input
	col       int        // column within input
	errMsg    string     // set to an appropriate error message when necessary
	indented  bool       // true if the only whitespace so far on this line
	bareWords bool       // lex only a sequence of words
}

// A lexerStateFun is simultaneously the state of the lexer and the next
// action the lexer will perform.
type lexerStateFun func(*lexer) lexerStateFun

func (l *lexer) lexError(what string) {
	if l.errMsg == "" {
		l.errMsg = what
	}
	l.emit(tokenError)
}

// Return the nth character without advancing.
func (l *lexer) peekN(n int) (c rune) {
	pos := l.pos
	var width int
	i := 0
	for ; i <= n && pos < len(l.input); i++ {
		c, width = utf8.DecodeRuneInString(l.input[pos:])
		pos += width
	}

	if i <= n {
		return eof
	}

	return
}

// Return the next character without advancing.
func (l *lexer) peek() rune {
	return l.peekN(0)
}

// Consume and return the next character in the lexer input.
func (l *lexer) next() rune {
	if l.pos >= len(l.input) {
		return eof
	}
	c, width := utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += width

	if c == '\n' {
		l.col = 0
		l.line += 1
		l.indented = true
	} else {
		l.col += 1
		if strings.IndexRune(" \t", c) < 0 {
			l.indented = false
		}
	}

	return c
}

// Skip and return the next character in the lexer input.
func (l *lexer) skip() {
	l.next()
	l.start = l.pos
	l.startCol = l.col
}

func (l *lexer) emit(typ tokenType) {
	l.output <- token{typ, l.input[l.start:l.pos], l.line, l.startCol}
	l.start = l.pos
	l.startCol = 0
}

// Consume the next run if it is in the given string.
func (l *lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.peek()) >= 0 {
		l.next()
		return true
	}
	return false
}

// Skip the next rune if it is in the valid string. Return true if it was
// skipped.
func (l *lexer) ignore(valid string) bool {
	if strings.IndexRune(valid, l.peek()) >= 0 {
		l.skip()
		return true
	}
	return false
}

// Consume characters from the valid string until the next is not.
func (l *lexer) acceptRun(valid string) int {
	prevpos := l.pos
	for strings.IndexRune(valid, l.peek()) >= 0 {
		l.next()
	}
	return l.pos - prevpos
}

// Accept until something from the given string is encountered.
func (l *lexer) acceptUntil(invalid string) {
	for l.pos < len(l.input) && strings.IndexRune(invalid, l.peek()) < 0 {
		l.next()
	}

	if l.peek() == eof {
		l.lexError(fmt.Sprintf("end of file encountered while looking for one of: %s", invalid))
	}
}

// Accept until something from the given string is encountered, or the end of th
// file
func (l *lexer) acceptUntilOrEof(invalid string) {
	for l.pos < len(l.input) && strings.IndexRune(invalid, l.peek()) < 0 {
		l.next()
	}
}

// Skip characters from the valid string until the next is not.
func (l *lexer) skipRun(valid string) int {
	prevpos := l.pos
	for strings.IndexRune(valid, l.peek()) >= 0 {
		l.skip()
	}
	return l.pos - prevpos
}

// Skip until something from the given string is encountered.
func (l *lexer) skipUntil(invalid string) {
	for l.pos < len(l.input) && strings.IndexRune(invalid, l.peek()) < 0 {
		l.skip()
	}

	if l.peek() == eof {
		l.lexError(fmt.Sprintf("end of file encountered while looking for one of: %s", invalid))
	}
}

// Start a new lexer to lex the given input.
func lex(input string) (*lexer, chan token) {
	l := &lexer{input: input, output: make(chan token), line: 1, col: 0, indented: true}
	go l.run()
	return l, l.output
}

func lexWords(input string) (*lexer, chan token) {
	l := &lexer{input: input, output: make(chan token), line: 1, col: 0, indented: true, bareWords: true}
	go l.run()
	return l, l.output
}

func (l *lexer) run() {
	for state := lexTopLevel; state != nil; {
		state = state(l)
	}
	close(l.output)
}

func lexTopLevel(l *lexer) lexerStateFun {
	for {
		l.skipRun(" \t\r")
		// emit a newline token if we are ending a non-empty line.
		if l.peek() == '\n' && !l.indented {
			l.next()
			if l.bareWords {
				return nil
			} else {
				l.emit(tokenNewline)
			}
		}
		l.skipRun(" \t\r\n")

		if l.peek() == '\\' && l.peekN(1) == '\n' {
			l.next()
			l.next()
			l.indented = false
		} else {
			break
		}
	}

	if l.indented && l.col > 0 {
		return lexRecipe
	}

	c := l.peek()
	switch c {
	case eof:
		return nil
	case '#':
		return lexComment
	case '<':
		return lexInclude
	case ':':
		return lexColon
	case '=':
		return lexAssign
	case '"':
		return lexDoubleQuotedWord
	case '\'':
		return lexSingleQuotedWord
	case '`':
		return lexBackQuotedWord
	}

	return lexBareWord
}

func lexColon(l *lexer) lexerStateFun {
	l.next()
	l.emit(tokenColon)
	return lexTopLevel
}

func lexAssign(l *lexer) lexerStateFun {
	l.next()
	l.emit(tokenAssign)
	return lexTopLevel
}

func lexComment(l *lexer) lexerStateFun {
	l.skip() // '#'
	l.skipUntil("\n")
	return lexTopLevel
}

func lexInclude(l *lexer) lexerStateFun {
	l.next() // '<'
	if l.accept("|") {
		l.emit(tokenPipeInclude)
	} else {
		l.emit(tokenRedirInclude)
	}
	return lexTopLevel
}

func lexDoubleQuotedWord(l *lexer) lexerStateFun {
	l.next() // '"'
	for l.peek() != '"' && l.peek() != eof {
		l.acceptUntil("\\\"")
		if l.accept("\\") {
			l.accept("\"")
		}
	}

	if l.peek() == eof {
		l.lexError("end of file encountered while parsing a quoted string.")
	}

	l.next() // '"'
	return lexBareWord
}

func lexBackQuotedWord(l *lexer) lexerStateFun {
	l.next() // '`'
	l.acceptUntil("`")
	l.next() // '`'
	return lexBareWord
}

func lexSingleQuotedWord(l *lexer) lexerStateFun {
	l.next() // '\''
	l.acceptUntil("'")
	l.next() // '\''
	return lexBareWord
}

func lexRecipe(l *lexer) lexerStateFun {
	for {
		l.acceptUntilOrEof("\n")
		l.acceptRun(" \t\n\r")
		if !l.indented || l.col == 0 {
			break
		}
	}

	if !onlyWhitespace(l.input[l.start:l.pos]) {
		l.emit(tokenRecipe)
	}
	return lexTopLevel
}

func lexBareWord(l *lexer) lexerStateFun {
	l.acceptUntil(nonBareRunes)
	c := l.peek()
	if c == '"' {
		return lexDoubleQuotedWord
	} else if c == '\'' {
		return lexSingleQuotedWord
	} else if c == '`' {
		return lexBackQuotedWord
	} else if c == '\\' {
		c1 := l.peekN(1)
		if c1 == '\n' || c1 == '\r' {
			if l.start < l.pos {
				l.emit(tokenWord)
			}
			l.skip()
			l.skip()
			return lexTopLevel
		} else {
			l.next()
			l.next()
			return lexBareWord
		}
	} else if c == '$' {
		c1 := l.peekN(1)
		if c1 == '{' {
			return lexBracketExpansion
		} else {
			l.next()
			return lexBareWord
		}
	}

	if l.start < l.pos {
		l.emit(tokenWord)
	}

	return lexTopLevel
}

func lexBracketExpansion(l *lexer) lexerStateFun {
	l.next() // '$'
	l.next() // '{'
	l.acceptUntil("}")
	l.next() // '}'
	return lexBareWord
}
