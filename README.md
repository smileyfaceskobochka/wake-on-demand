# Wake-On-Demand

Wake-On-Demand is a lightweight server and client system for remotely controlling ESP-based devices to power on/off servers or devices on demand. It uses HTTP and JSON for communication and supports short and long power pulses.

## Features

- Remote registration of ESP devices
- Short pulse (`on`) to power on devices
- Long pulse (`off`) to force shutdown
- List registered ESP devices
- Easy installation via Makefile
- Systemd service support for running the server as a daemon

## Requirements

- Go 1.21+
- Linux-based server for running the server component
- ESP32/ESP8266 devices with Wi-Fi 

[esp-client](https://github.com/smileyfaceskobochka/wake-on-demand-esp/)

## Installation

### Build and install

```bash
make install
````

This will:

* Build the `wake-on-demand` binary
* Copy it to `/usr/local/bin/`

### Install as systemd service

```bash
make install-service
```

This will:

* Install a systemd service
* Enable restart on crash
* Set security options

Enable and start the service:

```bash
sudo systemctl enable wake-on-demand
sudo systemctl start wake-on-demand
```

Check logs:

```bash
sudo journalctl -u wake-on-demand -f
```

## Usage

Start the server:

```bash
wake-on-demand server
```

List registered ESP devices:

```bash
wake-on-demand list
```

Send commands to ESP devices:

```bash
wake-on-demand on <esp_id>    # Short pulse to power on
wake-on-demand off <esp_id>   # Long pulse to force shutdown
```

### Options

```
-port <port>        Server port (default: 8080)
-server <url>       Server URL for client commands (default: http://localhost:8080)
-timeout <duration> ESP timeout duration (default: 30s)
-version            Print version
-help               Show help
```

## Makefile Commands

* `make build` – Build the binary
* `make install` – Build and install binary
* `make install-service` – Install as systemd service
* `make clean` – Remove build artifacts
* `make uninstall` – Remove binary and service

## License

MIT License
