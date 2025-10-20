.PHONY: build install clean test install-service uninstall

VERSION := 0.0.1
BINARY := wake-on-demand
PREFIX := /usr/local

build:
	@echo "Building $(BINARY)..."
	go build -ldflags="-X main.VERSION=$(VERSION)" -o $(BINARY) .

install: build
	@echo "Installing to $(PREFIX)/bin/..."
	sudo cp $(BINARY) $(PREFIX)/bin/
	@echo "✓ Installed to $(PREFIX)/bin/$(BINARY)"
	@echo ""
	@echo "Usage:"
	@echo "  $(BINARY) server          # Start server"
	@echo "  $(BINARY) list            # List ESPs"
	@echo "  $(BINARY) on <esp_id>     # Power on"

install-service: install
	@echo "Installing systemd service..."
	@mkdir -p systemd
	@echo "[Unit]" > systemd/wake-on-demand.service
	@echo "Description=Wake-On-Demand Server" >> systemd/wake-on-demand.service
	@echo "After=network.target" >> systemd/wake-on-demand.service
	@echo "" >> systemd/wake-on-demand.service
	@echo "[Service]" >> systemd/wake-on-demand.service
	@echo "Type=simple" >> systemd/wake-on-demand.service
	@echo "ExecStart=$(PREFIX)/bin/$(BINARY) server" >> systemd/wake-on-demand.service
	@echo "Restart=always" >> systemd/wake-on-demand.service
	@echo "RestartSec=5" >> systemd/wake-on-demand.service
	@echo "User=root" >> systemd/wake-on-demand.service
	@echo "" >> systemd/wake-on-demand.service
	@echo "# Security options" >> systemd/wake-on-demand.service
	@echo "NoNewPrivileges=true" >> systemd/wake-on-demand.service
	@echo "PrivateTmp=true" >> systemd/wake-on-demand.service
	@echo "ProtectSystem=strict" >> systemd/wake-on-demand.service
	@echo "ProtectHome=true" >> systemd/wake-on-demand.service
	@echo "" >> systemd/wake-on-demand.service
	@echo "[Install]" >> systemd/wake-on-demand.service
	@echo "WantedBy=multi-user.target" >> systemd/wake-on-demand.service
	sudo cp systemd/wake-on-demand.service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo "✓ Service installed"
	@echo ""
	@echo "To enable and start:"
	@echo "  sudo systemctl enable wake-on-demand"
	@echo "  sudo systemctl start wake-on-demand"
	@echo ""
	@echo "To view logs:"
	@echo "  sudo journalctl -u wake-on-demand -f"

clean:
	@echo "Cleaning..."
	rm -f $(BINARY)
	rm -rf systemd/
	@echo "✓ Clean complete"

test:
	@echo "Running tests..."
	go test -v ./...

uninstall:
	@echo "Uninstalling..."
	sudo systemctl stop wake-on-demand 2>/dev/null || true
	sudo systemctl disable wake-on-demand 2>/dev/null || true
	sudo rm -f /etc/systemd/system/wake-on-demand.service
	sudo systemctl daemon-reload
	sudo rm -f $(PREFIX)/bin/$(BINARY)
	@echo "✓ Uninstalled"