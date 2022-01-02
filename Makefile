GCCGO=gccgo
MK_SRCFILES=lex.go parse.go rules.go expand.go graph.go mk.go recipe.go

mk: $(MK_SRCFILES)
	$(GCCGO) $(LDFLAGS) $(MK_SRCFILES) -o mk

install: mk
	install -c mk $(prefix)/bin/mk

clean:
	rm -f mk
