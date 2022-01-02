Mk is a reboot of the Plan 9 mk command, which itself is [a successor to
make](http://www.cs.tufts.edu/~nr/cs257/archive/andrew-hume/mk.pdf). This tool
is for anyone who loves make, but hates all its stupid bullshit.

# Installation

 1. Install Go.
 2. Run `go get github.com/dcjones/mk`
 3. Make sure `$GOPATH/bin` is in your `PATH`.

# Improvements over Plan 9 mk

This mk stays mostly faithful to Plan 9, but makes a few (in my opinion)
improvements.

  1. A clean, modern implementation in Go, that doesn't depend on the whole Plan
     9 stack.
  1. Parallel by default. Modern computers can build more than one C file at a
     time. Cases that should not be run in parallel are the exception. Use
     `-p=1` if this is the case.
  1. Use Go regular expressions, which are perl-like. The original mk used plan9
     regex, which few people know or care to learn.
  1. Regex matches are substituted into rule prerequisites with `$stem1`,
     `$stem2`, etc, rather than `\1`, `\2`, etc.
  1. Allow blank lines in recipes. A recipe is any indented block of text, and
     continues until a non-indented character or the end of the file. (Similar
     to blocks in Python.)
  1. Add an 'S' attribute to execute recipes with programs other than sh. This
     way, you don't have to separate your six line python script into its own
     file. Just stick it directly in the mkfile.

# Usage

`mk [options] [target] ...`

## Options

  * `-f filename` Use the given file as the mkfile.
  * `-n` Dry run, print commands without actually executing.
  * `-r` Force building of the immediate targets.
  * `-a` Force building the targets and of all their dependencies.
  * `-p` Maximum number of jobs to execute in parallel (default: 8)
  * `-i` Show rules that will execute and prompt before executing.


# Non-shell recipes

Non-shell recipes are a major addition over Plan 9 mk. They can be used with the
`S[command]` attribute, where `command` is an arbitrary command that the recipe
will be piped into. For example, here's a recipe to add the read numbers from a
file and write their mean to another file. Unlike a typical recipe, it's written
in Julia.

```make
mean.txt:Sjulia: input.txt
    println(open("$target", "w"),
            mean(map(parseint, eachline(open("$prereq")))))
```

# Current State

Functional, but with some bugs and some unimplemented minor features.
