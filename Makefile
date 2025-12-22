CONTROLLERGEN_VERSION := 0.20.0
GORELEASER_VERSION := 2.12.7
HELM_VERSION := 4.0.4
HELMIFY_VERSION := 0.4.19
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

controllergen_arch = $(arch)
controllergen_url := https://github.com/kubernetes-sigs/controller-tools/releases/download/v$(CONTROLLERGEN_VERSION)/controller-gen-linux-$(controllergen_arch)
$(eval $(call tool-from-url,controller-gen,$(controllergen_url)))

goreleaser_arch := $(arch)
ifeq ($(goreleaser_arch),amd64)
	goreleaser_arch := x86_64
endif
goreleaser_url := https://github.com/goreleaser/goreleaser/releases/download/v$(GORELEASER_VERSION)/goreleaser_Linux_$(goreleaser_arch).tar.gz
$(eval $(call tool-from-tar-gz,goreleaser,$(goreleaser_url),0))

helm_arch := $(arch)
helm_url := https://get.helm.sh/helm-v$(HELM_VERSION)-linux-$(helm_arch).tar.gz
$(eval $(call tool-from-tar-gz,helm,$(helm_url),1))

helmify_arch := $(arch)
ifeq ($(helmify_arch),amd64)
	helmify_arch := x86_64
endif
helmify_url := https://github.com/arttor/helmify/releases/download/v$(HELMIFY_VERSION)/helmify_Linux_$(helmify_arch).tar.gz
$(eval $(call tool-from-tar-gz,helmify,$(helmify_url),0))

svu_url := https://github.com/caarlos0/svu/releases/download/v$(SVU_VERSION)/svu_$(SVU_VERSION)_linux_$(arch).tar.gz
$(eval $(call tool-from-tar-gz,svu,$(svu_url),0))

.PHONY: generate
generate: generate__gateway-controller

.PHONY: generate__gateway-controller
generate__gateway-controller:
	# clean up generated dirs
	rm -rf ./charts/gateway-controller/generated && mkdir -p ./charts/gateway-controller/generated
	# create deepcopy implementations
	controller-gen object paths="./internal/gatewaycontroller/api/..."
	# create crd manifests
	controller-gen crd paths="./internal/gatewaycontroller/..." output:stdout > charts/gateway-controller/generated/crds.yaml
	# create rbac manifests
	controller-gen rbac:roleName=__roleName__ paths="./internal/gatewaycontroller/..." output:stdout > ./charts/gateway-controller/generated/rbac.yaml