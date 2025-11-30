GORELEASER_VERSION := 2.12.7
SVU_VERSION := 3.3.0

include Makefile.include.mk

arch := $(shell uname -m)
ifeq ($(arch),aarch64)
    arch := arm64
else
    arch := amd64
endif

.PHONY: default
default: list-targets

list-targets:
	@echo "available targets:"
	@LC_ALL=C $(MAKE) -pRrq -f $(firstword $(MAKEFILE_LIST)) : 2>/dev/null \
		| awk -v RS= -F: '/(^|\n)# Files(\n|$$$$)/,/(^|\n)# Finished Make data base/ {if ($$$$1 !~ "^[#.]") {print $$$$1}}' \
		| sort \
		| grep -E -v -e '^[^[:alnum:]]' -e '^$$@$$$$' \
		| sed 's/^/\t/'
	

.PHONY: install-tools
install-tools:

$(eval $(call tool-from-apt,bsdtar,libarchive-tools))
$(eval $(call tool-from-apt,curl,curl))

goreleaser_arch := $(arch)
ifeq ($(goreleaser_arch),amd64)
	goreleaser_arch := x86_64
endif
goreleaser_url := https://github.com/goreleaser/goreleaser/releases/download/v$(GORELEASER_VERSION)/goreleaser_Linux_$(arch).tar.gz
$(eval $(call tool-from-tar-gz,goreleaser,$(goreleaser_url),0))

svu_url := https://github.com/caarlos0/svu/releases/download/v$(SVU_VERSION)/svu_$(SVU_VERSION)_linux_$(arch).tar.gz
$(eval $(call tool-from-tar-gz,svu,$(svu_url),0))
