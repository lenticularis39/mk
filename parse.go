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

// This is a mkfile parser. It executes assignments and includes as it goes, and
// collects a set of rules, which are returned as a ruleSet object.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type parser struct {
	l        *lexer   // underlying lexer
	name     string   // name of the file being parsed
	path     string   // full path of the file being parsed
	tokenBuf []token  // tokens consumed on the current statement
	rules    *ruleSet // current ruleSet
}

// Pretty errors.
func (p *parser) parseError(context string, expected string, found token) {
	mkPrintError(fmt.Sprintf("%s:%d: syntax error: ", p.name, found.line))
	mkPrintError(fmt.Sprintf("while %s, expected %s but found '%s'.\n",
		context, expected, found.String()))
	mkError("")
}

// More basic errors.
func (p *parser) basicErrorAtToken(what string, found token) {
	p.basicErrorAtLine(what, found.line)
}

func (p *parser) basicErrorAtLine(what string, line int) {
	mkError(fmt.Sprintf("%s:%d: syntax error: %s\n", p.name, line, what))
}

// Accept a token for use in the current statement being parsed.
func (p *parser) push(t token) {
	p.tokenBuf = append(p.tokenBuf, t)
}

// Clear all the accepted tokens. Called when a statement is finished.
func (p *parser) clear() {
	p.tokenBuf = p.tokenBuf[:0]
}

// A parser state function takes a parser and the next token and returns a new
// state function, or nil if there was a parse error.
type parserStateFun func(*parser, token) parserStateFun

// Parse a mkfile, returning a new ruleSet.
func parse(input string, name string, path string, env map[string][]string) *ruleSet {
	rules := &ruleSet{env,
		make([]rule, 0),
		make(map[string][]int)}
	parseInto(input, name, rules, path)
	return rules
}

// Parse a mkfile inserting rules and variables into a given ruleSet.
func parseInto(input string, name string, rules *ruleSet, path string) {
	l, tokens := lex(input)
	p := &parser{l, name, path, []token{}, rules}
	oldmkfiledir := p.rules.vars["mkfiledir"]
	p.rules.vars["mkfiledir"] = []string{filepath.Dir(path)}
	state := parseTopLevel
	for t := range tokens {
		if t.typ == tokenError {
			p.basicErrorAtLine(l.errMsg, t.line)
			break
		}

		state = state(p, t)
	}

	// insert a dummy newline to allow parsing of any assignments or recipeless
	// rules to finish.
	state = state(p, token{tokenNewline, "\n", l.line, l.col})

	p.rules.vars["mkfiledir"] = oldmkfiledir

	// TODO: Error when state != parseTopLevel
}

// We are at the top level of a mkfile, expecting rules, assignments, or
// includes.
func parseTopLevel(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		return parseTopLevel
	case tokenPipeInclude:
		return parsePipeInclude
	case tokenRedirInclude:
		return parseRedirInclude
	case tokenWord:
		return parseAssignmentOrTarget(p, t)
	default:
		p.parseError("parsing mkfile",
			"a rule, include, or assignment", t)
	}

	return parseTopLevel
}

// Consumed a '<|'
func parsePipeInclude(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		if len(p.tokenBuf) == 0 {
			p.basicErrorAtToken("empty pipe include", t)
		}

		args := make([]string, len(p.tokenBuf)+1)
		args[0] = "-c"
		for i := 0; i < len(p.tokenBuf); i++ {
			s := p.tokenBuf[i].val
			expanded := expand(s, p.rules.vars, false)
			if len(expanded) > 0 {
				s = expanded[0]
			}
			args[i+1] = s
		}

		output, success := subprocess("sh", args, "", true)
		if !success {
			p.basicErrorAtToken("subprocess include failed", t)
		}

		parseInto(output, fmt.Sprintf("%s:sh", p.name), p.rules, p.path)

		p.clear()
		return parseTopLevel

	// Almost anything goes. Let the shell sort it out.
	case tokenPipeInclude:
		fallthrough
	case tokenRedirInclude:
		fallthrough
	case tokenColon:
		fallthrough
	case tokenAssign:
		fallthrough
	case tokenWord:
		p.tokenBuf = append(p.tokenBuf, t)

	default:
		p.parseError("parsing piped include", "a shell command", t)
	}

	return parsePipeInclude
}

// Consumed a '<'
func parseRedirInclude(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		filename := ""
		for i := range p.tokenBuf {
			filename += p.tokenBuf[i].val
		}
		expanded := expand(filename, p.rules.vars, false)
		if len(expanded) > 0 {
			filename = expanded[0]
		}
		fmt.Printf("parsed filename: %v\nexpanded filename: %v\n", filename, expanded)
		file, err := os.Open(filename)
		if err != nil {
			p.basicErrorAtToken(fmt.Sprintf("cannot open %s", filename), p.tokenBuf[0])
		}
		input, _ := ioutil.ReadAll(file)

		path, err := filepath.Abs(filename)
		if err != nil {
			mkError("unable to find mkfile's absolute path")
		}

		parseInto(string(input), filename, p.rules, path)

		p.clear()
		return parseTopLevel

	case tokenWord:
		p.tokenBuf = append(p.tokenBuf, t)

	default:
		p.parseError("parsing include", "a file name", t)
	}

	return parseRedirInclude
}

// Encountered a bare string at the beginning of the line.
func parseAssignmentOrTarget(p *parser, t token) parserStateFun {
	p.push(t)
	return parseEqualsOrTarget
}

// Consumed one bare string ot the beginning of the line.
func parseEqualsOrTarget(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenAssign:
		return parseAssignment

	case tokenWord:
		p.push(t)
		return parseTargets

	case tokenColon:
		p.push(t)
		return parseAttributesOrPrereqs

	default:
		p.parseError("reading a target or assignment",
			"'=', ':', or another target", t)
	}

	return parseTopLevel // unreachable
}

// Consumed 'foo='. Everything else is a value being assigned to foo.
func parseAssignment(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		err := p.rules.executeAssignment(p.tokenBuf)
		if err != nil {
			p.basicErrorAtToken(err.what, err.where)
		}
		p.clear()
		return parseTopLevel

	default:
		p.push(t)
	}

	return parseAssignment
}

// Everything up to ':' must be a target.
func parseTargets(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenWord:
		p.push(t)
	case tokenColon:
		p.push(t)
		return parseAttributesOrPrereqs

	default:
		p.parseError("reading a rule's targets",
			"filename or pattern", t)
	}

	return parseTargets
}

// Consume one or more strings followed by a first ':'.
func parseAttributesOrPrereqs(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		return parseRecipe
	case tokenColon:
		p.push(t)
		return parsePrereqs
	case tokenWord:
		p.push(t)
	default:
		p.parseError("reading a rule's attributes or prerequisites",
			"an attribute, pattern, or filename", t)
	}

	return parseAttributesOrPrereqs
}

// Targets and attributes and the second ':' have been consumed.
func parsePrereqs(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		return parseRecipe
	case tokenWord:
		p.push(t)

	default:
		p.parseError("reading a rule's prerequisites",
			"filename or pattern", t)
	}

	return parsePrereqs
}

// An entire rule has been consumed.
func parseRecipe(p *parser, t token) parserStateFun {
	// Assemble the rule!
	r := rule{}

	// find one or two colons
	i := 0
	for ; i < len(p.tokenBuf) && p.tokenBuf[i].typ != tokenColon; i++ {
	}
	j := i + 1
	for ; j < len(p.tokenBuf) && p.tokenBuf[j].typ != tokenColon; j++ {
	}

	// rule has attributes
	if j < len(p.tokenBuf) {
		attribs := make([]string, 0)
		for k := i + 1; k < j; k++ {
			exparts := expand(p.tokenBuf[k].val, p.rules.vars, true)
			attribs = append(attribs, exparts...)
		}
		err := r.parseAttribs(attribs)
		if err != nil {
			msg := fmt.Sprintf("while reading a rule's attributes expected an attribute but found \"%c\".", err.found)
			p.basicErrorAtToken(msg, p.tokenBuf[i+1])
		}

		if r.attributes.regex {
			r.isMeta = true
		}
	} else {
		j = i
	}

	// targets
	r.targets = make([]pattern, 0)
	for k := 0; k < i; k++ {
		exparts := expand(p.tokenBuf[k].val, p.rules.vars, true)
		for i := range exparts {
			targetstr := exparts[i]
			r.targets = append(r.targets, pattern{spat: targetstr})

			if r.attributes.regex {
				rpat, err := regexp.Compile("^" + targetstr + "$")
				if err != nil {
					msg := fmt.Sprintf("invalid regular expression: %q", err)
					p.basicErrorAtToken(msg, p.tokenBuf[k])
				}
				r.targets[len(r.targets)-1].rpat = rpat
			} else {
				idx := strings.IndexRune(targetstr, '%')
				if idx >= 0 {
					var left, right string
					if idx > 0 {
						left = regexp.QuoteMeta(targetstr[:idx])
					}
					if idx < len(targetstr)-1 {
						right = regexp.QuoteMeta(targetstr[idx+1:])
					}

					patstr := fmt.Sprintf("^%s(.*)%s$", left, right)
					rpat, err := regexp.Compile(patstr)
					if err != nil {
						msg := fmt.Sprintf("error compiling suffix rule. This is a bug. Error: %s", err)
						p.basicErrorAtToken(msg, p.tokenBuf[k])
					}
					r.targets[len(r.targets)-1].rpat = rpat
					r.targets[len(r.targets)-1].isSuffix = true
					r.isMeta = true
				}
			}
		}
	}

	// prereqs
	r.prereqs = make([]string, 0)
	for k := j + 1; k < len(p.tokenBuf); k++ {
		exparts := expand(p.tokenBuf[k].val, p.rules.vars, true)
		r.prereqs = append(r.prereqs, exparts...)
	}

	if t.typ == tokenRecipe {
		r.recipe = expandRecipeSigils(stripIndentation(t.val, t.col), p.rules.vars)
	}

	p.rules.add(r)
	p.clear()

	// the current token doesn't belong to this rule
	if t.typ != tokenRecipe {
		return parseTopLevel(p, t)
	}

	return parseTopLevel
}
