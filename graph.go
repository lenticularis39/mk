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
	"io"
	"os"
	"sync"
	"time"
)

// A dependency graph
type graph struct {
	root  *node            // the intial target's node
	nodes map[string]*node // map targets to their nodes
}

// An edge in the graph.
type edge struct {
	v       *node    // node this edge directs to
	stem    string   // stem matched for meta-rule applications
	matches []string // regular expression matches
	togo    bool     // this edge is going to be pruned
	r       *rule
}

// Current status of a node in the build.
type nodeStatus int

const (
	nodeStatusReady nodeStatus = iota
	nodeStatusStarted
	nodeStatusNop
	nodeStatusDone
	nodeStatusFailed
)

type nodeFlag int

const (
	nodeFlagCycle    nodeFlag = 0x0002
	nodeFlagReady             = 0x0004
	nodeFlagProbable          = 0x0100
	nodeFlagVacuous           = 0x0200
)

// A node in the dependency graph
type node struct {
	r         *rule             // rule to be applied
	name      string            // target name
	prog      string            // custom program to compare times
	t         time.Time         // file modification time
	exists    bool              // does a non-virtual target exist
	prereqs   []*edge           // prerequisite rules
	status    nodeStatus        // current state of the node in the build
	mutex     sync.Mutex        // exclusivity for the status variable
	listeners []chan nodeStatus // channels to notify of completion
	flags     nodeFlag          // bitwise combination of node flags
}

// Update a node's timestamp and 'exists' flag.
func (u *node) updateTimestamp() {
	info, err := os.Stat(u.name)
	if err == nil {
		u.t = info.ModTime()
		u.exists = true
		u.flags |= nodeFlagProbable
	} else {
		_, ok := err.(*os.PathError)
		if ok {
			u.t = time.Unix(0, 0)
			u.exists = false
		} else {
			mkError(err.Error())
		}
	}

	if rebuildAll {
		u.flags |= nodeFlagProbable
	}
}

// Create a new node
func (g *graph) newnode(name string) *node {
	u := &node{name: name}
	u.updateTimestamp()
	g.nodes[name] = u
	return u
}

// Print a graph in graphviz format.
func (g *graph) visualize(w io.Writer) {
	fmt.Fprintln(w, "digraph mk {")
	for t, u := range g.nodes {
		for i := range u.prereqs {
			if u.prereqs[i].v != nil {
				fmt.Fprintf(w, "    \"%s\" -> \"%s\";\n", t, u.prereqs[i].v.name)
			}
		}
	}
	fmt.Fprintln(w, "}")
}

// Create a new arc.
func (u *node) newedge(v *node, r *rule) *edge {
	e := &edge{v: v, r: r}
	u.prereqs = append(u.prereqs, e)
	return e
}

// Create a dependency graph for the given target.
func buildgraph(rs *ruleSet, target string) *graph {
	g := &graph{nil, make(map[string]*node)}

	// keep track of how many times each rule is visited, to avoid cycles.
	rulecnt := make([]int, len(rs.rules))
	g.root = applyrules(rs, g, target, rulecnt)
	g.cyclecheck(g.root)
	g.root.flags |= nodeFlagProbable
	g.vacuous(g.root)
	g.ambiguous(g.root)

	return g
}

// Recursively match the given target to a rule in the rule set to construct the
// full graph.
func applyrules(rs *ruleSet, g *graph, target string, rulecnt []int) *node {
	u, ok := g.nodes[target]
	if ok {
		return u
	}
	u = g.newnode(target)

	// does the target match a concrete rule?

	ks, ok := rs.targetRules[target]
	if ok {
		for ki := range ks {
			k := ks[ki]
			if rulecnt[k] > maxRuleCnt {
				continue
			}

			r := &rs.rules[k]

			// skip meta-rules
			if r.isMeta {
				continue
			}

			// skip rules that have no effect
			if r.recipe == "" && len(r.prereqs) == 0 {
				continue
			}

			u.flags |= nodeFlagProbable
			rulecnt[k] += 1
			if len(r.prereqs) == 0 {
				u.newedge(nil, r)
			} else {
				for i := range r.prereqs {
					u.newedge(applyrules(rs, g, r.prereqs[i], rulecnt), r)
				}
			}
			rulecnt[k] -= 1
		}
	}

	// find applicable metarules
	for k := range rs.rules {
		if rulecnt[k] >= maxRuleCnt {
			continue
		}

		r := &rs.rules[k]

		if !r.isMeta {
			continue
		}

		// skip rules that have no effect
		if r.recipe == "" && len(r.prereqs) == 0 {
			continue
		}

		for j := range r.targets {
			mat := r.targets[j].match(target)
			if mat == nil {
				continue
			}

			var stem string
			var matches []string
			var match_vars = make(map[string][]string)

			if r.attributes.regex {
				matches = mat
				for i := range matches {
					key := fmt.Sprintf("stem%d", i)
					match_vars[key] = matches[i : i+1]
				}
			} else {
				stem = mat[1]
			}

			rulecnt[k] += 1
			if len(r.prereqs) == 0 {
				e := u.newedge(nil, r)
				e.stem = stem
				e.matches = matches
			} else {
				for i := range r.prereqs {
					var prereq string
					if r.attributes.regex {
						prereq = expandRecipeSigils(r.prereqs[i], match_vars)
					} else {
						prereq = expandSuffixes(r.prereqs[i], stem)
					}

					e := u.newedge(applyrules(rs, g, prereq, rulecnt), r)
					e.stem = stem
					e.matches = matches
				}
			}
			rulecnt[k] -= 1
		}
	}

	return u
}

// Remove edges marked as togo.
func (g *graph) togo(u *node) {
	n := 0
	for i := range u.prereqs {
		if !u.prereqs[i].togo {
			n++
		}
	}
	prereqs := make([]*edge, n)
	j := 0
	for i := range u.prereqs {
		if !u.prereqs[i].togo {
			prereqs[j] = u.prereqs[i]
			j++
		}
	}

	// TODO: We may have to delete nodes from g.nodes, right?

	u.prereqs = prereqs
}

// Remove vacous children of n.
func (g *graph) vacuous(u *node) bool {
	vac := u.flags&nodeFlagProbable == 0
	if u.flags&nodeFlagReady != 0 {
		return vac
	}
	u.flags |= nodeFlagReady

	for i := range u.prereqs {
		e := u.prereqs[i]
		if e.v != nil && g.vacuous(e.v) && e.r.isMeta {
			e.togo = true
		} else {
			vac = false
		}
	}

	// if a rule generated edges that are not togo, keep all of its edges
	for i := range u.prereqs {
		e := u.prereqs[i]
		if !e.togo {
			for j := range u.prereqs {
				f := u.prereqs[j]
				if e.r == f.r {
					f.togo = false
				}
			}
		}
	}

	g.togo(u)
	if vac {
		u.flags |= nodeFlagVacuous
	}

	return vac
}

// Check for cycles
func (g *graph) cyclecheck(u *node) {
	if u.flags&nodeFlagCycle != 0 && len(u.prereqs) > 0 {
		mkError(fmt.Sprintf("cycle in the graph detected at target %s", u.name))
	}
	u.flags |= nodeFlagCycle
	for i := range u.prereqs {
		if u.prereqs[i].v != nil {
			g.cyclecheck(u.prereqs[i].v)
		}
	}
	u.flags &= ^nodeFlagCycle

}

// Deal with ambiguous rules.
func (g *graph) ambiguous(u *node) {
	bad := 0
	var le *edge
	for i := range u.prereqs {
		e := u.prereqs[i]

		if e.v != nil {
			g.ambiguous(e.v)
		}
		if e.r.recipe == "" {
			continue
		}
		if le == nil || le.r == nil {
			le = e
		} else {
			if !le.r.equivRecipe(e.r) {
				if le.r.isMeta && !e.r.isMeta {
					mkPrintRecipe(u.name, le.r.recipe, false)
					le.togo = true
					le = e
				} else if !le.r.isMeta && e.r.isMeta {
					mkPrintRecipe(u.name, e.r.recipe, false)
					e.togo = true
					continue
				}
			}
			if !le.r.equivRecipe(e.r) {
				if bad == 0 {
					mkPrintError(fmt.Sprintf("mk: ambiguous recipes for %s\n", u.name))
					bad = 1
					g.trace(u.name, le)
				}
				g.trace(u.name, e)
			}
		}
	}
	if bad > 0 {
		mkError("")
	}
	g.togo(u)
}

// Print a trace of rules, k
func (g *graph) trace(name string, e *edge) {
	fmt.Fprintf(os.Stderr, "\t%s", name)
	for true {
		prereqname := ""
		if e.v != nil {
			prereqname = e.v.name
		}
		fmt.Fprintf(os.Stderr, " <-(%s:%d)- %s", e.r.file, e.r.line, prereqname)
		if e.v != nil {
			for i := range e.v.prereqs {
				if e.v.prereqs[i].r.recipe != "" {
					e = e.v.prereqs[i]
					continue
				}
			}
			break
		} else {
			break
		}
	}
}
