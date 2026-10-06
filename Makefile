# Сборка LangSwitch. Без аргументов показывает справку.

APP  := langswitch
ICON := assets/icon.png
TRAY := assets/tray.png

# Версия из git-тега (v1.2.3); показывается в настройках и сравнивается с GitHub Releases.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Fyne и платформенный код требуют CGO.
export CGO_ENABLED := 1

ifeq ($(OS),Windows_NT)
	EXT     := .exe
	LDFLAGS := -s -w -H windowsgui
	# Ресурс с иконкой и манифестом; go build подхватывает .syso автоматически.
	RSRC    := rsrc_windows_amd64.syso
else
	EXT     :=
	LDFLAGS := -s -w
	RSRC    :=
endif

BIN   := $(APP)$(EXT)
DEBUG := $(APP)-debug$(EXT)

.DEFAULT_GOAL := help
.PHONY: help build run debug icon tidy fmt vet test check clean

help: ## Показать эту справку
	@echo "Использование: make <цель>"
	@echo
	@awk 'BEGIN { FS = ":.*## " } /^[a-z-]+:.*## / { printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: $(ICON) $(TRAY) $(RSRC) ## Собрать приложение
	go build -trimpath -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" -o $(BIN) .

run: build ## Собрать и запустить
	./$(BIN)

debug: $(ICON) $(TRAY) $(RSRC) ## Собрать с консолью (видны логи) и запустить
	go build -ldflags "-X main.version=$(VERSION)" -o $(DEBUG) .
	./$(DEBUG)

icon: ## Перерисовать иконки приложения и трея
	rm -f $(ICON) $(TRAY) $(RSRC)
	$(MAKE) $(ICON) $(TRAY) $(RSRC)

$(ICON): tools/genicon/main.go
	go run ./tools/genicon -o $(ICON)

$(TRAY): tools/genicon/main.go
	go run ./tools/genicon -tray -o $(TRAY)

rsrc_windows_amd64.syso: $(ICON)
	go run github.com/tc-hib/go-winres@latest simply --arch amd64 --manifest gui --icon $(ICON)

tidy: ## Обновить зависимости (go mod tidy)
	go mod tidy

fmt: ## Отформатировать код
	gofmt -l -w .

vet: ## Статический анализ (go vet)
	go vet ./...

test: ## Запустить тесты
	go test ./...

check: fmt vet test ## fmt + vet + test

clean: ## Удалить собранные файлы
	rm -f $(BIN) $(DEBUG)
