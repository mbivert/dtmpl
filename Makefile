# Default installation directory.
#	make install dir=$HOME/bin
dir    ?= /bin/
mandir ?= /usr/share/man/man1/
root   ?= root
group  ?= root

.PHONY: all
all: dtmpl tests

.PHONY: help
help:
	@echo Available targets:
	@echo "	dtmpl      : build dtmpl"
	@echo "	all        : build dtmpl; run tests"
	@echo "	clean      : removed compiled files"
	@echo "	tests      : run automated tests"
	@echo "	update-doc : update README.md"
	@echo "	install    : install to ${dir} and ${mandir}"
	@echo "	uninstall  : remove installed files"

dtmpl: dtmpl.go
	@echo Building $@...
	@go build $<

.PHONY: update-doc
update-doc: dtmpl.1
	@echo Updating README.md...
	@(echo '# dtmpl(1) - deep/directory templater';echo; COLUMNS=80 man ./dtmpl.1 | sed 's/^/    /') > README.md

.PHONY: clean
clean:
	@echo Remove compiled binaries...
	@rm -f dtmpl

.PHONY: tests
tests:
	@echo TODO

.PHONY: install
install: dtmpl dtmpl.1
	@echo Installing $< to ${dir}/$<...
	@install -o ${root} -g ${group} -m 755 $< ${dir}/$<
	@echo Installing $<.1 to ${mandir}/$<.1...
	@install -o ${root} -g ${group} -m 644 $<.1 ${mandir}/$<.1

.PHONY: uninstall
uninstall:
	@echo Removing ${dir}/dtmpl...
	@rm -f ${dir}/dtmpl
	@echo Removing ${mandir}/dtmpl.1...
	@rm -f ${mandir}/dtmpl.1
