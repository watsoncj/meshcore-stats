BINARY := meshcore-stats
INSTALL_PATH := /usr/local/bin/$(BINARY)

.PHONY: build install clean

build:
	go build -o $(BINARY) ./cmd/meshcore-stats

install: build
	sudo systemctl stop $(BINARY) || true
	sudo cp $(BINARY) $(INSTALL_PATH)
	sudo systemctl start $(BINARY)

clean:
	rm -f $(BINARY)
