define tool-from-apt
install-tools: install-tools__$(1)
.PHONY: install-tools__$(1)
install-tools__$(1): $$(BIN)/$(1)
$$(BIN)/$(1): | $$(BIN)
	# update package index
	apt -y update
	# install $(2)
	DEBIAN_FRONTEND=noninteractive apt -y install $(2)
	# maybe symlink $(1)
	SRC="$$$$(which $(1))" DST="$$(BIN)/$(1)" && if [ "$$$${SRC}" != "$$$${DST}" ]; then ln -fs "$$$${SRC}" "$$$${DST}"; fi;
endef

define tool-from-tar-gz
install-tools: install-tools__$(1)
.PHONY: install-tools__$(1)
install-tools__$(1): $$(BIN)/$(1)
$$(BIN)/$(1): $$(BIN)/bsdtar $$(BIN)/curl | $$(BIN)
	# clean temp paths
	rm -rf $$(BIN)/.extract $$(BIN)/.archive.tar.gz && mkdir -p $$(BIN)/.extract
	# download $(1) archive
	curl -o $$(BIN)/.archive.tar.gz -fsSL $(2)
	# extract $(1)
	bsdtar xvzf $$(BIN)/.archive.tar.gz --strip-components $(3) -C $$(BIN)/.extract
	# move $(1)
	mv $$(BIN)/.extract/$(1) $$(BIN)/$(1)
	# clean temp paths
	rm -rf $$(BIN)/.extract $$(BIN)/.archive.tar.gz 
endef

define tool-from-url
install-tools: install-tools__$(1)
.PHONY: install-tools__$(1)
install-tools__$(1): $$(BIN)/$(1)
$$(BIN)/$(1): $$(BIN)/curl | $$(BIN)
	# clean temp paths
	rm -rf $$(BIN)/binary
	# download $(1)
	curl -o $$(BIN)/binary -fsSL $(2)
	# make $(1) executable
	chmod +x $$(BIN)/binary
	# move $(1)
	mv $$(BIN)/binary $$(BIN)/$(1)
endef
