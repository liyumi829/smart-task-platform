# =========================
# 基础目录
# =========================
ROOT_DIR        := $(CURDIR)
BUILD_DIR       := $(ROOT_DIR)/build#编译产物存储路径
MAIN_DIR        := $(ROOT_DIR)/cmd/server#主程序目录
PID_DIR         := $(BUILD_DIR)/pids#后台任务信息
LOG_DIR         := $(ROOT_DIR)/logs#日志存储路径
CONFIG_DIR      := $(ROOT_DIR)/configs#配置文件存储路径

# =========================
# 二进制文件
# =========================
APP_NAME := smart-task-platform
MAIN_NAME := main.go
APP_BIN     := $(BUILD_DIR)/$(APP_NAME)#编译产物
MAIN_FILE := $(MAIN_DIR)/$(MAIN_NAME)#主程序文件

# =========================
# 配置文件
# =========================
CONFIG_FILE     := $(CONFIG_DIR)/config.local.yaml#本地开发环境配置文件路径

# =========================
# 其他变量
# =========================
PORT			?= 8080#应用程序监听的端口
HOST			?= 127.0.0.1#应用程序监听的地址

.PHONY: all prepare build run start stop clean help

all: build

prepare:
	@echo "Preparing build environment..."
	@echo "$(LOG_DIR)"
	@echo "$(PID_DIR)"
	@echo "$(BUILD_DIR)"
	@mkdir -p "$(BUILD_DIR)" "$(PID_DIR)" "$(LOG_DIR)"

build: prepare
	@echo "Building the project..."
	@go build -o "$(APP_BIN)" "$(MAIN_FILE)"
	@echo "Build completed: $(APP_BIN)"

run: build
	@echo "Running the application in foreground..."
	@"$(APP_BIN)" \
	-c "$(CONFIG_FILE)" \
	-port "$(PORT)" \
	-host "$(HOST)"	


start: build
	@echo "Starting the application..."
	@"$(APP_BIN)" \
	-c "$(CONFIG_FILE)" \
	-port "$(PORT)" \
	-host "$(HOST)" \
	> "$(LOG_DIR)/app.log" 2>&1 &
	@echo $$! > "$(PID_DIR)/app.pid"
	@echo "Application is running with PID: $$(cat $(PID_DIR)/app.pid)"

stop:
	@echo "Stopping the application..."
	@kill -9 $$(cat "$(PID_DIR)/app.pid") 2>/dev/null || true
	@rm -f "$(PID_DIR)/app.pid"
	@echo "Application stopped."

clean:
	@echo "Cleaning build artifacts..."
	@kill -9 $$(cat "$(PID_DIR)/app.pid") 2>/dev/null || true
	@rm -rf "$(BUILD_DIR)" "$(PID_DIR)"
	@echo "Clean completed."	

help:
	@echo "Makefile for Smart Task Platform"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all       - Build the project (default)"
	@echo "  prepare   - Prepare the build environment"
	@echo "  build     - Build the project"
	@echo "  run       - Run the application in foreground"
	@echo "  start     - Start the application in background"
	@echo "  clean     - Clean build artifacts and logs"
	@echo "  help      - Show this help message"

