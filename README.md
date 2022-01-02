Mk is a reboot of the Plan 9 mk command, which itself is [a successor to
make](http://www.cs.tufts.edu/~nr/cs257/archive/andrew-hume/mk.pdf). It is
a fork of [mk by Daniel C. Jones](https://github.com/dcjones/mk).

# Installation

If you have gccgo, you can build and install mk with make:

```
$ make
$ make install
```

You can also use `go` to install mk with any Go implementation:

```
$ go get github.com/lenticularis39/mk
```

# Changes from Plan 9 mk

mk stays mostly faithful to Plan 9, but makes a few changes.

  1. A clean, modern implementation in Go.
  1. Parallel by default.
  1. Use Go regular expressions, which are perl-like, instead of Plan 9 regex.
  1. Regex matches are substituted into rule prerequisites with `$stem1`,
     `$stem2`, etc., rather than `\1`, `\2`, etc.
  1. Allow blank lines in recipes. A recipe is any indented block of text, and
     continues until a non-indented character or the end of the file.
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
