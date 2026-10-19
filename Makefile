.ONESHELL:
PRODUCT_NAME=hiddify-core
BASENAME=$(PRODUCT_NAME)
BINDIR=bin
LIBNAME=$(PRODUCT_NAME)
CLINAME=HiddifyCli

BRANCH=$(shell git branch --show-current)
VERSION=$(shell git describe --tags || echo "unknown version")
ifeq ($(OS),Windows_NT)
Not available for Windows! use bash in WSL
endif
CRONET_GO_VERSION := $(shell cat hiddify-sing-box/.github/CRONET_GO_VERSION)
TAGS=with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api,with_grpc,with_awg,tfogo_checklinkname0,with_naive_outbound,with_conntrack,with_embedded_tor,with_openvpn,with_openconnect
IOS_ADD_TAGS=with_dhcp,with_low_memory,with_purego
MACOS_ADD_TAGS=with_dhcp
WINDOWS_ADD_TAGS=with_purego
# Optional build-time secrets (GitHub Actions): HIDDIFY_SECRET_KEY (base64 AES-256 key) is embedded,
# and PSIPHON_CONFIG (Psiphon config, JSON or base64) is embedded encrypted with it (config "hiddify").
EMBED_LDFLAGS=
# Keep the local-only entry out of the legacy parse-time encryption command.
ifneq ($(filter ios-local,$(MAKECMDGOALS)),)
ifneq ($(words $(MAKECMDGOALS)),1)
$(error ios-local must be invoked by itself)
endif
ifneq ($(strip $(HIDDIFY_SECRET_KEY)$(PSIPHON_CONFIG)),)
$(error ios-local requires HIDDIFY_SECRET_KEY and PSIPHON_CONFIG to be unset)
endif
else
ifneq ($(strip $(HIDDIFY_SECRET_KEY)),)
EMBED_LDFLAGS+=-X github.com/sagernet/sing-box/hiddify/secret.Key=$(strip $(HIDDIFY_SECRET_KEY))
ifneq ($(strip $(PSIPHON_CONFIG)),)
PSIPHON_CONFIG_ENCRYPTED:=$(shell go run ./cmd/internal/encryptconfig)
ifeq ($(PSIPHON_CONFIG_ENCRYPTED),)
$(error failed to encrypt PSIPHON_CONFIG)
endif
EMBED_LDFLAGS+=-X github.com/sagernet/sing-box/protocol/psiphon.embeddedHiddifyConfig=$(PSIPHON_CONFIG_ENCRYPTED)
endif
endif
endif
LDFLAGS=-w -s -checklinkname=0 -buildid= $${CODE_VERSION} $(EMBED_LDFLAGS)
GOBUILDLIB=CGO_ENABLED=1 go build -trimpath -ldflags="$(LDFLAGS)" -buildmode=c-shared
GOBUILDSRV=CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -trimpath -tags $(TAGS)

CRONET_DIR=./cronet
.PHONY: protos
protos:
	go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@latest
	# protoc --go_out=./ --go-grpc_out=./ --proto_path=hiddifyrpc hiddifyrpc/*.proto
	# for f in $(shell find v2 -name "*.proto"); do \
	# 	protoc --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative --go_out=./ --go-grpc_out=./  $$f; \
	# done
	# for f in $(shell find extension -name "*.proto"); do \
	# 	protoc --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative --go_out=./ --go-grpc_out=./  $$f; \
	# done
	protoc --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative --go_out=./ --go-grpc_out=./  $(shell find v2 -name "*.proto") $(shell find extension -name "*.proto")
	protoc --doc_out=./docs  --doc_opt=markdown,hiddifyrpc.md $(shell find v2 -name "*.proto") $(shell find extension -name "*.proto")
	# protoc --js_out=import_style=commonjs,binary:./extension/html/rpc/ --grpc-web_out=import_style=commonjs,mode=grpcwebtext:./extension/html/rpc/ $(shell find v2 -name "*.proto") $(shell find extension -name "*.proto")
	# npx browserify extension/html/rpc/extension.js >extension/html/rpc.js


lib_install: prepare
	go install -v github.com/sagernet/gomobile/cmd/gomobile@v0.1.11
	go install -v github.com/sagernet/gomobile/cmd/gobind@v0.1.11
	npm install

headers:
	go build -buildmode=c-archive -o $(BINDIR)/ ./platform/desktop2

android: lib_install
	CGO_LDFLAGS="-O2 -g -s -w -Wl,-z,max-page-size=16384" gomobile bind -v -androidapi=21 -javapkg=com.hiddify.core -libname=hiddify-core -tags=$(TAGS) -trimpath -ldflags="$(LDFLAGS)" -target=android -gcflags "all=-N -l" -o $(BINDIR)/$(LIBNAME).aar github.com/sagernet/sing-box/experimental/libbox ./platform/mobile

ios-full: lib_install
	gomobile bind -v  -target ios,iossimulator,tvos,tvossimulator,macos -libname=hiddify-core -tags=$(TAGS),$(IOS_ADD_TAGS) -trimpath -ldflags="$(LDFLAGS)" -o $(BINDIR)/$(PRODUCT_NAME).xcframework github.com/sagernet/sing-box/experimental/libbox ./platform/mobile 
	mv $(BINDIR)/$(PRODUCT_NAME).xcframework $(BINDIR)/$(LIBNAME).xcframework 
	cp HiddifyCore.podspec $(BINDIR)/$(LIBNAME).xcframework/

ios: lib_install
	gomobile bind -v  -target ios -libname=hiddify-core -tags=$(TAGS),$(IOS_ADD_TAGS) -trimpath -ldflags="$(LDFLAGS)" -o $(BINDIR)/HiddifyCore.xcframework github.com/sagernet/sing-box/experimental/libbox ./platform/mobile
	cp Info.plist $(BINDIR)/HiddifyCore.xcframework/


# Local, offline, device-only build; no lib_install/prepare/tidy/npm or downloads.
# After native dependencies are provisioned and a bind is separately authorized:
#   env -u HIDDIFY_SECRET_KEY -u PSIPHON_CONFIG make ios-local \
#     IOS_LOCAL_GO=/absolute/private/go1.26.3/bin/go \
#     IOS_LOCAL_ROOT=/absolute/existing/private/core-attempt
# ROOT layout: tools/bin/{gomobile,gobind} prebuilt at v0.1.12; gopath/;
# cache/{go-mod,go-build,clang,xdg}/; tmp/; home/; out/HiddifyCore.xcframework.
# Populate the private module/native caches beforehand; missing inputs fail offline.
# out/ must not exist. Failed/partial output is retained; choose a new attempt root.
# Keep gomobile's generated Info.plist, not the legacy simulator/iOS 15.0 manifest.
# Native dependency/linkage completeness is NOT established by this entry's checks.
export IOS_LOCAL_GO IOS_LOCAL_ROOT
.PHONY: ios-local
ios-local:
	@set -eu; \
	fail() { printf 'ios-local: %s\n' "$$*" >&2; exit 1; }; \
	case "$${IOS_LOCAL_ROOT:-}" in /*) ;; *) fail 'IOS_LOCAL_ROOT must be an absolute private directory';; esac; \
	[ -d "$$IOS_LOCAL_ROOT" ] || fail 'IOS_LOCAL_ROOT must already exist'; \
	root=$$(CDPATH= cd "$$IOS_LOCAL_ROOT" && pwd -P); \
	case "$${IOS_LOCAL_GO:-}" in /*) ;; *) fail 'IOS_LOCAL_GO must be an absolute Go 1.26.3 executable';; esac; \
	[ -x "$$IOS_LOCAL_GO" ] || fail 'IOS_LOCAL_GO is not executable'; \
	out="$$root/out"; \
	[ ! -e "$$out" ] && [ ! -L "$$out" ] || fail 'out already exists; refusing to overwrite it'; \
	for dir in tools tools/bin gopath gopath/pkg gopath/pkg/gomobile cache cache/go-mod cache/go-build cache/clang cache/xdg tmp home; do \
		[ ! -L "$$root/$$dir" ] || fail "private directory must not be a symlink: $$dir"; \
	done; \
	for tool in gomobile gobind; do \
		[ -f "$$root/tools/bin/$$tool" ] && [ -x "$$root/tools/bin/$$tool" ] && [ ! -L "$$root/tools/bin/$$tool" ] || fail "missing private executable: $$tool"; \
	done; \
	grep -Eq '^go 1[.]26[.]3$$' go.mod || fail 'go.mod must declare Go 1.26.3'; \
	grep -Eq '^[[:space:]]*github[.]com/sagernet/gomobile[[:space:]]+v0[.]1[.]12([[:space:]]|$$)' go.mod || fail 'go.mod must pin gomobile v0.1.12'; \
	umask 077; \
	export HOME="$$root/home" GOPATH="$$root/gopath" GOBIN="$$root/tools/bin"; \
	export GOMODCACHE="$$root/cache/go-mod" GOCACHE="$$root/cache/go-build"; \
	export GOTMPDIR="$$root/tmp" TMPDIR="$$root/tmp" TMP="$$root/tmp" TEMP="$$root/tmp"; \
	export CLANG_MODULE_CACHE_PATH="$$root/cache/clang" XDG_CACHE_HOME="$$root/cache/xdg"; \
	export GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOENV=off GOWORK=off GOTELEMETRY=off GOCACHEPROG=; \
	export GOPROXY=off GOSUMDB=off GOVCS='*:off' GOAUTH=off; \
	export IPHONEOS_DEPLOYMENT_TARGET=15.5 CODE_VERSION="$${CODE_VERSION:-}"; \
	unset GOROOT GOOS GOARCH GOARM GOAMD64 GOMIPS GOMIPS64; \
	export PATH="$$GOBIN:$$(dirname "$$IOS_LOCAL_GO"):$$PATH"; \
	[ "$$(command -v go)" = "$$IOS_LOCAL_GO" ] || fail 'PATH go must resolve to IOS_LOCAL_GO'; \
	mkdir -p "$$HOME" "$$GOPATH" "$$GOMODCACHE" "$$GOCACHE" "$$GOTMPDIR" "$$CLANG_MODULE_CACHE_PATH" "$$XDG_CACHE_HOME"; \
	[ "$$("$$IOS_LOCAL_GO" env GOVERSION)" = go1.26.3 ] || fail 'Go version must be exactly go1.26.3'; \
	for tool in gomobile gobind; do \
		info=$$("$$IOS_LOCAL_GO" version -m "$$GOBIN/$$tool"); \
		printf '%s\n' "$$info" | grep -Eq "^[[:space:]]*path[[:space:]]+github[.]com/sagernet/gomobile/cmd/$$tool$$" || fail "unexpected tool path: $$tool"; \
		printf '%s\n' "$$info" | grep -Eq '^[[:space:]]*mod[[:space:]]+github[.]com/sagernet/gomobile[[:space:]]+v0[.]1[.]12([[:space:]]|$$)' || fail "$$tool must be gomobile v0.1.12"; \
		if printf '%s\n' "$$info" | grep -Eq '^[[:space:]]*=>'; then fail "replaced tool modules are not allowed: $$tool"; fi; \
	done; \
	mkdir "$$out" || fail 'cannot reserve new output directory'; \
	"$$GOBIN/gomobile" bind -v -target ios/arm64 -iosversion=15.5 -libname=hiddify-core -tags=$(TAGS),$(IOS_ADD_TAGS) -trimpath -ldflags="$(LDFLAGS)" -o "$$out/HiddifyCore.xcframework" github.com/sagernet/sing-box/experimental/libbox ./platform/mobile

webui:
	curl -L -o webui.zip  https://github.com/hiddify/Yacd-meta/archive/gh-pages.zip 
	unzip -d ./ -q webui.zip
	rm webui.zip
	rm -rf bin/webui
	mv Yacd-meta-gh-pages bin/webui

.PHONY: build
windows-amd64: prepare
	rm -rf $(BINDIR)/*
	$(MAKE) BINDIR=$(BINDIR) fetch-libcronet-windows_amd64.dll
	env GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc  $(GOBUILDLIB) -tags $(TAGS),$(WINDOWS_ADD_TAGS)   -o $(BINDIR)/$(LIBNAME).dll ./platform/desktop
	echo "core built, now building cli" 
	ls -R $(BINDIR)/
	go install -mod=readonly github.com/akavel/rsrc@latest ||echo "rsrc error in installation"
	go run ./cli tunnel exit
	cp $(BINDIR)/$(LIBNAME).dll ./$(LIBNAME).dll
	$$(go env GOPATH)/bin/rsrc -ico ./assets/hiddify-cli.ico -o ./cmd/bydll/cli.syso ||echo "rsrc error in syso"
	env GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc CGO_LDFLAGS="$(LIBNAME).dll" $(GOBUILDSRV) -o $(BINDIR)/$(CLINAME).exe ./cmd/bydll
	rm ./*.dll
	if [ ! -f $(BINDIR)/$(LIBNAME).dll -o ! -f $(BINDIR)/$(CLINAME).exe -o ! -f $(BINDIR)/libcronet.dll ]; then \
		echo "Error: $(LIBNAME).dll, $(CLINAME).exe or libcronet.dll missing"; \
		exit 1; \
	fi

# 	make webui
	



# Prebuilt libcronet from cronet-go's lib/<os>_<arch>/ (same as upstream sing-box CI).
# Usage: make fetch-libcronet-windows_amd64.dll
fetch-libcronet-%:
	rm -rf $(BINDIR)/.cronet-go-lib
	git init -q $(BINDIR)/.cronet-go-lib
	git -C $(BINDIR)/.cronet-go-lib remote add origin https://github.com/sagernet/cronet-go.git
	git -C $(BINDIR)/.cronet-go-lib sparse-checkout set --no-cone "/lib/$(basename $*)/libcronet$(suffix $*)"
	git -C $(BINDIR)/.cronet-go-lib fetch -q --depth=1 --filter=blob:none origin $(CRONET_GO_VERSION)
	git -C $(BINDIR)/.cronet-go-lib checkout -q FETCH_HEAD
	cp $(BINDIR)/.cronet-go-lib/lib/$(basename $*)/libcronet$(suffix $*) $(BINDIR)/
	rm -rf $(BINDIR)/.cronet-go-lib

cronet-%:
	$(MAKE) ARCH=$* build-cronet

build-cronet:
# 	rm -rf $(CRONET_DIR)
	git init $(CRONET_DIR) || echo "dir exist"
	cd $(CRONET_DIR) && \
	git remote add origin https://github.com/sagernet/cronet-go.git ||echo "remote exist"; \
	git fetch --depth=1 origin $(CRONET_GO_VERSION) && \
	git checkout FETCH_HEAD && \
	git submodule update --init --recursive --depth=1 && \
	if [ "$${VARIANT}" = "musl" ]; then \
		go run ./cmd/build-naive --target=linux/$(ARCH) --libc=musl download-toolchain && \
		go run ./cmd/build-naive --target=linux/$(ARCH) --libc=musl env > cronet.env; \
	else \
		go run ./cmd/build-naive --target=linux/$(ARCH) download-toolchain && \
		go run ./cmd/build-naive --target=linux/$(ARCH) env > cronet.env; \
	fi

################################
# Generic Linux Builder
################################
linux-%:
	$(MAKE) ARCH=$* build-linux

define load_cronet_env
set -a; \
while IFS= read -r line; do \
    key=$${line%%=*}; \
    value=$${line#*=}; \
    export "$$key=$$value"; \
	echo "$$key=$$value"; \
done < $(CRONET_DIR)/cronet.env; \
set +a;
endef

build-linux: prepare
	mkdir -p $(BINDIR)/lib

	$(load_cronet_env)
	FINAL_TAGS=$(TAGS); \
	if [ "$${VARIANT}" = "musl" ]; then \
		FINAL_TAGS=$${FINAL_TAGS},with_musl; \
	elif [ "$${VARIANT}" = "purego" ]; then \
		FINAL_TAGS="$${FINAL_TAGS},with_purego"; \
	fi; \
	echo "FinalTags: $$FINAL_TAGS"; \
	GOOS=linux GOARCH=$(ARCH) $(GOBUILDLIB) -tags $${FINAL_TAGS} -o $(BINDIR)/lib/$(LIBNAME).so ./platform/desktop ;\
	
	echo "Core library built, now building CLI with CGO linking to core library"
	mkdir lib
	cp $(BINDIR)/lib/$(LIBNAME).so ./lib/$(LIBNAME).so

	GOOS=linux GOARCH=$(ARCH) CGO_LDFLAGS="./lib/$(LIBNAME).so -Wl,-rpath,\$$ORIGIN/lib -fuse-ld=lld" $(GOBUILDSRV) -o $(BINDIR)/$(CLINAME) ./cmd/bydll
	
	rm -rf ./lib/*.so
	chmod +x $(BINDIR)/$(CLINAME)
	if [ ! -f $(BINDIR)/lib/$(LIBNAME).so -o ! -f $(BINDIR)/$(CLINAME) ]; then \
		echo "Error: $(LIBNAME).so or $(CLINAME) not built"; \
		ls -R $(BINDIR); \
		exit 1; \
	fi
# 	make webui


linux-custom: prepare  install_cronet
	mkdir -p $(BINDIR)/
	#env GOARCH=mips $(GOBUILDSRV) -o $(BINDIR)/$(CLINAME) ./cmd/
	$(load_cronet_env)
	go build -ldflags="$(LDFLAGS)" -trimpath -tags $(TAGS) -o $(BINDIR)/$(CLINAME) ./cmd/main
	chmod +x $(BINDIR)/$(CLINAME)
	make webui

macos-amd64:
	env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=10.11 -O2" CGO_LDFLAGS="-mmacosx-version-min=10.11 -O2 -lpthread" CGO_ENABLED=1 go build -trimpath -ldflags="$(EMBED_LDFLAGS)" -tags $(TAGS),$(MACOS_ADD_TAGS) -buildmode=c-shared -o $(BINDIR)/$(LIBNAME)-amd64.dylib ./platform/desktop
macos-arm64:
	env GOOS=darwin GOARCH=arm64 CGO_CFLAGS="-mmacosx-version-min=10.11 -O2" CGO_LDFLAGS="-mmacosx-version-min=10.11 -O2 -lpthread" CGO_ENABLED=1 go build -trimpath -ldflags="$(EMBED_LDFLAGS)" -tags $(TAGS),$(MACOS_ADD_TAGS) -buildmode=c-shared -o $(BINDIR)/$(LIBNAME)-arm64.dylib ./platform/desktop
	
macos: prepare macos-amd64 macos-arm64 
	
	lipo -create $(BINDIR)/$(LIBNAME)-amd64.dylib $(BINDIR)/$(LIBNAME)-arm64.dylib -output $(BINDIR)/$(LIBNAME).dylib
	cp $(BINDIR)/$(LIBNAME).dylib ./$(LIBNAME).dylib 
	mv $(BINDIR)/$(LIBNAME)-arm64.h $(BINDIR)/desktop.h 
	# env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=10.15" CGO_LDFLAGS="-mmacosx-version-min=10.15" CGO_LDFLAGS="bin/$(LIBNAME).dylib"  CGO_ENABLED=1 $(GOBUILDSRV)  -o $(BINDIR)/$(CLINAME) ./cmd/bydll
	# rm ./$(LIBNAME).dylib
	# chmod +x $(BINDIR)/$(CLINAME)

prepare: 
	go mod tidy

clean:
	rm $(BINDIR)/*




.PHONY: release
release: # Create a new tag for release.	
	@bash -c '.github/change_version.sh'
	


