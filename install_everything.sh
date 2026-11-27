#!/bin/bash

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'
gsmLoraServer="https://github.com/sou1burn/GsmLoraServer.git"
loraInterface="https://github.com/Ykio7614/LoRaStandInterface.git"

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Проверка прав администратора
check_sudo() {
    if [ "$EUID" -ne 0 ]; then
        print_warning "Скрипт требует прав суперпользователя для установки пакетов"
        print_info "Запрос прав sudo"
        sudo -v
        if [ $? -ne 0 ]; then
            print_error "Не удалось получить права sudo"
            exit 1
        fi
    fi
}

detect_distro() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        VER=$VERSION_ID
    else
        print_error "Не удалось определить дистрибутив"
        exit 1
    fi
}

# Установка Docker
install_docker() {
    print_info "Проверка установки Docker..."

    if command -v docker &> /dev/null; then
        print_info "Docker уже установлен"
        docker --version
        return 0
    fi

    print_info "Установка Docker..."

    case $OS in
        ubuntu)
            print_info "Добавление GPG ключа Docker..."
            sudo apt update
            sudo apt install -y ca-certificates curl
            sudo install -m 0755 -d /etc/apt/keyrings
            sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
            sudo chmod a+r /etc/apt/keyrings/docker.asc

            print_info "Добавление репозитория Docker..."
            if [ -f /etc/os-release ]; then
                . /etc/os-release
                UBUNTU_CODENAME=${UBUNTU_CODENAME:-$VERSION_CODENAME}
            else
                UBUNTU_CODENAME=$(lsb_release -cs)
            fi

            echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $UBUNTU_CODENAME stable" | \
                sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

            print_info "Обновление пакетов и установка Docker..."
            sudo apt update
            sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

            print_info "Docker успешно установлен"
            ;;
        debian)
            print_info "Установка Docker для Debian..."
            sudo apt-get update
            sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common gnupg
            curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
            echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/debian $(lsb_release -cs) stable" | \
                sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
            sudo apt-get update
            sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
            print_info "Docker успешно установлен"
            ;;
        *)
            print_error "Неподдерживаемый дистрибутив: $OS"
            print_info "Установите Docker вручную: https://docs.docker.com/engine/install/"
            exit 1
            ;;
    esac

    print_info "Запуск службы Docker..."
    sudo systemctl start docker
    sudo systemctl enable docker

    if ! groups $USER | grep -q '\bdocker\b'; then
        print_info "Добавление пользователя $USER в группу docker"
        sudo usermod -aG docker "$USER"
        print_warning "Для применения изменений группы docker необходимо перезапустить сессию (выйти и заново войти в систему)"
        print_info "Вы можете продолжить работу, но для использования docker без sudo потребуется перезагрузка сессии"
    fi

    print_info "Проверка версии Docker"
    docker --version

    print_info "Проверка версии Docker Compose"
    if docker compose version &> /dev/null; then
        docker compose version
    elif command -v docker-compose &> /dev/null; then
        docker-compose --version
    else
        print_warning "Docker Compose не установлен"
    fi
}

# Установка Java через SDKMAN
install_java_via_sdkman() {
    print_info "Установка Java через SDKMAN"

    if [ ! -d "$HOME/.sdkman" ]; then
        print_info "Установка SDKMAN..."
        curl -s "https://get.sdkman.io" | bash
        source "$HOME/.sdkman/bin/sdkman-init.sh"
    else
        source "$HOME/.sdkman/bin/sdkman-init.sh"
    fi

    if command -v sdk &> /dev/null; then
        print_info "Установка Java 23 Temurin через SDKMAN..."
        sdk install java 23.0.1-tem

        sdk default java 23.0.1-tem

        print_info "Java 23 успешно установлена через SDKMAN"
        java -version
    else
        print_error "Не удалось установить SDKMAN"
        return 1
    fi
}

install_maven() {
    print_info "Проверка установки Maven"

    if command -v mvn &> /dev/null; then
        print_info "Maven уже установлен"
        mvn --version | head -1
        return 0
    fi

    print_info "Установка Maven"

    case $OS in
        ubuntu|debian)
            sudo apt update
            sudo apt install -y maven
            ;;
        centos|rhel|fedora)
            sudo yum install -y maven
            ;;
        *)
            print_error "Неподдерживаемый дистрибутив для автоматической установки Maven"
            print_info "Установите Maven вручную: https://maven.apache.org/install.html"
            return 1
            ;;
    esac

    if command -v mvn &> /dev/null; then
        print_info "Maven успешно установлен"
        mvn --version | head -1
    else
        print_error "Не удалось установить Maven"
        return 1
    fi
}

download_repos() {
    local target_dir="./project"

    print_info "Скачивание репозиториев в папку: $target_dir"

    mkdir -p "$target_dir"
    cd "$target_dir"

    print_info "Скачивание первого репозитория: $gsmLoraServer"
    if [ -d "GsmLoraServer" ]; then
        print_info "Папка GsmLoraServer уже существует, обновляем"
        cd GsmLoraServer && git pull && cd ..
    else
        git clone "$gsmLoraServer" GsmLoraServer
    fi

    print_info "Скачивание второго репозитория: $loraInterface"
    if [ -d "LoRaStandInterface" ]; then
        print_info "Папка LoRaStandInterface уже существует, обновляем"
        cd LoRaStandInterface && git pull && cd ..
    else
        git clone "$loraInterface" LoRaStandInterface
    fi
    print_info "Репоизитории успешно скачаны"
}

# Основная функция установки зависимостей
main() {
    print_info "Начало установки зависимостей..."

    detect_distro

    check_sudo

    install_docker
    install_java_via_sdkman
    install_maven
    download_repos

    print_info "========================================="
    print_info "Зависимости успешно установлены!"
    print_info ""
    print_info "Установлено:"
    print_info "1. Docker и Docker Compose"
    print_info "2. Java 23 через SDKMAN"
    print_info "3. Apache Maven"
    print_info ""
    print_info "ВАЖНО: Для применения изменений группы docker необходимо:"
    print_info "  - Выйти из системы и заново войти"
    print_info "  - Или выполнить: newgrp docker"
    print_info "========================================="
}

trap 'print_info "Скрипт прерван пользователем"; exit 1' INT

main
