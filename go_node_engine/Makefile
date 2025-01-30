VERSION := $(shell git describe --tags --abbrev=0)
LDFLAGS := -X 'go_node_engine/cmd.Version=$(VERSION)'
NODE_ENGINE := NodeEngine
NODE_ENGINED := nodeengined
ENGINE_SRC := NodeEngine.go
ENGINED_SRC := pkg/nodeengined.go
ARCH ?= amd64

.PHONY: all arm64 amd64 clean install check-arch check-systemd run start stop configure-gpu

all: amd64

arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(NODE_ENGINE)_arm64 $(ENGINE_SRC)
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(NODE_ENGINED)_arm64 $(ENGINED_SRC)

amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(NODE_ENGINE)_amd64 $(ENGINE_SRC)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(NODE_ENGINED)_amd64 $(ENGINED_SRC)

clean:
	rm -f $(NODE_ENGINE)_arm64 $(NODE_ENGINE)_amd64 $(NODE_ENGINED)_arm64 $(NODE_ENGINED)_amd64

check-arch:
	@if [ "$(ARCH)" != "amd64" ] && [ "$(ARCH)" != "arm64" ]; then \
		echo "Architecture not set correctly"; \
		echo "Usage: make install ARCH=<architecture>"; \
		echo "supported architectures: amd64, arm64"; \
		exit 1; \
	fi

check-systemd:
	@if ! systemd --version > /dev/null 2>&1 && ! /usr/lib/systemd/systemd --version > /dev/null 2>&1; then \
		echo "Systemd not present on this machine"; \
		exit 1; \
	fi

install: check-arch check-systemd
	@if ! systemctl | grep -Fq 'containerd'; then \
		wget https://github.com/containerd/containerd/releases/download/v1.6.1/cri-containerd-cni-1.6.1-linux-$(ARCH).tar.gz; \
		chmod 777 cri-containerd-cni-1.6.1-linux-$(ARCH).tar.gz; \
		sudo tar --no-overwrite-dir -C / -xzf cri-containerd-cni-1.6.1-linux-$(ARCH).tar.gz; \
		sudo systemctl daemon-reload; \
		sudo systemctl enable --now containerd; \
		rm cri-containerd-cni-1.6.1-linux-$(ARCH).tar.*; \
	else \
		sudo systemctl daemon-reload; \
		sudo systemctl enable --now containerd; \
	fi
	sudo mkdir -p /var/log/oakestra
	sudo systemctl stop nodeengine || true
	sudo systemctl stop netmanager || true
	sudo cp $(NODE_ENGINE)_$(ARCH) /bin/$(NODE_ENGINE)
	sudo cp $(NODE_ENGINED)_$(ARCH) /bin/$(NODE_ENGINED)
	if [ -e nodeengine.service ]; then \
		sudo cp nodeengine.service /etc/systemd/system/nodeengine.service; \
	else \
		sudo cp build/nodeengine.service /etc/systemd/system/nodeengine.service; \
	fi
	sudo systemctl daemon-reload
	sudo chmod 755 /bin/$(NODE_ENGINE)
	sudo chmod 755 /bin/$(NODE_ENGINED)
	@echo "Done, installation successful" 

run: check-arch
	@echo "Starting $(NODE_ENGINED)..."
	sudo /bin/$(NODE_ENGINED) --debug 

start: check-systemd
	@echo "Starting nodeengine service..."
	sudo systemctl start nodeengine
	@echo "Service started. Check status with: systemctl status nodeengine"
	@systemctl status nodeengine --no-pager

stop: check-systemd
	@echo "Stopping nodeengine service..."
	sudo systemctl stop nodeengine || true
	@echo "Service stopped" 

configure-gpu:
	sudo apt install -y curl
	$(eval DISTRIBUTION := $(shell . /etc/os-release && echo $$ID$$VERSION_ID))
	curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
	curl -s -L https://nvidia.github.io/libnvidia-container/$(DISTRIBUTION)/libnvidia-container.list | \
		sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
		sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
	sudo apt-get update
	sudo apt install -y libnvidia-container1 libnvidia-container-tools
	sudo apt-get install -y nvidia-docker2
	@echo "NVIDIA GPU support configured successfully" 
