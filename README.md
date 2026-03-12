## O2Stock-Crawler

A tool implemented in Golang for crawling and storing NBA2K Online2 player price data.

### Features Overview

- **Periodic/One-time Invocation**: Request official interfaces to retrieve player price data from follow lists.
- **JSON Parsing & Model Mapping**: Parse `rosterList` data returned by the interface.
- **MySQL Persistence**:
  - Update/Insert current player prices into the `players`表.
  - Write price snapshots into the `p_p_history` table for each crawl.

### Project Architecture

The project follows a clear layered architecture (Clean Architecture / DDD pattern):

- **`internal/entity/`**: Domain entity models corresponding to MySQL table structures (GORM).
- **`internal/dto/`**: Data Transfer Objects for API response and request JSON serialization.
- **`internal/db/repositories/`**: Data Access Layer (Repository) responsible for pure CRUD operations.
- **`internal/service/`**: Business Logic Layer (Service) handling complex workflows and conversions between Entity and DTO.
- **`internal/controller/`**: Interface Layer responsible for HTTP requests, authentication, and response dispatching.
- **`api/`**: API definitions and public contracts.

### Environment Configuration

The program reads interface and database configurations via environment variables. You can create a `.env` file in the root directory, which will be automatically loaded during local development using `github.com/joho/godotenv`.

#### OL2 Interface Configuration

- **OL2_OPENID**: `openid` used in the interface.
- **OL2_ACCESS_TOKEN**: `access_token` used in the interface.
- **OL2_SIGN**: `sign` used in the interface.
- **OL2_NONSE_STR**: `nonseStr`.
- **OL2_BASE_URL**: API Base URL (refer to `.env` config file).

Example (please modify based on your actual account info):

```env
OL2_OPENID=
OL2_ACCESS_TOKEN=
OL2_SIGN=
OL2_NONSE_STR=
OL2_BASE_URL=https://nba2k2app.com/
```

#### Database Configuration

Override via environment variables:

```env
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASS=
DB_NAME=
```

### Install Dependencies

Execute in the project root:

```bash
go mod tidy
```

### Usage

The program entry point is `cmd/o2stock-crawler/main.go`.

- **One-time crawling and storage**:

```bash
go run ./cmd/o2stock-crawler
# OR
go run ./cmd/o2stock-crawler run-once
```

- **Looping periodic crawling** (e.g., every 1 hour):

```bash
go run ./cmd/o2stock-crawler loop 1h
```

Interval parameter uses Go's duration syntax (e.g., `30m`, `2h`, `90m`); defaults to 60 minutes if omitted.

## API Service

The project also provides an HTTP API service for querying player data and user transaction records.

### Start API Service

```bash
go run ./cmd/o2stock-api
```

Listens on `:8080` by default; can be modified via the `API_ADDR` environment variable.

### Running Tests

The project includes unit tests:

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/db/...

# Skip tests requiring database (short mode)
go test -short ./...
```

**Note:** Database-related tests require a real database connection. Tests will automatically skip if unable to connect.

## Build and Deployment

The project provides a `build.sh` script for rapid building, supporting configuration injection into the binary for easy deployment. It also includes Systemd service configuration for Linux (e.g., CentOS) systems.

### 1. `build.sh` Script Instructions

**Script Overview**
`build.sh` is an automated build script that reads the contents of the `.env` file in the current directory and injects them into the binary via Go's `-ldflags`. This allows the program to run using injected configurations if no external `.env` file is found, enabling "zero-config" deployment.

**Requirements**
- OS: Linux / macOS
- Dependencies: Go 1.18+
- Required File: A `.env` config file must exist in the root (for default config injection).

**Steps**

1.  **Set Permissions**
    ```bash
    chmod +x build.sh
    ```

2.  **Standard Build Command**
    - Build Crawler:
      ```bash
      ./build.sh
      ```
      Generates `o2stock-crawler` executable.

    - Build API Service:
      ```bash
      ./build.sh o2stock-api o2stock-api
      ```
      Generates `o2stock-api` executable.

3.  **Optional Parameters**
    Usage: `./build.sh [output_name] [target_cmd]`
    - `output_name`: (Optional) Output binary filename, defaults to `o2stock-crawler`.
    - `target_cmd`: (Optional) Target program directory name under `cmd/`. Use `o2stock-api` for API.

**Expected Output**
A binary file with the specified name will be generated in the current directory. The file size is typically slightly larger than a version without injected configuration (as it contains `.env` content).

### 2. Systemd Service Management

For Linux distributions using Systemd (like CentOS), it is recommended to manage the `o2stock-api` service via Systemd.

**Service Installation**

1.  **Modify Configuration File**
    Modify the `o2stock-api.service` file in the project root:
    - `WorkingDirectory`: Change to the actual directory where the program is located (e.g., `/opt/o2stock`).
    - `ExecStart`: Change to the absolute path of the executable (e.g., `/opt/o2stock/o2stock-api`).
    - `User`: Recommended to change to a non-root user (optional).

2.  **Copy File**
    ```bash
    sudo cp o2stock-api.service /etc/systemd/system/
    ```

3.  **Reload Configuration**
    ```bash
    sudo systemctl daemon-reload
    ```

**Common Commands**

- **Start Service**
  ```bash
  sudo systemctl start o2stock-api
  ```

- **Enable Auto-start on Boot**
  ```bash
  sudo systemctl enable o2stock-api
  ```

- **Check Status**
  ```bash
  sudo systemctl status o2stock-api
  ```

- **Stop Service**
  ```bash
  sudo systemctl stop o2stock-api
  ```

- **Restart Service**
  ```bash
  sudo systemctl restart o2stock-api
  ```

**Viewing Logs**
Logs output to the system log by default, viewable via `journalctl`:
```bash
# View real-time logs
journalctl -u o2stock-api -f

# View last 100 lines
journalctl -u o2stock-api -n 100
```

**File Locations**
- **Systemd Unit File**: `/etc/systemd/system/o2stock-api.service`
- **Program Configuration**: Usually `.env` in the run directory, or builtin configuration if `.env` is absent.

### 3. Precautions

**Permission Requirements**
- Executing the build script requires write access to the project directory.
- Managing Systemd services (`start`, `stop`, `enable`, copying to `/etc/systemd/system`) typically requires `root` or `sudo` privileges.

**Troubleshooting**
- **Build Failure**: Check Go environment and ensure `go mod tidy` has been executed.
- **Service Fails to Start**:
  - Check `WorkingDirectory` and `ExecStart` paths.
  - Ensure binary has execution permissions (`chmod +x o2stock-api`).
  - View detailed errors with `journalctl -u o2stock-api -xe`.
- **Config Not Taking Effect**: The program prioritizes the `.env` file in the run directory. If not present, it uses the built-in configuration.

**Compatibility**
- **OS**: CentOS 7+, Ubuntu 16.04+, Debian 8+, etc.
- **Go Version**: Go 1.18+ recommended.

### Future Extensions

- Use `cron` (crontab) to trigger `run-once`.
- Add more fields to database, such as `grade`, `popularity`, etc.
- Add file logging and Prometheus monitoring.
- Add user authentication middleware (retrieve `user_id` from token).
- Add more statistics and analysis APIs.

## Maintainer

This project is actively maintained by @supernarsi.

## Roadmap

- Improve data reliability
- Add automated data validation
- Improve developer documentation